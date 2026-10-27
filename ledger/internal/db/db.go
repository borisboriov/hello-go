package db

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func DSN() string {
	if value := os.Getenv("DATABASE_URL"); value != "" {
		return value
	}
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(env("DB_USER", "postgres"), env("DB_PASS", "postgres")),
		Host:   net.JoinHostPort(env("DB_HOST", "localhost"), env("DB_PORT", "5432")),
		Path:   "/" + env("DB_NAME", "cashapp"),
	}
	q := u.Query()
	q.Set("sslmode", "disable")
	u.RawQuery = q.Encode()
	return u.String()
}

func Open(ctx context.Context) (*sql.DB, error) {
	database, err := sql.Open("pgx", DSN())
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL: %w", err)
	}
	database.SetMaxOpenConns(10)
	database.SetMaxIdleConns(5)
	database.SetConnMaxLifetime(30 * time.Minute)
	if err := database.PingContext(ctx); err != nil {
		database.Close()
		return nil, fmt.Errorf("connect to PostgreSQL (check DATABASE_URL or DB_*): %w", err)
	}
	return database, nil
}
