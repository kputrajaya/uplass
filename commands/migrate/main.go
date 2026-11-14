package main

import (
	"log"

	"github.com/joho/godotenv"
	"github.com/kputrajaya/uplass/models"
	"github.com/kputrajaya/uplass/utils"
)

// AutoMigrate is additive; it never drops columns
func main() {
	godotenv.Load()

	db := utils.GetGDB()
	if err := db.AutoMigrate(
		&models.App{},
		&models.Token{},
	); err != nil {
		log.Fatal("Can't migrate", err)
	}
}
