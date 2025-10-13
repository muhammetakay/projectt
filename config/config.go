package config

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	DB *gorm.DB

	MaxPlayers           int
	ChunkSize            int
	MaxChunkViewDistance int
	MaxViewDistance      int
)

func Init() {
	var err error

	// Load game settings from environment variables
	MaxPlayers, err = strconv.Atoi(os.Getenv("MAX_PLAYERS"))
	if err != nil {
		log.Fatalf("Invalid MAX_PLAYERS value: %v", err)
	}

	ChunkSize, err = strconv.Atoi(os.Getenv("CHUNK_SIZE"))
	if err != nil {
		log.Fatalf("Invalid CHUNK_SIZE value: %v", err)
	}

	MaxChunkViewDistance, err = strconv.Atoi(os.Getenv("MAX_CHUNK_VIEW_DISTANCE"))
	if err != nil {
		log.Fatalf("Invalid MAX_CHUNK_VIEW_DISTANCE value: %v", err)
	}

	MaxViewDistance = ChunkSize * MaxChunkViewDistance
}

func ConnectDatabase() {
	host := os.Getenv("DB_HOST")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	dbname := os.Getenv("DB_NAME")
	port := os.Getenv("DB_PORT")

	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=UTC",
		host,
		user,
		password,
		dbname,
		port,
	)

	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// Connection pool configuration
	sqlDB, err := database.DB()
	if err != nil {
		log.Fatal("Failed to get database object:", err)
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	DB = database
}

func GetDBStats() sql.DBStats {
	sqlDB, err := DB.DB()
	if err != nil {
		log.Printf("Error getting database instance: %v", err)
		return sql.DBStats{}
	}
	return sqlDB.Stats()
}
