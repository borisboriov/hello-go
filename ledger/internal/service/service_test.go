package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/borisboriov/hello-go/ledger/internal/domain"
)

type testStore struct {
	domain.BudgetRepository
	domain.ExpenseRepository
	budget    domain.Budget
	spent     int64
	findErr   error
	sumErr    error
	insertErr error
	saved     []domain.Transaction
}

func (s *testStore) FindBudget(context.Context, string) (domain.Budget, error) {
	return s.budget, s.findErr
}

func (s *testStore) SumByCategory(context.Context, string) (int64, error) { return s.spent, s.sumErr }

func (s *testStore) InsertTransaction(_ context.Context, tx domain.Transaction) (domain.Transaction, error) {
	if s.insertErr != nil {
		return domain.Transaction{}, s.insertErr
	}
	tx.ID = len(s.saved) + 1
	s.saved = append(s.saved, tx)
	return tx, nil
}

type testTransactions struct {
	store *testStore
	calls int
	err   error
}

func (t *testTransactions) WithinTransaction(_ context.Context, fn func(domain.Repositories) error) error {
	t.calls++
	if t.err != nil {
		return t.err
	}
	return fn(domain.Repositories{Budgets: t.store, Expenses: t.store})
}

func TestCreateTransaction(t *testing.T) {
	storageErr := errors.New("storage unavailable")
	for _, tt := range []struct {
		name           string
		amount         int
		spent          int64
		findErr        error
		sumErr         error
		insertErr      error
		transactionErr error
		wantErr        error
		invalid        bool
	}{
		{name: "exact limit", amount: 50, spent: 50},
		{name: "exceeded", amount: 51, spent: 50, wantErr: domain.ErrBudgetExceeded},
		{name: "without budget", amount: 200, findErr: domain.ErrNotFound},
		{name: "invalid amount", invalid: true},
		{name: "budget read failed", amount: 1, findErr: storageErr, wantErr: storageErr},
		{name: "sum failed", amount: 1, sumErr: storageErr, wantErr: storageErr},
		{name: "insert failed", amount: 1, insertErr: storageErr, wantErr: storageErr},
		{name: "transaction failed", amount: 1, transactionErr: storageErr, wantErr: storageErr},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := &testStore{budget: domain.Budget{Category: "food", Limit: 100}, spent: tt.spent, findErr: tt.findErr, sumErr: tt.sumErr, insertErr: tt.insertErr}
			transactions := &testTransactions{store: store, err: tt.transactionErr}
			svc := New(domain.Repositories{Budgets: store, Expenses: store}, transactions, nil, nil)
			before := time.Now().UTC().Format(time.DateOnly)
			got, err := svc.CreateTransaction(context.Background(), domain.Transaction{Category: "food", Amount: tt.amount})
			if tt.invalid {
				var validation *domain.ValidationError
				if !errors.As(err, &validation) || transactions.calls != 0 {
					t.Fatalf("validation must run before storage: %v, calls=%d", err, transactions.calls)
				}
			} else if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error=%v, want %v", err, tt.wantErr)
			}
			if tt.invalid || tt.wantErr != nil {
				if len(store.saved) != 0 || got != (domain.Transaction{}) {
					t.Fatalf("rejected expense was returned or saved: %+v", got)
				}
				return
			}
			if got.ID != 1 || got.Amount != tt.amount || (got.Date != before && got.Date != time.Now().UTC().Format(time.DateOnly)) {
				t.Fatalf("created expense: %+v", got)
			}
		})
	}
}

type testReports struct {
	calls  int
	result domain.ReportSummary
}

func (r *testReports) GetReportSummary(context.Context, string, string) (domain.ReportSummary, error) {
	r.calls++
	return r.result, nil
}

type brokenCache struct{}

func (brokenCache) Get(context.Context, string) (domain.ReportSummary, error) {
	return domain.ReportSummary{}, errors.New("cache unavailable")
}

func (brokenCache) Set(context.Context, string, domain.ReportSummary) error {
	return errors.New("cache unavailable")
}

func TestReportWithoutCache(t *testing.T) {
	reports := &testReports{result: domain.ReportSummary{From: "2026-01-01", To: "2026-01-31", Total: 123, Categories: []domain.CategoryTotal{{Category: "food", Amount: 123}}}}
	svc := New(domain.Repositories{}, nil, reports, brokenCache{})
	got, err := svc.GetReportSummary(context.Background(), reports.result.From, reports.result.To)
	if err != nil || got.Total != 123 || reports.calls != 1 {
		t.Fatalf("cache failure blocked report: %+v, %v", got, err)
	}
}

func TestInvalidBudgetBeforeStorage(t *testing.T) {
	svc := New(domain.Repositories{}, nil, nil, nil)
	var validation *domain.ValidationError
	if err := svc.SetBudget(context.Background(), domain.Budget{Category: "food"}); !errors.As(err, &validation) {
		t.Fatalf("expected validation error: %v", err)
	}
}
