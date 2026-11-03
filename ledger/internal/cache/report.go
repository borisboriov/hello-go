package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/borisboriov/hello-go/ledger/internal/domain"
	"github.com/redis/go-redis/v9"
)

const ReportTTL = 30 * time.Second

type Reports struct{ client *redis.Client }

func NewReports(client *redis.Client) *Reports { return &Reports{client: client} }

var _ domain.ReportCache = (*Reports)(nil)

func (c *Reports) Get(ctx context.Context, key string) (domain.ReportSummary, error) {
	raw, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		return domain.ReportSummary{}, err
	}
	var result domain.ReportSummary
	err = json.Unmarshal(raw, &result)
	return result, err
}

func (c *Reports) Set(ctx context.Context, key string, result domain.ReportSummary) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, raw, ReportTTL).Err()
}
