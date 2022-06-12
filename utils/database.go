package utils

import (
	"database/sql"
	"log"
	"os"
	"strconv"

	_ "github.com/lib/pq"
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

	sdb, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Can't open SQL conn", err)
	}
	sdb.SetMaxOpenConns(maxOpenConns)

	memoizedGDB, err := gorm.Open(postgres.New(postgres.Config{Conn: sdb}), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		log.Fatal("Can't create GORM conn", err)
	}
	return memoizedGDB
}
