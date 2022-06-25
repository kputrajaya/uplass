package main

import (
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/joho/godotenv"
	"github.com/kputrajaya/uplass/handlers"
)

func main() {
	godotenv.Load()
	rand.Seed(time.Now().UnixNano())

	app := fiber.New()

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
