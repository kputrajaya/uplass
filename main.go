package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/compress"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/joho/godotenv"
	"github.com/kputrajaya/uplass/handlers"
)

func newApp() *fiber.App {
	app := fiber.New(fiber.Config{
		BodyLimit: handlers.FileSizeMax + (1 << 20),
	})

	app.Use(compress.New())
	app.Use(cors.New())

	app.Get("/ping", handlers.Ping)

	app.Post("/token", handlers.GetToken)
	app.Post("/upload", handlers.UploadAsset)

	return app
}

func listenAddress() string {
	if port := os.Getenv("PORT"); port != "" {
		return ":" + port
	}
	return "localhost:8080"
}

func main() {
	godotenv.Load()
	log.Fatal(newApp().Listen(listenAddress()))
}
