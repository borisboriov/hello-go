package ledger

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/borisboriov/hello-go/ledger/internal/cache"
	"github.com/borisboriov/hello-go/ledger/internal/db"
	"github.com/borisboriov/hello-go/ledger/internal/domain"
	"github.com/borisboriov/hello-go/ledger/internal/repository/pg"
	"github.com/borisboriov/hello-go/ledger/internal/service"
)

// Open собирает сервис. Миграции применяются отдельно до запуска приложения.
func Open(ctx context.Context) (Service, func() error, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	connection, err := db.Open(ctx)
	if err != nil {
		return nil, nil, err
	}
	client, err := cache.Open(ctx)
	if err != nil {
		connection.Close()
		return nil, nil, err
	}
	store := pg.New(connection)
	svc := service.New(domain.Repositories{Budgets: store, Expenses: store}, pg.NewTransactions(connection), store, cache.NewReports(client))
	closeFn := sync.OnceValue(func() error { return errors.Join(client.Close(), connection.Close()) })
	log.Println("Ledger: connected to PostgreSQL (pool: open=10 idle=5)")
	log.Println("Ledger: connected to Redis")
	return svc, closeFn, nil
}
