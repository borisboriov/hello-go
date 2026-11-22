package api

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/borisboriov/hello-go/ledger"
)

type bulkServiceStub struct {
	ledger.Service
	importTransactions func(context.Context, []ledger.Transaction, int) (ledger.BulkImportSummary, error)
}

func (s bulkServiceStub) ImportTransactions(ctx context.Context, transactions []ledger.Transaction, workers int) (ledger.BulkImportSummary, error) {
	return s.importTransactions(ctx, transactions, workers)
}

func TestBulkImportPartialSuccess(t *testing.T) {
	input := []ledger.Transaction{
		{Amount: 150, Category: "food", Description: "lunch", Date: "2026-11-22"},
		{Category: "food"},
		{Amount: 1000, Category: "food"},
		{Amount: 1, Category: "books"},
	}
	stub := bulkServiceStub{importTransactions: func(ctx context.Context, transactions []ledger.Transaction, workers int) (ledger.BulkImportSummary, error) {
		if _, ok := ctx.Deadline(); !ok || workers != 2 || !reflect.DeepEqual(transactions, input) {
			t.Fatalf("workers=%d, transactions=%+v, deadline set=%t", workers, transactions, ok)
		}
		return ledger.BulkImportSummary{Accepted: 1, Rejected: 3, Errors: []ledger.BulkImportError{
			{Index: 1, Err: &ledger.ValidationError{Err: errors.New("transaction amount must be positive")}},
			{Index: 2, Err: ledger.ErrBudgetExceeded},
			{Index: 3, Err: errors.New("private storage details")},
		}}, nil
	}}
	body := `[{"amount":150,"category":"food","description":"lunch","date":"2026-11-22"},` +
		`{"amount":0,"category":"food"},{"amount":1000,"category":"food"},{"amount":1,"category":"books"}]`
	w := request(t, NewHandler(stub), "POST", "/api/transactions/bulk?workers=2", body, 200)
	got := decodeResponse[bulkImportResponse](t, w)
	want := bulkImportResponse{Accepted: 1, Rejected: 3, Errors: []bulkImportErrorResponse{
		{Index: 1, Error: "transaction amount must be positive"},
		{Index: 2, Error: "budget exceeded"},
		{Index: 3, Error: "internal server error"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("summary=%+v, want %+v", got, want)
	}
}

func TestBulkImportEmptyBatch(t *testing.T) {
	stub := bulkServiceStub{importTransactions: func(_ context.Context, transactions []ledger.Transaction, workers int) (ledger.BulkImportSummary, error) {
		if len(transactions) != 0 || workers != 0 {
			t.Fatalf("expected empty input and default workers, got %+v, %d", transactions, workers)
		}
		return ledger.BulkImportSummary{}, nil
	}}
	w := request(t, NewHandler(stub), "POST", "/api/transactions/bulk", `[]`, 200)
	if got := decodeResponse[bulkImportResponse](t, w); got.Accepted != 0 || got.Rejected != 0 || got.Errors == nil || len(got.Errors) != 0 {
		t.Fatalf("expected zero counts with an empty errors array, got %s", w.Body)
	}
}

func TestBulkImportBadRequest(t *testing.T) {
	for _, tt := range []struct{ name, query, body string }{
		{"null", "", `null`},
		{"object", "", `{"amount":1,"category":"food"}`},
		{"broken JSON", "", `[`},
		{"multiple values", "", `[] []`},
		{"invalid element type", "", `[1]`},
		{"unknown field", "", `[{"unknown":1}]`},
		{"zero workers", "?workers=0", `[]`},
		{"missing worker count", "?workers=", `[]`},
		{"invalid worker count", "?workers=many", `[]`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request(t, NewHandler(nil), "POST", "/api/transactions/bulk"+tt.query, tt.body, 400)
		})
	}
	w := request(t, NewHandler(nil), "GET", "/api/transactions/bulk", "", 405)
	if got := w.Header().Get("Allow"); got != "POST" {
		t.Fatalf("Allow=%q", got)
	}
}

func TestBulkImportTimeout(t *testing.T) {
	stub := bulkServiceStub{importTransactions: func(ctx context.Context, _ []ledger.Transaction, _ int) (ledger.BulkImportSummary, error) {
		<-ctx.Done()
		return ledger.BulkImportSummary{Accepted: 1}, ctx.Err()
	}}
	server := NewHandlerWithTimeout(stub, 20*time.Millisecond)
	w := request(t, server, "POST", "/api/transactions/bulk", `[{"amount":1,"category":"food"}]`, http.StatusGatewayTimeout)
	if got := decodeResponse[map[string]string](t, w)["error"]; got != "request timeout or canceled" {
		t.Fatalf("timeout error=%q", got)
	}
}

func TestBulkImportRespectsBudgetInPostgres(t *testing.T) {
	handler := setupAPI(t)
	request(t, handler, "POST", "/api/budgets", `{"category":"food","limit":100}`, 201)
	w := request(t, handler, "POST", "/api/transactions/bulk?workers=3",
		`[{"amount":60,"category":"food"},{"amount":60,"category":"food"},{"amount":0,"category":"food"}]`, 200)
	got := decodeResponse[bulkImportResponse](t, w)
	if got.Accepted != 1 || got.Rejected != 2 || len(got.Errors) != 2 ||
		got.Errors[0].Index > 1 || got.Errors[0].Error != "budget exceeded" ||
		got.Errors[1] != (bulkImportErrorResponse{Index: 2, Error: "transaction amount must be positive"}) {
		t.Fatalf("unexpected import summary: %+v", got)
	}
	saved := decodeResponse[[]TransactionResponse](t, request(t, handler, "GET", "/api/transactions", "", 200))
	if len(saved) != 1 || saved[0].Amount != 60 || saved[0].Category != "food" {
		t.Fatalf("unexpected saved transactions: %+v", saved)
	}
}
