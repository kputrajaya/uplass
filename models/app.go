package models

import "gorm.io/gorm"

type App struct {
	gorm.Model
	AppKey    string `gorm:"uniqueIndex"`
	AppSecret string
}
