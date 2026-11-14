package utils

import (
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var getGDB = sync.OnceValue(openGDB)

// GetGDB returns the process-wide pool
func GetGDB() *gorm.DB {
	return getGDB()
}

func openGDB() *gorm.DB {
	connStr := os.Getenv("PGSQL_CONN_STR")
	maxOpenConns, err := strconv.Atoi(os.Getenv("PGSQL_MAX_OPEN_CONNS"))
	if connStr == "" || err != nil || maxOpenConns <= 0 {
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
	sdb.SetMaxIdleConns(maxOpenConns)
	// Recycle connections so TLS handshakes and DB-side failovers stay fresh
	sdb.SetConnMaxLifetime(5 * time.Minute)

	return gdb
}
