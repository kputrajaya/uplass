package handlers

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
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

// setupDB points the handlers at a throwaway in-memory database so the full
// request flow can be exercised without a server.
func setupDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	sdb, err := db.DB()
	if err != nil {
		t.Fatalf("db pool: %v", err)
	}
	// One connection keeps the ":memory:" database alive for the whole test.
	sdb.SetMaxOpenConns(1)

	if err := db.AutoMigrate(&models.App{}, &models.Token{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	previous := getGDB
	getGDB = func() *gorm.DB { return db }
	t.Cleanup(func() { getGDB = previous })

	t.Setenv("ASSET_HOST", testAssetHost)
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
	t.Cleanup(func() { uploadToS3 = original })
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

func expectMessage(t *testing.T, resp *http.Response, wantCode int, wantBody string) {
	t.Helper()

	if body := expectStatus(t, resp, wantCode); body != wantBody {
		t.Fatalf("body = %q, want %q", body, wantBody)
	}
}

func postJSON(t *testing.T, app *fiber.App, path, body string) *http.Response {
	t.Helper()

	req := httptest.NewRequest(fiber.MethodPost, path, strings.NewReader(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return resp
}

func countTokens(t *testing.T, db *gorm.DB) int64 {
	t.Helper()

	var count int64
	if err := db.Model(&models.Token{}).Count(&count).Error; err != nil {
		t.Fatalf("count tokens: %v", err)
	}
	return count
}

func TestPing(t *testing.T) {
	resp, err := newTestApp().Test(httptest.NewRequest(fiber.MethodGet, "/ping", nil))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if body := expectStatus(t, resp, fiber.StatusOK); body != "Pong" {
		t.Fatalf("body = %q, want %q", body, "Pong")
	}
	if got := resp.Header.Get(fiber.HeaderContentType); got != fiber.MIMETextPlainCharsetUTF8 {
		t.Fatalf("content type = %q, want %q", got, fiber.MIMETextPlainCharsetUTF8)
	}
}

func TestRouting(t *testing.T) {
	app := newTestApp()

	t.Run("unknown route", func(t *testing.T) {
		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/nope", nil))
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		expectMessage(t, resp, fiber.StatusNotFound, "Not Found")
	})

	t.Run("wrong method", func(t *testing.T) {
		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/upload", nil))
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		expectMessage(t, resp, fiber.StatusMethodNotAllowed, "Method Not Allowed")
		if allow := resp.Header.Get("Allow"); allow != fiber.MethodPost {
			t.Fatalf("Allow = %q, want %q", allow, fiber.MethodPost)
		}
	})

	t.Run("head ping", func(t *testing.T) {
		resp, err := app.Test(httptest.NewRequest(fiber.MethodHead, "/ping", nil))
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		expectMessage(t, resp, fiber.StatusOK, "")
	})
}

func TestGetTokenRejectsBadRequests(t *testing.T) {
	app := newTestApp()

	t.Run("missing credentials", func(t *testing.T) {
		expectMessage(t, postJSON(t, app, "/token", `{"AppKey":"app-key"}`),
			fiber.StatusBadRequest, "App key or secret not provided")
	})

	t.Run("empty object", func(t *testing.T) {
		expectMessage(t, postJSON(t, app, "/token", `{}`),
			fiber.StatusBadRequest, "App key or secret not provided")
	})

	t.Run("malformed json", func(t *testing.T) {
		expectMessage(t, postJSON(t, app, "/token", "not-json"),
			fiber.StatusBadRequest, "Failed to parse body")
	})
}

func TestGetToken(t *testing.T) {
	db := setupDB(t)
	app := newTestApp()
	created := seedApp(t, db, "app-key", "s3cret")

	t.Run("valid credentials", func(t *testing.T) {
		resp := postJSON(t, app, "/token", `{"AppKey":"app-key","AppSecret":"s3cret"}`)
		body := expectStatus(t, resp, fiber.StatusOK)

		if len(body) != 50 {
			t.Fatalf("token length = %d, want 50", len(body))
		}
		for _, r := range body {
			if !strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZ234567", r) {
				t.Fatalf("token contains %q, outside the base32 alphabet", r)
			}
		}
		if got := resp.Header.Get(fiber.HeaderContentType); got != fiber.MIMETextPlainCharsetUTF8 {
			t.Fatalf("content type = %q, want plain text", got)
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
	})

	t.Run("wrong secret", func(t *testing.T) {
		before := countTokens(t, db)

		resp := postJSON(t, app, "/token", `{"AppKey":"app-key","AppSecret":"wrong"}`)
		expectMessage(t, resp, fiber.StatusBadRequest, "App is not valid")

		if after := countTokens(t, db); after != before {
			t.Fatalf("token count changed from %d to %d on invalid credentials", before, after)
		}
	})

	t.Run("unknown app", func(t *testing.T) {
		resp := postJSON(t, app, "/token", `{"AppKey":"missing","AppSecret":"s3cret"}`)
		expectMessage(t, resp, fiber.StatusBadRequest, "App is not valid")
	})

	t.Run("secret is hashed at rest", func(t *testing.T) {
		stored := models.App{}
		if err := db.First(&stored, created.ID).Error; err != nil {
			t.Fatalf("load app: %v", err)
		}
		if !strings.HasPrefix(stored.AppSecret, "$2") {
			t.Fatalf("stored secret = %q, want a bcrypt hash", stored.AppSecret)
		}
		if !utils.VerifySecret(stored.AppSecret, "s3cret") {
			t.Fatal("stored hash does not verify the original secret")
		}
	})
}

func TestUploadAssetRejectsBadRequests(t *testing.T) {
	app := newTestApp()

	t.Run("missing token", func(t *testing.T) {
		expectMessage(t, doUpload(t, app, "", "pic.png", []byte("hello")),
			fiber.StatusBadRequest, "Token not provided")
	})

	t.Run("missing file", func(t *testing.T) {
		expectMessage(t, doUpload(t, app, "some-token", "", nil),
			fiber.StatusBadRequest, "File is not valid")
	})

	t.Run("file too large", func(t *testing.T) {
		content := bytes.Repeat([]byte{'a'}, FileSizeMax+1)
		expectMessage(t, doUpload(t, app, "some-token", "big.bin", content),
			fiber.StatusBadRequest, "File is too large")
	})
}

func TestUploadAsset(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db := setupDB(t)
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
		if !regexp.MustCompile(`^app-key/\d{4}/\d{2}/\d{2}/[A-Z2-7]{5}/$`).MatchString(call.prefix) {
			t.Fatalf("prefix = %q, want <app_key>/YYYY/MM/DD/<5 base32>/", call.prefix)
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
		db := setupDB(t)
		app := newTestApp()
		appModel := seedApp(t, db, "app-key", "s3cret")
		seedToken(t, db, appModel.ID)
		call := stubUpload(t, nil)

		expectMessage(t, doUpload(t, app, "not-a-token", "pic.png", []byte("hello")),
			fiber.StatusBadRequest, "Token is not valid")

		if call.called {
			t.Fatal("S3 upload must not run for an invalid token")
		}
	})

	t.Run("expired token", func(t *testing.T) {
		db := setupDB(t)
		app := newTestApp()
		appModel := seedApp(t, db, "app-key", "s3cret")
		token := seedToken(t, db, appModel.ID)
		if err := db.Model(&token).
			UpdateColumn("created_at", time.Now().Add(-(TokenExpiryHour+1)*time.Hour)).Error; err != nil {
			t.Fatalf("expire token: %v", err)
		}
		call := stubUpload(t, nil)

		expectMessage(t, doUpload(t, app, token.Value, "pic.png", []byte("hello")),
			fiber.StatusBadRequest, "Token is not valid")

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
		db := setupDB(t)
		app := newTestApp()
		appModel := seedApp(t, db, "app-key", "s3cret")
		token := seedToken(t, db, appModel.ID)
		if err := db.Model(&token).Update("used", true).Error; err != nil {
			t.Fatalf("mark token used: %v", err)
		}
		call := stubUpload(t, nil)

		expectMessage(t, doUpload(t, app, token.Value, "pic.png", []byte("hello")),
			fiber.StatusBadRequest, "Token is not valid")

		if call.called {
			t.Fatal("S3 upload must not run for a used token")
		}
	})

	t.Run("token is single use", func(t *testing.T) {
		db := setupDB(t)
		app := newTestApp()
		appModel := seedApp(t, db, "app-key", "s3cret")
		token := seedToken(t, db, appModel.ID)
		stubUpload(t, nil)

		expectStatus(t, doUpload(t, app, token.Value, "one.png", []byte("first")), fiber.StatusOK)
		expectMessage(t, doUpload(t, app, token.Value, "two.png", []byte("second")),
			fiber.StatusBadRequest, "Token is not valid")
	})

	t.Run("upload failure burns token", func(t *testing.T) {
		db := setupDB(t)
		app := newTestApp()
		appModel := seedApp(t, db, "app-key", "s3cret")
		token := seedToken(t, db, appModel.ID)
		stubUpload(t, errors.New("s3 is down"))

		expectMessage(t, doUpload(t, app, token.Value, "pic.png", []byte("hello")),
			fiber.StatusInternalServerError, "Failed to upload asset")

		stored := models.Token{}
		if err := db.First(&stored, token.ID).Error; err != nil {
			t.Fatalf("load token: %v", err)
		}
		if !stored.Used {
			t.Fatal("token must be burned even when the upload fails")
		}
	})

	t.Run("soft deleted app", func(t *testing.T) {
		db := setupDB(t)
		app := newTestApp()
		appModel := seedApp(t, db, "app-key", "s3cret")
		token := seedToken(t, db, appModel.ID)
		if err := db.Delete(&models.App{}, appModel.ID).Error; err != nil {
			t.Fatalf("delete app: %v", err)
		}
		call := stubUpload(t, nil)

		expectMessage(t, doUpload(t, app, token.Value, "pic.png", []byte("hello")),
			fiber.StatusInternalServerError, "Failed to load app")

		if call.called {
			t.Fatal("S3 upload must not run when the app cannot be loaded")
		}
	})

	t.Run("asset host comes from env", func(t *testing.T) {
		db := setupDB(t)
		app := newTestApp()
		appModel := seedApp(t, db, "app-key", "s3cret")
		token := seedToken(t, db, appModel.ID)
		call := stubUpload(t, nil)
		t.Setenv("ASSET_HOST", "https://other.test/")

		body := expectStatus(t, doUpload(t, app, token.Value, "pic.png", []byte("hello")), fiber.StatusOK)
		want := "https://other.test/" + call.prefix + "pic.png"
		if body != want {
			t.Fatalf("body = %q, want %q", body, want)
		}
	})
}
