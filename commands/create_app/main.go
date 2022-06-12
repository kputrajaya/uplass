package main

import (
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/kputrajaya/uplass/models"
	"github.com/kputrajaya/uplass/utils"
)

func main() {
	godotenv.Load()

	if len(os.Args) < 3 {
		log.Fatal("Provide app key and secret as first and second args.")
	}

	db := utils.GetGDB()
	appKey := os.Args[1]
	appSecret := os.Args[2]
	app := models.App{AppKey: appKey, AppSecret: utils.HashString(appSecret)}
	db.Create(&app)
}
