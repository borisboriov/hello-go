package ledger

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/borisboriov/hello-go/ledger/testutil"
)

func TestConcurrentBudgetLimit(t *testing.T) {
	resetLedger(t)
	if err := SetBudget(Budget{Category: "parallel", Limit: 10}); err != nil {
		t.Fatal(err)
	}
	var accepted, rejected atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := AddTransaction(Transaction{Category: "parallel", Amount: 1})
			if err == nil {
				accepted.Add(1)
			} else if errors.Is(err, ErrBudgetExceeded) {
				rejected.Add(1)
			} else {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 10 || rejected.Load() != 10 || len(mustTransactions(t)) != 10 {
		t.Fatalf("accepted=%d rejected=%d", accepted.Load(), rejected.Load())
	}
}

func TestPersistenceAndOrdering(t *testing.T) {
	resetLedger(t)
	if err := SetBudget(Budget{Category: "travel", Limit: 12345}); err != nil {
		t.Fatal(err)
	}
	for _, tx := range []Transaction{
		{Amount: 101, Category: "travel", Date: "2026-10-02"},
		{Amount: 202, Category: "travel", Date: "2026-10-01"},
		{Amount: 303, Category: "travel", Date: "2026-10-02"},
	} {
		if err := AddTransaction(tx); err != nil {
			t.Fatal(err)
		}
	}
	before := mustTransactions(t)
	if before[0].ID != 3 || before[1].ID != 1 || before[2].ID != 2 {
		t.Fatalf("order: %+v", before)
	}
	if err := Close(); err != nil {
		t.Fatal(err)
	}
	if err := Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := mustTransactions(t); !reflect.DeepEqual(got, before) {
		t.Fatalf("lost transactions: %+v", got)
	}
	if got := mustBudgets(t); len(got) != 1 || got[0].Limit != 12345 {
		t.Fatalf("lost budgets: %+v", got)
	}
}

func TestReportCache(t *testing.T) {
	resetLedger(t)
	ctx := context.Background()
	from, to := "2041-01-01", "2041-01-31"
	key := testutil.ReportKey(t, from, to)
	for _, tx := range []Transaction{
		{Amount: 101, Category: "a", Date: from},
		{Amount: 202, Category: "b", Date: to},
		{Amount: 999, Category: "a", Date: "2041-02-01"},
	} {
		if err := AddTransaction(tx); err != nil {
			t.Fatal(err)
		}
	}
	first, err := GetReportSummary(ctx, from, to)
	if err != nil {
		t.Fatal(err)
	}
	want := ReportSummary{From: from, To: to, Total: 303, Categories: []CategoryTotal{{"a", 101}, {"b", 202}}}
	if !reflect.DeepEqual(first, want) {
		t.Fatalf("report=%+v", first)
	}
	ttl, err := reportCache.TTL(ctx, key).Result()
	if err != nil || ttl <= 0 || ttl > 30*time.Second {
		t.Fatalf("ttl=%s err=%v", ttl, err)
	}
	if err := AddTransaction(Transaction{Amount: 1, Category: "a", Date: to}); err != nil {
		t.Fatal(err)
	}
	hit, err := GetReportSummary(ctx, from, to)
	if err != nil || !reflect.DeepEqual(hit, first) {
		t.Fatalf("expected cached snapshot, got %+v, %v", hit, err)
	}
	// Истечение ключа должно привести к пересчёту из БД.
	if err := reportCache.PExpire(ctx, key, time.Millisecond).Err(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	fresh, err := GetReportSummary(ctx, from, to)
	if err != nil || fresh.Total != 304 {
		t.Fatalf("after expiration: %+v, %v", fresh, err)
	}
	if err := reportCache.Set(ctx, key, "{broken", ReportTTL).Err(); err != nil {
		t.Fatal(err)
	}
	fresh, err = GetReportSummary(ctx, from, to)
	if err != nil || fresh.Total != 304 {
		t.Fatalf("corrupt cache fallback: %+v, %v", fresh, err)
	}
	if err := reportCache.Close(); err != nil {
		t.Fatal(err)
	}
	// При недоступном кеше отчёт остаётся доступен через PostgreSQL.
	fresh, err = GetReportSummary(ctx, from, to)
	if err != nil || fresh.Total != 304 {
		t.Fatalf("unavailable cache fallback: %+v, %v", fresh, err)
	}
	// Закрытый клиент больше не участвует в Cleanup/Close.
	reportCache = nil
}

func TestInvalidReportDates(t *testing.T) {
	for _, dates := range [][2]string{{"", ""}, {"2026-02-30", "2026-03-01"}, {"2026-02-02", "2026-02-01"}} {
		if _, err := GetReportSummary(context.Background(), dates[0], dates[1]); !errors.Is(err, ErrInvalidReportDates) {
			t.Fatalf("dates=%v: %v", dates, err)
		}
	}
}

func TestUninitializedStorage(t *testing.T) {
	if _, err := ListBudgets(); !errors.Is(err, ErrNotInitialized) {
		t.Fatal(err)
	}
	if _, err := ListTransactions(); !errors.Is(err, ErrNotInitialized) {
		t.Fatal(err)
	}
	if err := SetBudget(Budget{Category: "a", Limit: 1}); !errors.Is(err, ErrNotInitialized) {
		t.Fatal(err)
	}
	if err := AddTransaction(Transaction{Category: "a", Amount: 1}); !errors.Is(err, ErrNotInitialized) {
		t.Fatal(err)
	}
}
