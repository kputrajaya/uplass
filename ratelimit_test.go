package main

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestClientKey(t *testing.T) {
	cases := []struct {
		name string
		xff  string
		want string
	}{
		{"single entry", "203.0.113.7", "203.0.113.7"},
		{"chain uses last", "1.1.1.1, 203.0.113.7", "203.0.113.7"},
		{"spoofed prefix ignored", "9.9.9.9, 8.8.8.8, 203.0.113.7", "203.0.113.7"},
		{"spaces trimmed", "1.1.1.1 ,  203.0.113.7 ", "203.0.113.7"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(fiber.MethodPost, "/upload", nil)
			req.Header.Set(fiber.HeaderXForwardedFor, tc.xff)
			app := fiber.New()
			var got string
			app.Post("/upload", func(c fiber.Ctx) error {
				got = clientKey(c)
				return nil
			})
			if _, err := app.Test(req); err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			if got != tc.want {
				t.Fatalf("clientKey() = %q, want %q", got, tc.want)
			}
		})
	}

	t.Run("no header falls back to peer ip", func(t *testing.T) {
		req := httptest.NewRequest(fiber.MethodPost, "/upload", nil)
		app := fiber.New()
		var got, peer string
		app.Post("/upload", func(c fiber.Ctx) error {
			got = clientKey(c)
			peer = c.IP()
			return nil
		})
		if _, err := app.Test(req); err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		if got != peer {
			t.Fatalf("clientKey() = %q, want the peer address %q", got, peer)
		}
	})
}

func TestRateLimitPerClient(t *testing.T) {
	app := newApp()

	post := func(clientIP string) (int, string, string) {
		t.Helper()
		req := httptest.NewRequest(fiber.MethodPost, "/upload", nil)
		req.Header.Set(fiber.HeaderXForwardedFor, clientIP)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		defer resp.Body.Close()
		body := make([]byte, 64)
		n, _ := resp.Body.Read(body)
		return resp.StatusCode, string(body[:n]), resp.Header.Get(fiber.HeaderRetryAfter)
	}

	const attemptsPerMinute = 10
	for i := 0; i < attemptsPerMinute; i++ {
		if code, _, _ := post("203.0.113.7"); code != fiber.StatusBadRequest {
			t.Fatalf("request %d = %d, want 400", i+1, code)
		}
	}

	code, body, retryAfter := post("203.0.113.7")
	if code != fiber.StatusTooManyRequests {
		t.Fatalf("request over the limit = %d, want 429", code)
	}
	if body != "Too many requests" {
		t.Fatalf("body = %q, want %q", body, "Too many requests")
	}
	if retryAfter != "60" {
		t.Fatalf("Retry-After = %q, want %q", retryAfter, "60")
	}

	if code, _, _ := post("198.51.100.9"); code != fiber.StatusBadRequest {
		t.Fatalf("second client = %d, want 400 (own bucket)", code)
	}
}
