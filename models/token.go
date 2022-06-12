package models

import "gorm.io/gorm"

type Token struct {
	gorm.Model
	AppID uint
	App   App
	Value string `gorm:"uniqueIndex"`
	Used  bool   `gorm:"default: false"`
}
