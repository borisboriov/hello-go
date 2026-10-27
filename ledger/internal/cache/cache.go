package cache

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

func Open(ctx context.Context) (*redis.Client, error) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	number := 0
	if value := os.Getenv("REDIS_DB"); value != "" {
		var err error
		number, err = strconv.Atoi(value)
		if err != nil || number < 0 {
			return nil, fmt.Errorf("REDIS_DB must be a non-negative integer")
		}
	}
	client := redis.NewClient(&redis.Options{
		Addr: addr, DB: number, Password: os.Getenv("REDIS_PASSWORD"),
		DialTimeout: 2 * time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second,
		MaxRetries: -1, ContextTimeoutEnabled: true,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("connect to Redis (check REDIS_*): %w", err)
	}
	return client, nil
}
