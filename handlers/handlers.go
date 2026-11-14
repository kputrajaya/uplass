package handlers

import (
	"errors"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/kputrajaya/uplass/models"
	"github.com/kputrajaya/uplass/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const TokenExpiryHour = 3
const FileSizeMax = 10 * 1024 * 1024

var getGDB = utils.GetGDB
var uploadToS3 = utils.UploadToS3

func Ping(c fiber.Ctx) error {
	return c.SendString("Pong")
}

func GetToken(c fiber.Ctx) error {
	// Parse body
	data := models.App{}
	if err := c.Bind().Body(&data); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Failed to parse body")
	}
	if data.AppKey == "" || data.AppSecret == "" {
		return fiber.NewError(fiber.StatusBadRequest, "App key or secret not provided")
	}

	db := getGDB()

	// Validate key and secret; the secret is compared in Go so it never reaches the database
	app := models.App{}
	err := db.Select("id", "app_secret").Where("app_key = ?", data.AppKey).Take(&app).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fiber.NewError(fiber.StatusBadRequest, "App is not valid")
	}
	if err != nil {
		log.Println(err)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to validate app")
	}
	if !utils.VerifySecret(app.AppSecret, data.AppSecret) {
		return fiber.NewError(fiber.StatusBadRequest, "App is not valid")
	}

	// Create token
	token := models.Token{AppID: app.ID, Value: utils.RandomString(50)}
	if err := db.Create(&token).Error; err != nil {
		log.Println(err)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create token")
	}
	return c.SendString(token.Value)
}

func UploadAsset(c fiber.Ctx) error {
	tokenValue := c.FormValue("token")
	if tokenValue == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Token not provided")
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "File is not valid")
	}
	if fileHeader.Size > FileSizeMax {
		return fiber.NewError(fiber.StatusBadRequest, "File is too large")
	}

	db := getGDB()

	// Validate and mark token as used atomically
	firstValidDate := time.Now().Add(-TokenExpiryHour * time.Hour)
	token := models.Token{}
	res := db.Model(&token).
		Clauses(clause.Returning{Columns: []clause.Column{{Name: "app_id"}}}).
		Where("value = ? AND used = ? AND created_at >= ?", tokenValue, false, firstValidDate).
		Update("used", true)
	if res.Error != nil {
		log.Println(res.Error)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to use token")
	}
	if res.RowsAffected == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Token is not valid")
	}

	app := models.App{}
	if err := db.Select("app_key").Take(&app, token.AppID).Error; err != nil {
		log.Println(err)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to load app")
	}

	prefix := app.AppKey + time.Now().Format("/2006/01/02/") + utils.RandomString(5) + "/"
	if err := uploadToS3(fileHeader, prefix); err != nil {
		log.Println(err)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to upload asset")
	}

	host := os.Getenv("ASSET_HOST")
	return c.SendString(host + prefix + fileHeader.Filename)
}
