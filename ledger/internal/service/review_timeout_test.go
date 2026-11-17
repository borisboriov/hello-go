package service

import (
	"context"
	"errors"
	"github.com/borisboriov/hello-go/ledger/internal/domain"
	"testing"
	"time"
)

type deadlineReportCache struct{}

func (deadlineReportCache) Get(context.Context, string) (domain.ReportSummary, error) {
	return domain.ReportSummary{}, domain.ErrCacheMiss
}
func (deadlineReportCache) Set(ctx context.Context, _ string, _ domain.ReportSummary) error {
	<-ctx.Done()
	return ctx.Err()
}

type deadlineBudgetCache struct{}

func (deadlineBudgetCache) Get(context.Context, string) ([]domain.Budget, error) {
	return nil, domain.ErrCacheMiss
}
func (deadlineBudgetCache) Set(ctx context.Context, _ string, _ []domain.Budget) error {
	<-ctx.Done()
	return ctx.Err()
}
func (deadlineBudgetCache) Delete(ctx context.Context, _ string) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestReviewDeadlineDuringReportCacheWrite(t *testing.T) {
	reports := &testReports{result: domain.ReportSummary{Categories: []domain.CategoryTotal{{Category: "food", Amount: 123}}}}
	svc := New(domain.Repositories{}, nil, reports, deadlineReportCache{}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	got, err := svc.GetReportSummary(ctx, "2026-01-01", "2026-01-31")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ctx=%v but error=%v, total=%d: deadline swallowed", ctx.Err(), err, got.Total)
	}
}

func TestReviewDeadlineDuringBudgetCacheWrite(t *testing.T) {
	store := &testStore{budgets: []domain.Budget{{Category: "food", Limit: 100}}}
	svc := New(domain.Repositories{Budgets: store}, nil, nil, nil, deadlineBudgetCache{})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	got, err := svc.ListBudgets(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ctx=%v but error=%v, budgets=%v: deadline swallowed", ctx.Err(), err, got)
	}
}
