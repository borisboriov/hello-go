package service

import (
	"bytes"
	"context"
	"errors"
	"log"
	"reflect"
	"strings"
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
	budgets   []domain.Budget
	setErr    error
	listErr   error
	setCalls  int
	listCalls int
}

func (s *testStore) SetBudget(_ context.Context, b domain.Budget) error {
	s.setCalls++
	if s.setErr != nil {
		return s.setErr
	}
	s.budgets = []domain.Budget{b}
	return nil
}

func (s *testStore) ListBudgets(context.Context) ([]domain.Budget, error) {
	s.listCalls++
	return s.budgets, s.listErr
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
			svc := New(domain.Repositories{Budgets: store, Expenses: store}, transactions, nil, nil, nil)
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
	svc := New(domain.Repositories{}, nil, reports, brokenCache{}, nil)
	got, err := svc.GetReportSummary(context.Background(), reports.result.From, reports.result.To)
	if err != nil || got.Total != 123 || reports.calls != 1 {
		t.Fatalf("cache failure blocked report: %+v, %v", got, err)
	}
}

func TestInvalidBudgetBeforeStorage(t *testing.T) {
	svc := New(domain.Repositories{}, nil, nil, nil, nil)
	var validation *domain.ValidationError
	if err := svc.SetBudget(context.Background(), domain.Budget{Category: "food"}); !errors.As(err, &validation) {
		t.Fatalf("expected validation error: %v", err)
	}
}

type fakeBudgetCache struct {
	value                  []domain.Budget
	getErr, setErr, delErr error
	gets, sets, dels       int
}

func (c *fakeBudgetCache) Get(_ context.Context, key string) ([]domain.Budget, error) {
	c.gets++
	if c.getErr != nil {
		return nil, c.getErr
	}
	if c.value == nil {
		return nil, domain.ErrCacheMiss
	}
	return c.value, nil
}

func (c *fakeBudgetCache) Set(_ context.Context, key string, budgets []domain.Budget) error {
	c.sets++
	if c.setErr != nil {
		return c.setErr
	}
	c.value = budgets
	return nil
}

func (c *fakeBudgetCache) Delete(context.Context, string) error {
	c.dels++
	if c.delErr != nil {
		return c.delErr
	}
	c.value = nil
	return nil
}

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })
	return &output
}

func TestListBudgetsCache(t *testing.T) {
	output := captureLog(t)
	store := &testStore{budgets: []domain.Budget{{Category: "food", Limit: 100}}}
	cache := &fakeBudgetCache{}
	svc := New(domain.Repositories{Budgets: store}, nil, nil, nil, cache)

	first, err := svc.ListBudgets(context.Background())
	if err != nil || store.listCalls != 1 || cache.sets != 1 {
		t.Fatalf("miss must read the repository and fill the cache: %v, repo=%d, sets=%d", err, store.listCalls, cache.sets)
	}
	second, err := svc.ListBudgets(context.Background())
	if err != nil || store.listCalls != 1 || !reflect.DeepEqual(first, second) {
		t.Fatalf("hit must not touch the repository: %v, repo=%d, %+v", err, store.listCalls, second)
	}
	for _, line := range []string{"budgets cache miss key=budgets:all", "budgets cache hit key=budgets:all"} {
		if !strings.Contains(output.String(), line) {
			t.Fatalf("log must contain %q: %s", line, output)
		}
	}
	if strings.Index(output.String(), "cache miss") > strings.Index(output.String(), "cache hit") {
		t.Fatalf("miss must be logged before hit: %s", output)
	}
}

func TestListBudgetsCachesEmptyList(t *testing.T) {
	store := &testStore{budgets: []domain.Budget{}}
	cache := &fakeBudgetCache{}
	svc := New(domain.Repositories{Budgets: store}, nil, nil, nil, cache)
	for i := 0; i < 2; i++ {
		got, err := svc.ListBudgets(context.Background())
		if err != nil || got == nil || len(got) != 0 {
			t.Fatalf("call %d: empty list must stay [] and not null: %#v, %v", i, got, err)
		}
	}
	if store.listCalls != 1 {
		t.Fatalf("empty list must be served from cache, repo calls=%d", store.listCalls)
	}
}

func TestListBudgetsNullInCacheIsMiss(t *testing.T) {
	store := &testStore{budgets: []domain.Budget{{Category: "food", Limit: 100}}}
	cache := &fakeBudgetCache{value: nil}
	svc := New(domain.Repositories{Budgets: store}, nil, nil, nil, cache)
	got, err := svc.ListBudgets(context.Background())
	if err != nil || len(got) != 1 || store.listCalls != 1 {
		t.Fatalf("got %+v, %v, repo=%d", got, err, store.listCalls)
	}
}

func TestListBudgetsBrokenCache(t *testing.T) {
	output := captureLog(t)
	store := &testStore{budgets: []domain.Budget{{Category: "food", Limit: 100}}}
	cache := &fakeBudgetCache{getErr: errors.New("cache unavailable"), setErr: errors.New("cache unavailable")}
	svc := New(domain.Repositories{Budgets: store}, nil, nil, nil, cache)
	got, err := svc.ListBudgets(context.Background())
	if err != nil || len(got) != 1 || store.listCalls != 1 {
		t.Fatalf("cache failure blocked budgets: %+v, %v", got, err)
	}
	if !strings.Contains(output.String(), "budgets cache read failed") || !strings.Contains(output.String(), "budgets cache write failed") {
		t.Fatalf("cache failures must be logged: %s", output)
	}
}

func TestListBudgetsStorageErrorIsNotCached(t *testing.T) {
	storageErr := errors.New("storage unavailable")
	store := &testStore{listErr: storageErr}
	cache := &fakeBudgetCache{}
	svc := New(domain.Repositories{Budgets: store}, nil, nil, nil, cache)
	if _, err := svc.ListBudgets(context.Background()); !errors.Is(err, storageErr) || cache.sets != 0 {
		t.Fatalf("error=%v, sets=%d", err, cache.sets)
	}
}

func TestSetBudgetInvalidatesCache(t *testing.T) {
	storageErr := errors.New("storage unavailable")
	for _, tt := range []struct {
		name     string
		budget   domain.Budget
		setErr   error
		delErr   error
		wantErr  error
		wantDels int
	}{
		{name: "success", budget: domain.Budget{Category: "food", Limit: 100}, wantDels: 1},
		{name: "invalid budget", budget: domain.Budget{Category: "food"}, wantErr: &domain.ValidationError{}},
		{name: "storage failed", budget: domain.Budget{Category: "food", Limit: 100}, setErr: storageErr, wantErr: storageErr},
		{name: "invalidation failed", budget: domain.Budget{Category: "food", Limit: 100}, delErr: errors.New("cache unavailable"), wantDels: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := &testStore{setErr: tt.setErr}
			cache := &fakeBudgetCache{value: []domain.Budget{{Category: "old", Limit: 1}}, delErr: tt.delErr}
			svc := New(domain.Repositories{Budgets: store}, nil, nil, nil, cache)
			err := svc.SetBudget(context.Background(), tt.budget)
			var validation *domain.ValidationError
			switch {
			case errors.As(tt.wantErr, &validation):
				if !errors.As(err, &validation) {
					t.Fatalf("expected validation error: %v", err)
				}
			case !errors.Is(err, tt.wantErr):
				t.Fatalf("error=%v, want %v", err, tt.wantErr)
			}
			if cache.dels != tt.wantDels {
				t.Fatalf("deletes=%d, want %d", cache.dels, tt.wantDels)
			}
		})
	}
}

func TestSetBudgetMakesNextListFresh(t *testing.T) {
	store := &testStore{budgets: []domain.Budget{{Category: "food", Limit: 100}}}
	svc := New(domain.Repositories{Budgets: store}, nil, nil, nil, &fakeBudgetCache{})
	if _, err := svc.ListBudgets(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetBudget(context.Background(), domain.Budget{Category: "food", Limit: 200}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.ListBudgets(context.Background())
	if err != nil || !reflect.DeepEqual(got, []domain.Budget{{Category: "food", Limit: 200}}) || store.listCalls != 2 {
		t.Fatalf("list after SetBudget must come from the repository: %+v, %v, repo=%d", got, err, store.listCalls)
	}
}
