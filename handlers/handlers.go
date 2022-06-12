package handlers

import (
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kputrajaya/uplass/models"
	"github.com/kputrajaya/uplass/utils"
)

const tokenExpiryHour = 3
const fileSizeMax = 10 * 1024 * 1024

func Ping(c *fiber.Ctx) error {
	return c.SendString("Pong")
}

func GetToken(c *fiber.Ctx) error {
	// Parse body
	data := models.App{}
	if err := c.BodyParser(&data); err != nil {
		log.Println(err)
		return fiber.NewError(fiber.StatusBadRequest, "Failed to parse body")
	}
	if data.AppKey == "" || data.AppSecret == "" {
		return fiber.NewError(fiber.StatusBadRequest, "App key or secret not provided.")
	}

	db := utils.GetGDB()

	// Validate key and secret
	app := models.App{}
	db.Where("app_key = ? AND app_secret = ?", data.AppKey, utils.HashString(data.AppSecret)).Limit(1).Find(&app)
	if app.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "App with given key and secret not found.")
	}

	// Create token
	token := models.Token{AppID: app.ID, Value: utils.RandomString(32)}
	if err := db.Create(&token).Error; err != nil {
		log.Println(err)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create token.")
	}
	return c.SendString(token.Value)
}

func UploadAsset(c *fiber.Ctx) error {
	tokenValue := c.FormValue("token")
	if tokenValue == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Token is not valid.")
	}
	file, err := c.FormFile("file")
	if err != nil {
		log.Println(err)
		return fiber.NewError(fiber.StatusBadRequest, "File is not valid.")
	}
	if file.Size > fileSizeMax {
		return fiber.NewError(fiber.StatusBadRequest, "File is too large.")
	}

	db := utils.GetGDB()

	// Validate token
	token := models.Token{}
	firstValidDate := time.Now().Add(-tokenExpiryHour * time.Hour)
	db.Where("value = ? AND used = ? AND created_at >= ?", tokenValue, false, firstValidDate).Limit(1).Find(&token)
	if token.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Valid token with given value not found.")
	}

	// Mark token as used
	token.Used = true
	if err := db.Save(&token).Error; err != nil {
		log.Println(err)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to use token.")
	}

	return c.SendString("X")
}
