package main

import (
	"errors"
	"log/slog"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		slog.Error("DATABASE_URL must be configured")
		os.Exit(1)
	}
	migrator, err := migrate.New("file://migrations", databaseURL)
	if err != nil {
		slog.Error("failed to initialize payments migrations", "error", err)
		os.Exit(1)
	}
	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		slog.Error("payments migration failed", "error", err)
		os.Exit(1)
	}
}
