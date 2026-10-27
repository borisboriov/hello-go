package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/borisboriov/hello-go/ledger/internal/cache"
	"github.com/borisboriov/hello-go/ledger/internal/db"
	"github.com/redis/go-redis/v9"
)

// Соединения инициализируются до запуска обработчиков и закрываются после их остановки.
var database *sql.DB
var reportCache *redis.Client

var ErrNotInitialized = errors.New("Ledger is not initialized: call Open before serving requests")

// Open подключает PostgreSQL и Redis. Не вызывается одновременно с операциями Ledger.
func Open(ctx context.Context) error {
	if database != nil {
		return errors.New("Ledger is already initialized")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	connection, err := db.Open(ctx)
	if err != nil {
		return err
	}
	client, err := cache.Open(ctx)
	if err != nil {
		connection.Close()
		return err
	}
	database, reportCache = connection, client
	log.Println("Ledger: connected to PostgreSQL (pool: open=10 idle=5)")
	log.Println("Ledger: connected to Redis")
	return nil
}

// Close вызывается после завершения всех запросов.
func Close() error {
	var dbErr, cacheErr error
	if reportCache != nil {
		cacheErr = reportCache.Close()
		reportCache = nil
	}
	if database != nil {
		dbErr = database.Close()
		database = nil
	}
	return errors.Join(dbErr, cacheErr)
}

func operationContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

// money хранит копейки как десятичную строку рублей без потери точности float64.
func money(cents int) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}
