package utils

import (
	"log"
	"os"
	"strconv"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var memoizedGDB *gorm.DB

func GetGDB() *gorm.DB {
	if memoizedGDB != nil {
		return memoizedGDB
	}

	connStr := os.Getenv("PGSQL_CONN_STR")
	maxOpenConns, err := strconv.Atoi(os.Getenv("PGSQL_MAX_OPEN_CONNS"))
	if connStr == "" || err != nil {
		log.Fatal("Invalid PGSQL settings", err)
	}

	gdb, err := gorm.Open(postgres.Open(connStr), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		log.Fatal("Can't create GORM conn", err)
	}

	sdb, err := gdb.DB()
	if err != nil {
		log.Fatal("Can't get SQL pool", err)
	}
	sdb.SetMaxOpenConns(maxOpenConns)

	memoizedGDB = gdb
	return memoizedGDB
}
