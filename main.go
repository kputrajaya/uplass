package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/compress"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/joho/godotenv"
	"github.com/kputrajaya/uplass/handlers"
	"github.com/kputrajaya/uplass/utils"
)

// newApp builds the real router for main and tests
func newApp() *fiber.App {
	app := fiber.New(fiber.Config{
		BodyLimit:    handlers.FileSizeMax + (1 << 20),
		ReadTimeout:  2 * time.Minute,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  2 * time.Minute,
	})

	app.Use(recover.New(recover.Config{
		PanicHandler: func(_ fiber.Ctx, r any) error {
			log.Printf("panic: %v", r)
			return fiber.NewError(fiber.StatusInternalServerError, "Internal server error")
		},
	}))
	app.Use(compress.New())
	app.Use(cors.New())

	app.Get("/ping", handlers.Ping)

	app.Post("/token", rateLimiter(10), handlers.GetToken)
	app.Post("/upload", rateLimiter(10), handlers.UploadAsset)

	return app
}

// listenAddress uses localhost unless PORT is set
func listenAddress() string {
	if port := os.Getenv("PORT"); port != "" {
		return ":" + port
	}
	return "localhost:8080"
}

func main() {
	godotenv.Load()
	if err := validateConfig(); err != nil {
		log.Fatal(err)
	}

	// Open the pool now so a malformed PGSQL_CONN_STR cannot kill the process mid-request
	utils.GetGDB()

	// SIGTERM drains in-flight requests before the process exits
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := newApp().Listen(listenAddress(), fiber.ListenConfig{GracefulContext: ctx})
	if err != nil {
		log.Fatal(err)
	}
}
