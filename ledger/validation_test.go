package ledger

import (
	"strings"
	"testing"
)

func TestTransactionValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		tx      Transaction
		wantErr string
	}{
		{"ok", Transaction{Amount: 450, Category: "еда", Date: "2025-09-10"}, ""},
		{"minimum_amount", Transaction{Amount: 1, Category: "еда"}, ""},
		{"invalid_date", Transaction{Amount: 1, Category: "еда", Date: "2026-02-30"}, "date"},
		{"too_large", Transaction{Amount: 100000000000000, Category: "еда"}, "NUMERIC"},
		{"zero_amount", Transaction{Category: "еда"}, "amount"},
		{"negative_amount", Transaction{Amount: -1, Category: "еда"}, "amount"},
		{"empty_category", Transaction{Amount: 450}, "category"},
		{"blank_category", Transaction{Amount: 450, Category: " \t\n"}, "category"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel() // Validate не обращается к общему хранилищу.
			assertValidationError(t, tt.tx.Validate(), tt.wantErr)
		})
	}
}

func TestBudgetValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		budget  Budget
		wantErr string
	}{
		{"ok", Budget{Limit: 5000, Category: "еда"}, ""},
		{"minimum_limit", Budget{Limit: 1, Category: "еда"}, ""},
		{"zero_limit", Budget{Category: "еда"}, "limit"},
		{"negative_limit", Budget{Limit: -1, Category: "еда"}, "limit"},
		{"empty_category", Budget{Limit: 5000}, "category"},
		{"blank_category", Budget{Limit: 5000, Category: " \t\n"}, "category"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertValidationError(t, tt.budget.Validate(), tt.wantErr)
		})
	}
}

func TestCheckValid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   Validatable
		wantErr string
	}{
		{"transaction_ok", Transaction{Amount: 1, Category: "Книги"}, ""},
		{"transaction_invalid", Transaction{Category: "Книги"}, "amount"},
		{"budget_ok", Budget{Limit: 1, Category: "Книги"}, ""},
		{"budget_invalid", Budget{Category: "Книги"}, "limit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertValidationError(t, CheckValid(tt.value), tt.wantErr)
		})
	}
}

func assertValidationError(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want error containing %q", err, want)
	}
}
