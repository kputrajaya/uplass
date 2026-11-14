package handlers

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/kputrajaya/uplass/models"
	"github.com/kputrajaya/uplass/utils"
	"gorm.io/gorm"
)

const testAssetHost = "https://cdn.test/"

// newTestApp mirrors the routes registered in main.go.
func newTestApp() *fiber.App {
	app := fiber.New(fiber.Config{
		BodyLimit: FileSizeMax + (1 << 20),
	})
	app.Get("/ping", Ping)
	app.Post("/token", GetToken)
	app.Post("/upload", UploadAsset)
	return app
}

// testDB connects to the database pointed at by TEST_PGSQL_CONN_STR and clears it.
// The test is skipped when the variable is unset, so `go test ./...` works without
// external services.
func testDB(t *testing.T) *gorm.DB {
	t.Helper()

	conn := os.Getenv("TEST_PGSQL_CONN_STR")
	if conn == "" {
		t.Skip("TEST_PGSQL_CONN_STR not set, skipping database integration test")
	}

	t.Setenv("PGSQL_CONN_STR", conn)
	t.Setenv("PGSQL_MAX_OPEN_CONNS", "5")
	t.Setenv("ASSET_HOST", testAssetHost)

	db := utils.GetGDB()
	if err := db.AutoMigrate(&models.App{}, &models.Token{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	if err := db.Exec("TRUNCATE TABLE tokens, apps RESTART IDENTITY CASCADE").Error; err != nil {
		t.Fatalf("truncate tables: %v", err)
	}
	return db
}

func seedApp(t *testing.T, db *gorm.DB, key, secret string) models.App {
	t.Helper()

	hashed, err := utils.HashSecret(secret)
	if err != nil {
		t.Fatalf("hash secret: %v", err)
	}

	app := models.App{AppKey: key, AppSecret: hashed}
	if err := db.Create(&app).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}
	return app
}

func seedToken(t *testing.T, db *gorm.DB, appID uint) models.Token {
	t.Helper()

	token := models.Token{AppID: appID, Value: utils.RandomString(50)}
	if err := db.Create(&token).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}
	return token
}

type uploadCall struct {
	called bool
	prefix string
	name   string
	size   int64
}

// stubUpload replaces the S3 uploader for the duration of the test.
func stubUpload(t *testing.T, err error) *uploadCall {
	t.Helper()

	call := &uploadCall{}
	original := uploadToS3
	uploadToS3 = func(header *multipart.FileHeader, prefix string) error {
		call.called = true
		call.prefix = prefix
		call.name = header.Filename
		call.size = header.Size
		return err
	}
	t.Cleanup(func() {
		uploadToS3 = original
	})
	return call
}

func doUpload(t *testing.T, app *fiber.App, tokenValue, filename string, content []byte) *http.Response {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if tokenValue != "" {
		if err := writer.WriteField("token", tokenValue); err != nil {
			t.Fatalf("write token field: %v", err)
		}
	}
	if filename != "" {
		fileWriter, err := writer.CreateFormFile("file", filename)
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := fileWriter.Write(content); err != nil {
			t.Fatalf("write file content: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(fiber.MethodPost, "/upload", body)
	req.Header.Set(fiber.HeaderContentType, writer.FormDataContentType())

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return resp
}

func expectStatus(t *testing.T, resp *http.Response, want int) string {
	t.Helper()

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != want {
		t.Fatalf("status = %d, want %d, body = %q", resp.StatusCode, want, body)
	}
	return string(body)
}

func TestPing(t *testing.T) {
	resp, err := newTestApp().Test(httptest.NewRequest(fiber.MethodGet, "/ping", nil))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if body := expectStatus(t, resp, fiber.StatusOK); body != "Pong" {
		t.Fatalf("body = %q, want %q", body, "Pong")
	}
}

func TestGetToken(t *testing.T) {
	db := testDB(t)
	app := newTestApp()
	created := seedApp(t, db, "app-key", "s3cret")

	t.Run("valid credentials", func(t *testing.T) {
		req := httptest.NewRequest(fiber.MethodPost, "/token",
			strings.NewReader(`{"AppKey":"app-key","AppSecret":"s3cret"}`))
		req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		body := expectStatus(t, resp, fiber.StatusOK)
		if len(body) != 50 {
			t.Fatalf("token length = %d, want 50", len(body))
		}

		stored := models.Token{}
		if err := db.Where("value = ?", body).First(&stored).Error; err != nil {
			t.Fatalf("load created token: %v", err)
		}
		if stored.AppID != created.ID {
			t.Fatalf("token app id = %d, want %d", stored.AppID, created.ID)
		}
		if stored.Used {
			t.Fatal("new token is already marked as used")
		}
		if !strings.HasPrefix(created.AppSecret, "$2") {
			t.Fatalf("stored app secret is not a bcrypt hash: %q", created.AppSecret)
		}
	})

	t.Run("wrong secret", func(t *testing.T) {
		var before int64
		if err := db.Model(&models.Token{}).Count(&before).Error; err != nil {
			t.Fatalf("count tokens: %v", err)
		}

		req := httptest.NewRequest(fiber.MethodPost, "/token",
			strings.NewReader(`{"AppKey":"app-key","AppSecret":"wrong"}`))
		req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		expectStatus(t, resp, fiber.StatusBadRequest)

		var after int64
		if err := db.Model(&models.Token{}).Count(&after).Error; err != nil {
			t.Fatalf("count tokens: %v", err)
		}
		if after != before {
			t.Fatalf("token count changed from %d to %d on invalid credentials", before, after)
		}
	})

	t.Run("missing credentials", func(t *testing.T) {
		req := httptest.NewRequest(fiber.MethodPost, "/token",
			strings.NewReader(`{"AppKey":"app-key"}`))
		req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		expectStatus(t, resp, fiber.StatusBadRequest)
	})
}

func TestUploadAsset(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db := testDB(t)
		app := newTestApp()
		appModel := seedApp(t, db, "app-key", "s3cret")
		token := seedToken(t, db, appModel.ID)
		call := stubUpload(t, nil)

		resp := doUpload(t, app, token.Value, "pic.png", []byte("hello"))
		body := expectStatus(t, resp, fiber.StatusOK)

		want := testAssetHost + call.prefix + "pic.png"
		if body != want {
			t.Fatalf("body = %q, want %q", body, want)
		}
		if !call.called {
			t.Fatal("S3 upload was not called")
		}
		if !strings.HasPrefix(call.prefix, appModel.AppKey+"/") {
			t.Fatalf("prefix = %q, want app key prefix %q", call.prefix, appModel.AppKey+"/")
		}
		if !strings.HasSuffix(call.prefix, "/") {
			t.Fatalf("prefix = %q, want trailing slash", call.prefix)
		}
		if call.name != "pic.png" {
			t.Fatalf("uploaded filename = %q, want pic.png", call.name)
		}
		if call.size != int64(len("hello")) {
			t.Fatalf("uploaded size = %d, want %d", call.size, len("hello"))
		}

		stored := models.Token{}
		if err := db.First(&stored, token.ID).Error; err != nil {
			t.Fatalf("load token: %v", err)
		}
		if !stored.Used {
			t.Fatal("token was not marked as used")
		}
	})

	t.Run("unknown token", func(t *testing.T) {
		db := testDB(t)
		app := newTestApp()
		appModel := seedApp(t, db, "app-key", "s3cret")
		seedToken(t, db, appModel.ID)
		call := stubUpload(t, nil)

		resp := doUpload(t, app, "not-a-token", "pic.png", []byte("hello"))
		expectStatus(t, resp, fiber.StatusBadRequest)

		if call.called {
			t.Fatal("S3 upload must not run for an invalid token")
		}
	})

	t.Run("expired token", func(t *testing.T) {
		db := testDB(t)
		app := newTestApp()
		appModel := seedApp(t, db, "app-key", "s3cret")
		token := seedToken(t, db, appModel.ID)
		if err := db.Model(&token).
			UpdateColumn("created_at", time.Now().Add(-(TokenExpiryHour+1)*time.Hour)).Error; err != nil {
			t.Fatalf("expire token: %v", err)
		}
		call := stubUpload(t, nil)

		resp := doUpload(t, app, token.Value, "pic.png", []byte("hello"))
		expectStatus(t, resp, fiber.StatusBadRequest)

		if call.called {
			t.Fatal("S3 upload must not run for an expired token")
		}

		stored := models.Token{}
		if err := db.First(&stored, token.ID).Error; err != nil {
			t.Fatalf("load token: %v", err)
		}
		if stored.Used {
			t.Fatal("expired token must stay unused")
		}
	})

	t.Run("already used token", func(t *testing.T) {
		db := testDB(t)
		app := newTestApp()
		appModel := seedApp(t, db, "app-key", "s3cret")
		token := seedToken(t, db, appModel.ID)
		if err := db.Model(&token).Update("used", true).Error; err != nil {
			t.Fatalf("mark token used: %v", err)
		}
		call := stubUpload(t, nil)

		resp := doUpload(t, app, token.Value, "pic.png", []byte("hello"))
		expectStatus(t, resp, fiber.StatusBadRequest)

		if call.called {
			t.Fatal("S3 upload must not run for a used token")
		}
	})

	t.Run("token is single use", func(t *testing.T) {
		db := testDB(t)
		app := newTestApp()
		appModel := seedApp(t, db, "app-key", "s3cret")
		token := seedToken(t, db, appModel.ID)
		stubUpload(t, nil)

		first := doUpload(t, app, token.Value, "one.png", []byte("first"))
		expectStatus(t, first, fiber.StatusOK)

		second := doUpload(t, app, token.Value, "two.png", []byte("second"))
		expectStatus(t, second, fiber.StatusBadRequest)
	})

	t.Run("upload failure burns token", func(t *testing.T) {
		db := testDB(t)
		app := newTestApp()
		appModel := seedApp(t, db, "app-key", "s3cret")
		token := seedToken(t, db, appModel.ID)
		stubUpload(t, errors.New("s3 is down"))

		resp := doUpload(t, app, token.Value, "pic.png", []byte("hello"))
		expectStatus(t, resp, fiber.StatusInternalServerError)

		stored := models.Token{}
		if err := db.First(&stored, token.ID).Error; err != nil {
			t.Fatalf("load token: %v", err)
		}
		if !stored.Used {
			t.Fatal("token must be burned even when the upload fails")
		}
	})

	t.Run("soft deleted app", func(t *testing.T) {
		db := testDB(t)
		app := newTestApp()
		appModel := seedApp(t, db, "app-key", "s3cret")
		token := seedToken(t, db, appModel.ID)
		if err := db.Delete(&models.App{}, appModel.ID).Error; err != nil {
			t.Fatalf("delete app: %v", err)
		}
		call := stubUpload(t, nil)

		resp := doUpload(t, app, token.Value, "pic.png", []byte("hello"))
		expectStatus(t, resp, fiber.StatusInternalServerError)

		if call.called {
			t.Fatal("S3 upload must not run when the app cannot be loaded")
		}
	})

	t.Run("missing token", func(t *testing.T) {
		app := newTestApp()
		call := stubUpload(t, nil)

		resp := doUpload(t, app, "", "pic.png", []byte("hello"))
		expectStatus(t, resp, fiber.StatusBadRequest)

		if call.called {
			t.Fatal("S3 upload must not run without a token")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		app := newTestApp()
		call := stubUpload(t, nil)

		resp := doUpload(t, app, "some-token", "", nil)
		expectStatus(t, resp, fiber.StatusBadRequest)

		if call.called {
			t.Fatal("S3 upload must not run without a file")
		}
	})

	t.Run("file too large", func(t *testing.T) {
		app := newTestApp()
		call := stubUpload(t, nil)

		content := bytes.Repeat([]byte{'a'}, FileSizeMax+1)
		resp := doUpload(t, app, "some-token", "big.bin", content)
		expectStatus(t, resp, fiber.StatusBadRequest)

		if call.called {
			t.Fatal("S3 upload must not run for oversized files")
		}
	})
}
