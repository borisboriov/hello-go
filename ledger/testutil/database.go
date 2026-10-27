// Package testutil создаёт отдельную схему PostgreSQL для каждого теста.
package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/borisboriov/hello-go/ledger/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
)

// Database подготавливает DATABASE_URL; вызывающий код затем открывает Ledger.
// Тесты не очищают таблицы приложения, а удаляют только собственную схему.
func Database(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	schema := fmt.Sprintf("ledger_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Error(err)
		}
	})
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme == "" {
		t.Fatal("TEST_DATABASE_URL must be a PostgreSQL URL")
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	t.Setenv("DATABASE_URL", u.String())
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	t.Setenv("REDIS_ADDR", addr)
	// Отдельная логическая БД для тестов; FLUSHDB не используется.
	t.Setenv("REDIS_DB", "15")
	connection, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close() })
	files, err := migrations.Files.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		raw, err := migrations.Files.ReadFile(file.Name())
		if err != nil {
			t.Fatal(err)
		}
		up := strings.Split(string(raw), "-- +goose Down")[0]
		if _, err := connection.Exec(up); err != nil {
			t.Fatal(err)
		}
	}
	return connection
}

// ReportKey очищает только ключ данного сценария до и после проверки.
func ReportKey(t *testing.T, from, to string) string {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: os.Getenv("REDIS_ADDR"), Password: os.Getenv("REDIS_PASSWORD"), DB: 15})
	key := "report:summary:" + from + ":" + to
	if err := client.Del(context.Background(), key).Err(); err != nil {
		client.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.Del(context.Background(), key).Err(); err != nil {
			t.Error(err)
		}
		client.Close()
	})
	return key
}
