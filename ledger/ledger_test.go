package ledger

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"
)

func resetLedger(t *testing.T) {
	t.Helper()
	Reset()
	t.Cleanup(Reset)
}

func TestAddTransaction(t *testing.T) {
	tests := []struct {
		name    string
		limit   int
		saved   []Transaction
		tx      Transaction
		wantErr string
	}{
		{name: "ok", limit: 5000, tx: Transaction{Amount: 450, Category: "еда", Date: "2025-09-10"}},
		{name: "exact_limit", limit: 5000, saved: []Transaction{{Amount: 4500, Category: "еда"}}, tx: Transaction{Amount: 500, Category: "еда"}},
		{name: "exceeded", limit: 5000, tx: Transaction{Amount: 5001, Category: "еда"}, wantErr: "budget exceeded"},
		{name: "cumulative_exceeded", limit: 5000, saved: []Transaction{{Amount: 4500, Category: "еда"}}, tx: Transaction{Amount: 501, Category: "еда"}, wantErr: "budget exceeded"},
		{name: "other_category", limit: 5000, saved: []Transaction{{Amount: 9000, Category: "книги"}}, tx: Transaction{Amount: 5000, Category: "еда"}},
		{name: "without_budget", tx: Transaction{Amount: 9000, Category: "еда"}},
		{name: "zero_amount", tx: Transaction{Category: "еда"}, wantErr: "amount"},
		{name: "negative_amount", tx: Transaction{Amount: -1, Category: "еда"}, wantErr: "amount"},
		{name: "empty_category", tx: Transaction{Amount: 100}, wantErr: "category"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetLedger(t) // Сценарии с общим состоянием выполняются последовательно.
			if tt.limit > 0 {
				if err := SetBudget(Budget{Category: "еда", Limit: tt.limit}); err != nil {
					t.Fatal(err)
				}
			}
			for _, tx := range tt.saved {
				if err := AddTransaction(tx); err != nil {
					t.Fatal(err)
				}
			}
			before := ListTransactions()
			err := AddTransaction(tt.tx)
			assertValidationError(t, err, tt.wantErr)
			after := ListTransactions()
			if tt.wantErr != "" {
				if tt.wantErr == "budget exceeded" && !errors.Is(err, ErrBudgetExceeded) {
					t.Fatalf("error = %v, want ErrBudgetExceeded", err)
				}
				if !reflect.DeepEqual(before, after) {
					t.Fatalf("rejected transaction changed storage: before=%+v after=%+v", before, after)
				}
				return
			}
			if len(after) != len(before)+1 {
				t.Fatalf("count = %d, want %d", len(after), len(before)+1)
			}
			want := tt.tx
			want.ID = len(before) + 1
			if after[len(after)-1] != want {
				t.Fatalf("saved transaction = %+v, want %+v", after[len(after)-1], want)
			}
		})
	}
}

func TestSetBudget(t *testing.T) {
	for _, tt := range []struct {
		name    string
		budget  Budget
		wantErr string
	}{
		{"update", Budget{Category: "еда", Limit: 6000}, ""},
		{"zero_limit", Budget{Category: "еда"}, "limit"},
		{"negative_limit", Budget{Category: "еда", Limit: -1}, "limit"},
		{"empty_category", Budget{Limit: 6000}, "category"},
		{"blank_category", Budget{Category: " \t", Limit: 6000}, "category"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resetLedger(t)
			original := Budget{Category: "еда", Limit: 5000}
			if err := SetBudget(original); err != nil {
				t.Fatal(err)
			}
			assertValidationError(t, SetBudget(tt.budget), tt.wantErr)
			want := original
			if tt.wantErr == "" {
				want = tt.budget
			}
			if got := ListBudgets(); !reflect.DeepEqual(got, []Budget{want}) {
				t.Fatalf("budgets = %+v, want %+v", got, want)
			}
			if err := AddTransaction(Transaction{Category: "еда", Amount: want.Limit}); err != nil {
				t.Fatalf("stored limit not applied: %v", err)
			}
		})
	}
}

func TestLoadBudgets(t *testing.T) {
	readErr := errors.New("read failed")
	tests := []struct {
		name    string
		reader  io.Reader
		wantErr string
	}{
		{"ok", strings.NewReader(`[{"category":"Книги","limit":100}]`), ""},
		{"zero_limit", strings.NewReader(`[{"category":"Книги","limit":0}]`), "limit"},
		{"negative_limit", strings.NewReader(`[{"category":"Книги","limit":-1}]`), "limit"},
		{"empty_category", strings.NewReader(`[{"category":"","limit":100}]`), "category"},
		{"bad_json", strings.NewReader(`[{`), "JSON"},
		{"read_error", iotest.ErrReader(readErr), "read failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetLedger(t)
			err := LoadBudgets(tt.reader)
			assertValidationError(t, err, tt.wantErr)
			if tt.name == "read_error" && !errors.Is(err, readErr) {
				t.Fatalf("read error not wrapped: %v", err)
			}
			if tt.wantErr != "" {
				if len(ListBudgets()) != 0 {
					t.Fatal("invalid budget was saved")
				}
			} else if got := ListBudgets(); !reflect.DeepEqual(got, []Budget{{Category: "Книги", Limit: 100}}) {
				t.Fatalf("loaded budgets = %+v", got)
			}
		})
	}
}

func TestStorageCopies(t *testing.T) {
	resetLedger(t)
	for _, category := range []string{"b", "a"} {
		if err := SetBudget(Budget{Category: category, Limit: 5000}); err != nil {
			t.Fatal(err)
		}
	}
	if err := AddTransaction(Transaction{Amount: 450, Category: "a"}); err != nil {
		t.Fatal(err)
	}
	snapshot := ListBudgets()
	if snapshot[0].Category != "a" || snapshot[1].Category != "b" {
		t.Fatalf("budgets not sorted: %+v", snapshot)
	}
	snapshot[0].Limit = 1
	txs := ListTransactions()
	txs[0].Amount = 9999
	if ListBudgets()[0].Limit != 5000 || ListTransactions()[0].Amount != 450 {
		t.Fatal("modifying a returned list changed storage")
	}
}

func TestReset(t *testing.T) {
	resetLedger(t)
	if err := SetBudget(Budget{Category: "еда", Limit: 5000}); err != nil {
		t.Fatal(err)
	}
	if err := AddTransaction(Transaction{Category: "еда", Amount: 450}); err != nil {
		t.Fatal(err)
	}
	Reset()
	if len(ListTransactions()) != 0 || len(ListBudgets()) != 0 {
		t.Fatal("Reset did not clear both collections")
	}
	Reset() // Повторный сброс также безопасен.
	if err := AddTransaction(Transaction{Category: "еда", Amount: 6000}); err != nil {
		t.Fatalf("old budget remained after Reset: %v", err)
	}
	if got := ListTransactions()[0].ID; got != 1 {
		t.Fatalf("ID after Reset = %d, want 1", got)
	}
}
