package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5"
	"velocity/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	connStr := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.Name,
		cfg.Database.SSLMode,
	)

	conn, err := pgx.Connect(context.Background(), connStr)
	if err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}
	defer conn.Close(context.Background())

	sqlBytes, err := os.ReadFile("migrations/0011_create_wallet_transactions.up.sql")
	if err != nil {
		log.Fatalf("failed to read migration file: %v", err)
	}

	_, err = conn.Exec(context.Background(), string(sqlBytes))
	if err != nil {
		log.Fatalf("failed to execute migration: %v", err)
	}

	fmt.Println("Migration 0011 applied successfully!")
}
