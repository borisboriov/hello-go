package ledger

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/borisboriov/hello-go/ledger/internal/cache"
	"github.com/borisboriov/hello-go/ledger/internal/domain"
	"github.com/borisboriov/hello-go/ledger/internal/service"
	"github.com/borisboriov/hello-go/ledger/testutil"
)

func TestConcurrentBudgetLimit(t *testing.T) {
	svc := resetLedger(t)
	if err := svc.SetBudget(context.Background(), Budget{Category: "parallel", Limit: 10}); err != nil {
		t.Fatal(err)
	}
	var accepted, rejected atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := svc.AddTransaction(context.Background(), Transaction{Category: "parallel", Amount: 1})
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
	if accepted.Load() != 10 || rejected.Load() != 10 || len(mustTransactions(t, svc)) != 10 {
		t.Fatalf("accepted=%d rejected=%d", accepted.Load(), rejected.Load())
	}
}

func TestPersistenceAndOrdering(t *testing.T) {
	testutil.Database(t)
	svc, closeFn, err := Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeFn() })
	if err := svc.SetBudget(context.Background(), Budget{Category: "travel", Limit: 12345}); err != nil {
		t.Fatal(err)
	}
	for _, tx := range []Transaction{
		{Amount: 101, Category: "travel", Date: "2026-10-02"},
		{Amount: 202, Category: "travel", Date: "2026-10-01"},
		{Amount: 303, Category: "travel", Date: "2026-10-02"},
	} {
		if err := svc.AddTransaction(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
	}
	before := mustTransactions(t, svc)
	if before[0].ID != 3 || before[1].ID != 1 || before[2].ID != 2 {
		t.Fatalf("order: %+v", before)
	}
	if err := closeFn(); err != nil {
		t.Fatal(err)
	}
	svc, closeFn, err = Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := mustTransactions(t, svc); !reflect.DeepEqual(got, before) {
		t.Fatalf("lost transactions: %+v", got)
	}
	if got := mustBudgets(t, svc); len(got) != 1 || got[0].Limit != 12345 {
		t.Fatalf("lost budgets: %+v", got)
	}
}

func TestReportCache(t *testing.T) {
	svc := resetLedger(t)
	ctx := context.Background()
	reportCache, err := cache.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reportCache.Close() })
	from, to := "2041-01-01", "2041-01-31"
	key := testutil.ReportKey(t, from, to)
	for _, tx := range []Transaction{
		{Amount: 101, Category: "a", Date: from},
		{Amount: 202, Category: "b", Date: to},
		{Amount: 999, Category: "a", Date: "2041-02-01"},
	} {
		if err := svc.AddTransaction(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
	}
	first, err := svc.GetReportSummary(ctx, from, to)
	if err != nil {
		t.Fatal(err)
	}
	want := ReportSummary{From: from, To: to, Total: 303, Categories: []CategoryTotal{{Category: "a", Amount: 101}, {Category: "b", Amount: 202}}}
	if !reflect.DeepEqual(first, want) {
		t.Fatalf("report=%+v", first)
	}
	ttl, err := reportCache.TTL(ctx, key).Result()
	if err != nil || ttl <= 0 || ttl > 30*time.Second {
		t.Fatalf("ttl=%s err=%v", ttl, err)
	}
	if err := svc.AddTransaction(context.Background(), Transaction{Amount: 1, Category: "a", Date: to}); err != nil {
		t.Fatal(err)
	}
	hit, err := svc.GetReportSummary(ctx, from, to)
	if err != nil || !reflect.DeepEqual(hit, first) {
		t.Fatalf("expected cached snapshot, got %+v, %v", hit, err)
	}
	// Истечение ключа должно привести к пересчёту из БД.
	if err := reportCache.PExpire(ctx, key, time.Millisecond).Err(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	fresh, err := svc.GetReportSummary(ctx, from, to)
	if err != nil || fresh.Total != 304 {
		t.Fatalf("after expiration: %+v, %v", fresh, err)
	}
	if err := reportCache.Set(ctx, key, "{broken", cache.ReportTTL).Err(); err != nil {
		t.Fatal(err)
	}
	fresh, err = svc.GetReportSummary(ctx, from, to)
	if err != nil || fresh.Total != 304 {
		t.Fatalf("corrupt cache fallback: %+v, %v", fresh, err)
	}
}

func TestBudgetsCache(t *testing.T) {
	svc := resetLedger(t)
	ctx := context.Background()
	client, err := cache.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	key := testutil.BudgetsKey
	exists := func() bool {
		n, err := client.Exists(ctx, key).Result()
		if err != nil {
			t.Fatal(err)
		}
		return n == 1
	}
	if err := svc.SetBudget(ctx, Budget{Category: "b", Limit: 200}); err != nil {
		t.Fatal(err)
	}
	if exists() {
		t.Fatal("SetBudget must leave no stale budgets:all")
	}
	// Первый вызов — промах: список читается из БД и сохраняется с TTL.
	first, err := svc.ListBudgets(ctx)
	if err != nil || !reflect.DeepEqual(first, []Budget{{Category: "b", Limit: 200}}) {
		t.Fatalf("budgets=%+v, %v", first, err)
	}
	ttl, err := client.TTL(ctx, key).Result()
	if err != nil || ttl <= 0 || ttl > cache.BudgetsTTL {
		t.Fatalf("ttl=%s err=%v", ttl, err)
	}
	// Подмена значения в Redis доказывает, что второй вызов читает кеш, а не БД.
	if err := client.Set(ctx, key, `[{"category":"cached","limit":1}]`, cache.BudgetsTTL).Err(); err != nil {
		t.Fatal(err)
	}
	hit, err := svc.ListBudgets(ctx)
	if err != nil || !reflect.DeepEqual(hit, []Budget{{Category: "cached", Limit: 1}}) {
		t.Fatalf("expected cached value, got %+v, %v", hit, err)
	}
	// Запись бюджета удаляет ключ, следующий список снова приходит из БД.
	if err := svc.SetBudget(ctx, Budget{Category: "a", Limit: 100}); err != nil {
		t.Fatal(err)
	}
	if exists() {
		t.Fatal("SetBudget must delete budgets:all")
	}
	fresh, err := svc.ListBudgets(ctx)
	if err != nil || !reflect.DeepEqual(fresh, []Budget{{Category: "a", Limit: 100}, {Category: "b", Limit: 200}}) {
		t.Fatalf("after SetBudget: %+v, %v", fresh, err)
	}
	// Повреждённое значение и пустой список (null) не ломают чтение.
	for _, broken := range []string{"{broken", "null"} {
		if err := client.Set(ctx, key, broken, cache.BudgetsTTL).Err(); err != nil {
			t.Fatal(err)
		}
		got, err := svc.ListBudgets(ctx)
		if err != nil || !reflect.DeepEqual(got, fresh) {
			t.Fatalf("fallback for %q: %+v, %v", broken, got, err)
		}
	}
}

func TestInvalidReportDates(t *testing.T) {
	svc := service.New(domain.Repositories{}, nil, nil, nil, nil)
	for _, dates := range [][2]string{{"", ""}, {"2026-02-30", "2026-03-01"}, {"2026-02-02", "2026-02-01"}} {
		if _, err := svc.GetReportSummary(context.Background(), dates[0], dates[1]); !errors.Is(err, ErrInvalidReportDates) {
			t.Fatalf("dates=%v: %v", dates, err)
		}
	}
}

func TestOpenCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc, closeFn, err := Open(ctx)
	if err == nil || svc != nil || closeFn != nil {
		t.Fatalf("Open canceled context: %v, %v", svc, err)
	}
}
