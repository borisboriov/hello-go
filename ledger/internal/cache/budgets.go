package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/borisboriov/hello-go/ledger/internal/domain"
	"github.com/redis/go-redis/v9"
)

const BudgetsTTL = 30 * time.Second

type Budgets struct{ client *redis.Client }

func NewBudgets(client *redis.Client) *Budgets { return &Budgets{client: client} }

var _ domain.BudgetCache = (*Budgets)(nil)

func (c *Budgets) Get(ctx context.Context, key string) ([]domain.Budget, error) {
	raw, err := c.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, domain.ErrCacheMiss
	}
	if err != nil {
		return nil, err
	}
	var result []domain.Budget
	err = json.Unmarshal(raw, &result)
	return result, err
}

func (c *Budgets) Set(ctx context.Context, key string, budgets []domain.Budget) error {
	raw, err := json.Marshal(budgets)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, raw, BudgetsTTL).Err()
}

func (c *Budgets) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}
