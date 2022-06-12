package models

import "gorm.io/gorm"

type Asset struct {
	gorm.Model
	AppID    uint
	App      App
	FileName string
	Path     string `gorm:"uniqueIndex"`
	Size     int32
}
