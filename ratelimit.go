package main

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
)

const (
	// tokenRateLimit allows 20 token attempts per client per minute.
	tokenRateLimit = 20
	// uploadRateLimit allows 20 upload attempts per client per minute.
	uploadRateLimit = 20
	rateLimitWindow = time.Minute
)

// rateLimiter allows max requests per client per window.
func rateLimiter(max int) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:          max,
		Expiration:   rateLimitWindow,
		KeyGenerator: clientKey,
		LimitReached: func(c fiber.Ctx) error {
			c.Set(fiber.HeaderRetryAfter, "60")
			return fiber.NewError(fiber.StatusTooManyRequests, "Too many requests")
		},
	})
}

// clientKey uses the last X-Forwarded-For entry: Cloud Run appends the real
// client and earlier entries are caller-supplied.
func clientKey(c fiber.Ctx) string {
	xff := c.Get(fiber.HeaderXForwardedFor)
	if i := strings.LastIndexByte(xff, ','); i >= 0 {
		return strings.TrimSpace(xff[i+1:])
	}
	if xff != "" {
		return strings.TrimSpace(xff)
	}
	return c.IP()
}
