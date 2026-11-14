package handlers

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/kputrajaya/uplass/models"
	"github.com/kputrajaya/uplass/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	FileSizeMax     = 10 * 1024 * 1024
	TokenExpiryHour = 3
	dbTimeout       = 10 * time.Second
)

var (
	getGDB     = utils.GetGDB
	uploadToS3 = utils.UploadToS3
)

func Ping(c fiber.Ctx) error {
	return c.SendString("Pong")
}

// GetToken returns a single-use token; the secret is compared here, never in SQL
func GetToken(c fiber.Ctx) error {
	body := models.App{}
	if err := c.Bind().Body(&body); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Failed to parse body")
	}
	if body.AppKey == "" || body.AppSecret == "" {
		return fiber.NewError(fiber.StatusBadRequest, "App key or secret not provided")
	}

	db, cancel := requestDB(c)
	defer cancel()

	app := models.App{}
	err := db.Select("id", "app_secret").Where("app_key = ?", body.AppKey).Take(&app).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fiber.NewError(fiber.StatusBadRequest, "App is not valid")
	}
	if err != nil {
		log.Println(err)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to validate app")
	}
	if !utils.VerifySecret(app.AppSecret, body.AppSecret) {
		return fiber.NewError(fiber.StatusBadRequest, "App is not valid")
	}

	token := models.Token{AppID: app.ID, Value: utils.RandomString(50)}
	if err := db.Create(&token).Error; err != nil {
		log.Println(err)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create token")
	}
	return c.SendString(token.Value)
}

// UploadAsset spends the token before uploading: a failed upload burns it
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

	db, cancel := requestDB(c)
	defer cancel()

	// Claim the token atomically so a concurrent upload cannot reuse it
	token := models.Token{}
	res := db.Model(&token).
		Clauses(clause.Returning{Columns: []clause.Column{{Name: "app_id"}}}).
		Where("value = ? AND used = ? AND created_at >= ?", tokenValue, false, time.Now().Add(-TokenExpiryHour*time.Hour)).
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

	// Object path: <app_key>/YYYY/MM/DD/<10 random chars>/<filename>
	prefix := app.AppKey + time.Now().Format("/2006/01/02/") + utils.RandomString(10) + "/"
	filePath, err := uploadToS3(fileHeader, prefix)
	if err != nil {
		log.Println(err)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to upload asset")
	}

	assetHost := strings.TrimRight(os.Getenv("ASSET_HOST"), "/") + "/"
	return c.SendString(assetHost + filePath)
}

// requestDB scopes the pool to a bounded context; callers defer the cancel
func requestDB(c fiber.Ctx) (*gorm.DB, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	return getGDB().WithContext(ctx), cancel
}
