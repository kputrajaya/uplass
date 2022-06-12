package handlers

import (
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kputrajaya/uplass/models"
	"github.com/kputrajaya/uplass/utils"
	"gorm.io/gorm"
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
		return fiber.NewError(fiber.StatusBadRequest, "App key or secret not provided")
	}

	db := utils.GetGDB()

	// Validate key and secret
	app := models.App{}
	db.Where("app_key = ? AND app_secret = ?", data.AppKey, utils.HashString(data.AppSecret)).Limit(1).Find(&app)
	if app.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "App with given key and secret not found")
	}

	// Create token
	token := models.Token{AppID: app.ID, Value: utils.RandomString(50)}
	if err := db.Create(&token).Error; err != nil {
		log.Println(err)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create token")
	}
	return c.SendString(token.Value)
}

func UploadAsset(c *fiber.Ctx) error {
	tokenValue := c.FormValue("token")
	if tokenValue == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Token is not valid")
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		log.Println(err)
		return fiber.NewError(fiber.StatusBadRequest, "File is not valid")
	}
	if fileHeader.Size > fileSizeMax {
		return fiber.NewError(fiber.StatusBadRequest, "File is too large")
	}

	path := ""
	db := utils.GetGDB()
	err = db.Transaction(func(tx *gorm.DB) error {
		// Validate token
		token := models.Token{}
		firstValidDate := time.Now().Add(-tokenExpiryHour * time.Hour)
		db.Joins("App").Where("tokens.value = ? AND tokens.used = ? AND tokens.created_at >= ?", tokenValue, false, firstValidDate).Limit(1).Find(&token)
		if token.ID == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "Valid token with given value not found")
		}

		// Mark token as used
		token.Used = true
		if err := db.Save(&token).Error; err != nil {
			log.Println(err)
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to use token")
		}

		// Save to asset
		prefix := token.App.AppKey + time.Now().Format("/2006/01/02/") + utils.RandomString(5) + "/"
		path = prefix + fileHeader.Filename
		asset := models.Asset{AppID: token.AppID, FileName: fileHeader.Filename, Path: path, Size: fileHeader.Size}
		if err := db.Create(&asset).Error; err != nil {
			log.Println(err)
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to create asset")
		}

		// Upload to S3
		err = utils.UploadToS3(fileHeader, prefix)
		if err != nil {
			log.Println(err)
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to upload asset")
		}

		return nil
	})
	if err != nil {
		return err
	}

	host := os.Getenv("ASSET_HOST")
	return c.SendString(host + path)
}
