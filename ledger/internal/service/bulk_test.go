package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/borisboriov/hello-go/ledger/internal/domain"
)

type importRunner func(context.Context, func(domain.Repositories) error) error

func (run importRunner) WithinTransaction(ctx context.Context, fn func(domain.Repositories) error) error {
	return run(ctx, fn)
}

func TestImportTransactionsMixedResults(t *testing.T) {
	store := &testStore{budget: domain.Budget{Category: "food", Limit: 100}}
	var mu sync.Mutex
	runner := importRunner(func(_ context.Context, fn func(domain.Repositories) error) error {
		mu.Lock()
		defer mu.Unlock()
		return fn(domain.Repositories{Budgets: store, Expenses: store})
	})
	svc := New(domain.Repositories{}, runner, nil, nil, nil)
	input := []domain.Transaction{
		{Amount: 50, Category: "food"},
		{Amount: -1, Category: "food"},
		{Amount: 101, Category: "food"},
		{Amount: 45, Category: "food"},
	}
	summary, err := svc.ImportTransactions(context.Background(), input, 3)
	if err != nil || summary.Accepted != 2 || summary.Rejected != 2 || len(summary.Errors) != 2 || len(store.saved) != 2 {
		t.Fatalf("summary=%+v, saved=%+v, err=%v", summary, store.saved, err)
	}
	var invalid *domain.ValidationError
	if got := summary.Errors[0]; got.Index != 1 || !errors.As(got.Err, &invalid) {
		t.Fatalf("expected validation error for row 1, got %+v", got)
	}
	if got := summary.Errors[1]; got.Index != 2 || !errors.Is(got.Err, domain.ErrBudgetExceeded) {
		t.Fatalf("expected budget error for row 2, got %+v", got)
	}
}

func TestImportTransactionsCancellationStopsWorkers(t *testing.T) {
	const expectedWorkers = 4
	input := make([]domain.Transaction, 10)
	for i := range input {
		input[i] = domain.Transaction{Amount: 1, Category: "food"}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, len(input))
	runner := importRunner(func(ctx context.Context, _ func(domain.Repositories) error) error {
		started <- struct{}{}
		<-ctx.Done()
		return ctx.Err()
	})
	svc := New(domain.Repositories{}, runner, nil, nil, nil)
	type result struct {
		summary domain.BulkImportSummary
		err     error
	}
	done := make(chan result, 1)
	go func() {
		summary, err := svc.ImportTransactions(ctx, input, 0)
		done <- result{summary, err}
	}()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for range expectedWorkers {
		select {
		case <-started:
		case <-timer.C:
			t.Fatal("the default workers did not start concurrently")
		}
	}
	cancel()
	select {
	case got := <-done:
		if !errors.Is(got.err, context.Canceled) || got.summary.Accepted != 0 || got.summary.Rejected != expectedWorkers || len(got.summary.Errors) != expectedWorkers {
			t.Fatalf("expected partial result on cancellation, got %+v", got)
		}
		for i, failure := range got.summary.Errors {
			if failure.Index != i || !errors.Is(failure.Err, context.Canceled) {
				t.Fatalf("expected canceled row %d, got %+v", i, failure)
			}
		}
	case <-timer.C:
		t.Fatal("import did not terminate after cancellation")
	}
	if len(started) != 0 {
		t.Fatal("more than four storage calls started")
	}
}

func TestImportTransactionsKeepsCommittedExpenseOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := &testStore{findErr: domain.ErrNotFound}
	runner := importRunner(func(_ context.Context, fn func(domain.Repositories) error) error {
		err := fn(domain.Repositories{Budgets: store, Expenses: store})
		cancel()
		return err
	})
	svc := New(domain.Repositories{}, runner, nil, nil, nil)
	tx := domain.Transaction{Amount: 1, Category: "food"}
	summary, err := svc.ImportTransactions(ctx, []domain.Transaction{tx, tx}, 1)
	if !errors.Is(err, context.Canceled) || summary.Accepted != 1 || summary.Rejected != 0 || len(summary.Errors) != 0 || len(store.saved) != 1 {
		t.Fatalf("summary=%+v, saved=%+v, err=%v", summary, store.saved, err)
	}
}
