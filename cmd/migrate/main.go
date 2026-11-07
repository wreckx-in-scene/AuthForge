package main

import (
	"log"
	"os"

	"github.com/wreckx-in-scene/AuthForge/internal/migrator"
)

func main() {
	url := os.Getenv("MIGRATE_DATABASE_URL")
	if url == "" {
		log.Fatal("MIGRATE_DATABASE_URL is required")
	}
	if err := migrator.Up(url); err != nil {
		log.Fatal(err)
	}
}
