package service

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/borisboriov/hello-go/ledger/internal/domain"
)

// slowReports имитирует медленную БД: каждый запрос суммы ждёт delay или отмены ctx.
type slowReports struct {
	categories []string
	delay      time.Duration
	active     atomic.Int32
	maxActive  atomic.Int32
}

func (r *slowReports) ReportCategories(context.Context, string, string) ([]string, error) {
	return r.categories, nil
}

func (r *slowReports) SumByCategoryPeriod(ctx context.Context, category, _, _ string) (int64, error) {
	n := r.active.Add(1)
	defer r.active.Add(-1)
	for {
		m := r.maxActive.Load()
		if n <= m || r.maxActive.CompareAndSwap(m, n) {
			break
		}
	}
	select {
	case <-time.After(r.delay):
		return int64(len(category)) * 100, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func TestReportRunsCategoriesInParallel(t *testing.T) {
	repo := &slowReports{categories: []string{"еда", "транспорт", "кафе"}, delay: 100 * time.Millisecond}
	svc := New(domain.Repositories{}, nil, repo, nil, nil)
	started := time.Now()
	got, err := svc.GetReportSummary(context.Background(), "2026-01-01", "2026-01-31")
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("categories were computed sequentially: %s", elapsed)
	}
	if repo.maxActive.Load() != 3 || len(got.Categories) != 3 || got.Categories[0].Category != "еда" {
		t.Fatalf("report: %+v, max parallel=%d", got, repo.maxActive.Load())
	}
}

func TestReportStopsOnContextTimeout(t *testing.T) {
	before := runtime.NumGoroutine()
	repo := &slowReports{categories: []string{"a", "b", "c"}, delay: 5 * time.Second}
	svc := New(domain.Repositories{}, nil, repo, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := svc.GetReportSummary(ctx, "2026-01-01", "2026-01-31")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("workers did not stop on ctx.Done")
	}
	time.Sleep(50 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before {
		t.Fatalf("goroutines leaked: before=%d after=%d", before, after)
	}
}
