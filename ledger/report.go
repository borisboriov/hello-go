package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

const ReportTTL = 30 * time.Second

type CategoryTotal struct {
	Category string `json:"category"`
	Amount   int64  `json:"amount"`
}

type ReportSummary struct {
	From       string          `json:"from"`
	To         string          `json:"to"`
	Total      int64           `json:"total"`
	Categories []CategoryTotal `json:"categories"`
}

var ErrInvalidReportDates = errors.New("from and to must be YYYY-MM-DD with from <= to")

// GetReportSummary возвращает суммы в копейках, включая обе границы интервала.
// Ошибки кеша не мешают чтению из PostgreSQL. Допустимая задержка актуальности — TTL.
func GetReportSummary(ctx context.Context, from, to string) (ReportSummary, error) {
	start, errFrom := time.Parse(time.DateOnly, from)
	end, errTo := time.Parse(time.DateOnly, to)
	if errFrom != nil || errTo != nil || start.Year() < 1 || end.Year() < 1 || start.After(end) {
		return ReportSummary{}, ErrInvalidReportDates
	}
	if database == nil {
		return ReportSummary{}, ErrNotInitialized
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	key := "report:summary:" + from + ":" + to
	if reportCache != nil {
		raw, err := reportCache.Get(ctx, key).Bytes()
		if err == nil {
			var result ReportSummary
			if json.Unmarshal(raw, &result) == nil && result.From == from && result.To == to && result.Categories != nil {
				log.Printf("report cache hit key=%s", key)
				return result, nil
			}
			log.Printf("report cache invalid JSON key=%s", key)
		} else if !errors.Is(err, redis.Nil) {
			log.Printf("report cache read failed: %v", err)
		}
	}
	log.Printf("report cache miss key=%s", key)
	rows, err := database.QueryContext(ctx, `SELECT category, (SUM(amount)*100)::bigint
        FROM expenses WHERE date >= $1::date AND date <= $2::date
        GROUP BY category ORDER BY category`, from, to)
	if err != nil {
		return ReportSummary{}, fmt.Errorf("query report: %w", err)
	}
	defer rows.Close()
	result := ReportSummary{From: from, To: to, Categories: make([]CategoryTotal, 0)}
	for rows.Next() {
		var item CategoryTotal
		if err := rows.Scan(&item.Category, &item.Amount); err != nil {
			return ReportSummary{}, fmt.Errorf("scan report: %w", err)
		}
		result.Total += item.Amount
		result.Categories = append(result.Categories, item)
	}
	if err := rows.Err(); err != nil {
		return ReportSummary{}, fmt.Errorf("read report: %w", err)
	}
	if reportCache != nil {
		raw, err := json.Marshal(result)
		if err == nil {
			err = reportCache.Set(ctx, key, raw, ReportTTL).Err()
		}
		if err != nil {
			log.Printf("report cache write failed: %v", err)
		}
	}
	return result, nil
}
