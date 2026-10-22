package main

import (
	"reflect"
	"strings"
	"testing"
)

func resetLedger(t *testing.T) {
	t.Helper()
	oldTransactions, oldBudgets := transactions, budgets
	transactions = make([]Transaction, 0)
	budgets = make(map[string]Budget)
	t.Cleanup(func() {
		transactions, budgets = oldTransactions, oldBudgets
	})
}

func TestAddTransactionValidationAndBudget(t *testing.T) {
	resetLedger(t)
	if err := SetBudget(Budget{Category: "Продукты", Limit: 500}); err != nil {
		t.Fatal(err)
	}
	for _, tx := range []Transaction{
		{Amount: 0, Category: "Продукты"},
		{Amount: -100, Category: "Продукты"},
		{Amount: 100},
		{Amount: 100, Category: " \t"},
	} {
		if err := AddTransaction(tx); err == nil {
			t.Fatalf("invalid transaction accepted: %+v", tx)
		}
	}
	if len(ListTransactions()) != 0 {
		t.Fatal("invalid transactions were saved")
	}
	for _, amount := range []int{200, 300} {
		if err := AddTransaction(Transaction{Amount: amount, Category: "Продукты"}); err != nil {
			t.Fatalf("transaction within budget rejected: %v", err)
		}
	}
	before := ListTransactions()
	if err := AddTransaction(Transaction{Amount: 1, Category: "Продукты"}); err == nil || err.Error() != "budget exceeded" {
		t.Fatalf("expected budget exceeded, got %v", err)
	}
	if !reflect.DeepEqual(before, ListTransactions()) {
		t.Fatal("rejected transaction changed storage")
	}
	if err := AddTransaction(Transaction{Amount: 1000, Category: "Книги"}); err != nil {
		t.Fatalf("transaction without budget rejected: %v", err)
	}
	for i, tx := range ListTransactions() {
		if tx.ID != i+1 {
			t.Fatalf("ID = %d, want %d", tx.ID, i+1)
		}
	}
	// Выдача списка не должна давать вызывающему коду доступ к хранилищу.
	copyOfTransactions := ListTransactions()
	copyOfTransactions[0].Amount = 9999
	if ListTransactions()[0].Amount != 200 {
		t.Fatal("ListTransactions returned the underlying storage")
	}
}

func TestSetBudgetRejectsInvalidUpdates(t *testing.T) {
	resetLedger(t)
	original := Budget{Category: "Продукты", Limit: 500}
	if err := SetBudget(original); err != nil {
		t.Fatal(err)
	}
	for _, b := range []Budget{
		{Category: "Продукты", Limit: 0},
		{Category: "Продукты", Limit: -1},
		{Limit: 100},
		{Category: " \t", Limit: 100},
	} {
		if err := SetBudget(b); err == nil {
			t.Fatalf("invalid budget accepted: %+v", b)
		}
	}
	if len(budgets) != 1 || budgets[original.Category] != original {
		t.Fatalf("invalid update changed budgets: %+v", budgets)
	}
	if err := SetBudget(Budget{Category: "Продукты", Limit: 600}); err != nil {
		t.Fatal(err)
	}
	if err := AddTransaction(Transaction{Category: "Продукты", Amount: 600}); err != nil {
		t.Fatalf("updated budget not applied: %v", err)
	}
}

func TestLoadBudgets(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
	}{
		{"valid", `[{"category":"Книги","limit":100}]`, false},
		{"zero limit", `[{"category":"Книги","limit":0}]`, true},
		{"negative limit", `[{"category":"Книги","limit":-1}]`, true},
		{"empty category", `[{"category":"","limit":100}]`, true},
		{"malformed JSON", `[{`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetLedger(t)
			err := LoadBudgets(strings.NewReader(tt.json))
			if (err != nil) != tt.wantErr {
				t.Fatalf("LoadBudgets() = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && len(budgets) != 0 {
				t.Fatalf("invalid budget was saved: %+v", budgets)
			}
			if !tt.wantErr && budgets["Книги"].Limit != 100 {
				t.Fatalf("valid budget not loaded: %+v", budgets)
			}
		})
	}
}
