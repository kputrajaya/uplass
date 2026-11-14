package main

import (
	"github.com/joho/godotenv"
	"github.com/kputrajaya/uplass/models"
	"github.com/kputrajaya/uplass/utils"
)

// AutoMigrate is additive; it never drops columns.
func main() {
	godotenv.Load()

	db := utils.GetGDB()
	db.AutoMigrate(
		&models.App{},
		&models.Token{},
	)
}
