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

func main() {
	godotenv.Load()

	app := fiber.New(fiber.Config{
		BodyLimit: handlers.FileSizeMax + (1 << 20),
	})

	app.Use(compress.New())
	app.Use(cors.New())

	app.Get("/ping", handlers.Ping)

	app.Post("/token", handlers.GetToken)
	app.Post("/upload", handlers.UploadAsset)

	listenAddress := "localhost:8080"
	if port := os.Getenv("PORT"); port != "" {
		listenAddress = ":" + port
	}
	log.Fatal(app.Listen(listenAddress))
}
