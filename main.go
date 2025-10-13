package main

import (
	"log"
	"projectt/config"
	"projectt/migrations"
	"projectt/socket"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	// set timezone to utc
	time.Local = time.UTC

	// load environment variables
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	config.Init()

	// database connection
	config.ConnectDatabase()
	// migrations and seeders
	migrations.Migrate(config.DB)

	// start socket server
	socket.StartServer()
}
