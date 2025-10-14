package main

import (
	"log"
	"os"
	"os/signal"
	"projectt/config"
	"projectt/migrations"
	"projectt/socket"
	"syscall"
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
	go func() {
		socket.StartServer()
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// Save any necessary data before exiting
	socket.Save()

	log.Println("Server gracefully stopped")
}
