package handlers

import (
	"bytes"
	"errors"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/kputrajaya/uplass/models"
	"github.com/kputrajaya/uplass/utils"
)

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
	appRow := seedApp(t, db, "app-key", "s3cret")

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
		if stored.AppID != appRow.ID {
			t.Fatalf("token app id = %d, want %d", stored.AppID, appRow.ID)
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
		if err := db.First(&stored, appRow.ID).Error; err != nil {
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
	prefixPattern := regexp.MustCompile(`^app-key/\d{4}/\d{2}/\d{2}/[A-Z2-7]{10}/$`)

	t.Run("success", func(t *testing.T) {
		f := newFixture(t, nil)

		body := expectStatus(t, doUpload(t, f.app, f.token.Value, "pic.png", []byte("hello")), fiber.StatusOK)

		if want := testAssetHost + f.upload.prefix + "pic.png"; body != want {
			t.Fatalf("body = %q, want %q", body, want)
		}
		if !f.upload.called {
			t.Fatal("S3 upload was not called")
		}
		if !prefixPattern.MatchString(f.upload.prefix) {
			t.Fatalf("prefix = %q, want <app_key>/YYYY/MM/DD/<10 base32>/", f.upload.prefix)
		}
		if f.upload.name != "pic.png" || f.upload.size != int64(len("hello")) {
			t.Fatalf("uploaded %q (%d bytes), want pic.png (5 bytes)", f.upload.name, f.upload.size)
		}

		stored := models.Token{}
		if err := f.db.First(&stored, f.token.ID).Error; err != nil {
			t.Fatalf("load token: %v", err)
		}
		if !stored.Used {
			t.Fatal("token was not marked as used")
		}
	})

	t.Run("filename is sanitised", func(t *testing.T) {
		f := newFixture(t, nil)

		body := expectStatus(t, doUpload(t, f.app, f.token.Value, "../evil.png", []byte("hello")), fiber.StatusOK)
		if want := testAssetHost + f.upload.prefix + "evil.png"; body != want {
			t.Fatalf("body = %q, want %q", body, want)
		}
	})

	t.Run("asset host comes from env", func(t *testing.T) {
		f := newFixture(t, nil)
		t.Setenv("ASSET_HOST", "https://other.test/")

		body := expectStatus(t, doUpload(t, f.app, f.token.Value, "pic.png", []byte("hello")), fiber.StatusOK)
		if want := "https://other.test/" + f.upload.prefix + "pic.png"; body != want {
			t.Fatalf("body = %q, want %q", body, want)
		}
	})

	t.Run("unknown token", func(t *testing.T) {
		f := newFixture(t, nil)

		expectMessage(t, doUpload(t, f.app, "not-a-token", "pic.png", []byte("hello")),
			fiber.StatusBadRequest, "Token is not valid")
		if f.upload.called {
			t.Fatal("S3 upload must not run for an invalid token")
		}
	})

	t.Run("expired token", func(t *testing.T) {
		f := newFixture(t, nil)
		expire := time.Now().Add(-(TokenExpiryHour + 1) * time.Hour)
		if err := f.db.Model(&f.token).UpdateColumn("created_at", expire).Error; err != nil {
			t.Fatalf("expire token: %v", err)
		}

		expectMessage(t, doUpload(t, f.app, f.token.Value, "pic.png", []byte("hello")),
			fiber.StatusBadRequest, "Token is not valid")
		if f.upload.called {
			t.Fatal("S3 upload must not run for an expired token")
		}

		stored := models.Token{}
		if err := f.db.First(&stored, f.token.ID).Error; err != nil {
			t.Fatalf("load token: %v", err)
		}
		if stored.Used {
			t.Fatal("expired token must stay unused")
		}
	})

	t.Run("already used token", func(t *testing.T) {
		f := newFixture(t, nil)
		if err := f.db.Model(&f.token).Update("used", true).Error; err != nil {
			t.Fatalf("mark token used: %v", err)
		}

		expectMessage(t, doUpload(t, f.app, f.token.Value, "pic.png", []byte("hello")),
			fiber.StatusBadRequest, "Token is not valid")
		if f.upload.called {
			t.Fatal("S3 upload must not run for a used token")
		}
	})

	t.Run("token is single use", func(t *testing.T) {
		f := newFixture(t, nil)

		expectStatus(t, doUpload(t, f.app, f.token.Value, "one.png", []byte("first")), fiber.StatusOK)
		expectMessage(t, doUpload(t, f.app, f.token.Value, "two.png", []byte("second")),
			fiber.StatusBadRequest, "Token is not valid")
	})

	t.Run("upload failure burns token", func(t *testing.T) {
		f := newFixture(t, errors.New("s3 is down"))

		expectMessage(t, doUpload(t, f.app, f.token.Value, "pic.png", []byte("hello")),
			fiber.StatusInternalServerError, "Failed to upload asset")

		stored := models.Token{}
		if err := f.db.First(&stored, f.token.ID).Error; err != nil {
			t.Fatalf("load token: %v", err)
		}
		if !stored.Used {
			t.Fatal("token must be burned even when the upload fails")
		}
	})

	t.Run("soft deleted app", func(t *testing.T) {
		f := newFixture(t, nil)
		if err := f.db.Delete(&models.App{}, f.appRow.ID).Error; err != nil {
			t.Fatalf("delete app: %v", err)
		}

		expectMessage(t, doUpload(t, f.app, f.token.Value, "pic.png", []byte("hello")),
			fiber.StatusInternalServerError, "Failed to load app")
		if f.upload.called {
			t.Fatal("S3 upload must not run when the app cannot be loaded")
		}
	})
}
