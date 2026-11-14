package main

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestListenAddress(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv("PORT", "")
		if got := listenAddress(); got != "localhost:8080" {
			t.Fatalf("listenAddress() = %q, want %q", got, "localhost:8080")
		}
	})

	t.Run("PORT overrides", func(t *testing.T) {
		t.Setenv("PORT", "9090")
		if got := listenAddress(); got != ":9090" {
			t.Fatalf("listenAddress() = %q, want %q", got, ":9090")
		}
	})
}

func TestNewAppRoutes(t *testing.T) {
	app := newApp()

	t.Run("ping", func(t *testing.T) {
		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/ping", nil))
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
	})

	t.Run("unknown route", func(t *testing.T) {
		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/nope", nil))
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		if resp.StatusCode != fiber.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
	})
}

func TestCORS(t *testing.T) {
	app := newApp()

	req := httptest.NewRequest(fiber.MethodGet, "/ping", nil)
	req.Header.Set(fiber.HeaderOrigin, "https://example.test")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if got := resp.Header.Get(fiber.HeaderAccessControlAllowOrigin); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}

	preflight := httptest.NewRequest(fiber.MethodOptions, "/token", nil)
	preflight.Header.Set(fiber.HeaderOrigin, "https://example.test")
	preflight.Header.Set(fiber.HeaderAccessControlRequestMethod, fiber.MethodPost)
	preflightResp, err := app.Test(preflight)
	if err != nil {
		t.Fatalf("app.Test preflight: %v", err)
	}
	if preflightResp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", preflightResp.StatusCode)
	}
}
