package db

import (
	"net/url"
	"testing"
)

func TestDSN(t *testing.T) {
	for _, key := range []string{"DATABASE_URL", "DB_HOST", "DB_PORT", "DB_USER", "DB_PASS", "DB_NAME"} {
		t.Setenv(key, "")
	}
	if got := DSN(); got != "postgres://postgres:postgres@localhost:5432/cashapp?sslmode=disable" {
		t.Fatal(got)
	}
	t.Setenv("DB_HOST", "::1")
	t.Setenv("DB_PORT", "5433")
	t.Setenv("DB_USER", "user@local")
	t.Setenv("DB_PASS", "a:/?#@ secret")
	t.Setenv("DB_NAME", "custom")
	u, err := url.Parse(DSN())
	if err != nil {
		t.Fatal(err)
	}
	password, _ := u.User.Password()
	if u.Host != "[::1]:5433" || u.User.Username() != "user@local" || password != "a:/?#@ secret" || u.Path != "/custom" {
		t.Fatal("invalid fallback DSN")
	}
	t.Setenv("DATABASE_URL", "postgres://preferred/db?sslmode=require")
	if got := DSN(); got != "postgres://preferred/db?sslmode=require" {
		t.Fatal("DATABASE_URL must take priority")
	}
}
