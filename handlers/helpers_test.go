package handlers

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v3"
	"github.com/kputrajaya/uplass/models"
	"github.com/kputrajaya/uplass/utils"
	"gorm.io/gorm"
)

const testAssetHost = "https://cdn.test/"

// newTestApp mirrors the main.go route table
func newTestApp() *fiber.App {
	app := fiber.New(fiber.Config{
		BodyLimit: FileSizeMax + (1 << 20),
	})
	app.Get("/ping", Ping)
	app.Post("/token", GetToken)
	app.Post("/upload", UploadAsset)
	return app
}

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
	// One connection keeps the ":memory:" database alive for the whole test
	sdb.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sdb.Close() })

	if err := db.AutoMigrate(&models.App{}, &models.Token{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	previous := getGDB
	getGDB = func() *gorm.DB { return db }
	t.Cleanup(func() { getGDB = previous })

	t.Setenv("ASSET_HOST", testAssetHost)
	return db
}

type fixture struct {
	app    *fiber.App
	db     *gorm.DB
	appRow models.App
	token  models.Token
	upload *uploadCall
}

func newFixture(t *testing.T, uploadErr error) *fixture {
	t.Helper()

	db := setupDB(t)
	appRow := seedApp(t, db, "app-key", "s3cret")
	return &fixture{
		app:    newTestApp(),
		db:     db,
		appRow: appRow,
		token:  seedToken(t, db, appRow.ID),
		upload: stubUpload(t, uploadErr),
	}
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

func stubUpload(t *testing.T, err error) *uploadCall {
	t.Helper()

	call := &uploadCall{}
	original := uploadToS3
	uploadToS3 = func(header *multipart.FileHeader, prefix string) (string, error) {
		call.called = true
		call.prefix = prefix
		call.name = header.Filename
		call.size = header.Size
		if err != nil {
			return "", err
		}
		return prefix + utils.SafeFilename(header.Filename), nil
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
