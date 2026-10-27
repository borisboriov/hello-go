package api

import (
	"testing"

	"github.com/borisboriov/hello-go/ledger"
	"github.com/borisboriov/hello-go/ledger/testutil"
)

func TestCreateReturnsInsertedID(t *testing.T) {
	handler := setupAPI(t)
	first := decodeResponse[TransactionResponse](t, request(t, handler, "POST", "/api/transactions",
		`{"amount":101,"category":"a","date":"2026-10-27"}`, 201))
	older := decodeResponse[TransactionResponse](t, request(t, handler, "POST", "/api/transactions",
		`{"amount":102,"category":"a","date":"2026-10-01"}`, 201))
	if older.ID <= first.ID || older.Amount != 102 || older.Date != "2026-10-01" {
		t.Fatalf("wrong inserted object: %+v", older)
	}
	rows := decodeResponse[[]TransactionResponse](t, request(t, handler, "GET", "/api/transactions", "", 200))
	if len(rows) != 2 || rows[0].ID != first.ID || rows[1].ID != older.ID {
		t.Fatalf("wrong order: %+v", rows)
	}
	request(t, handler, "POST", "/api/transactions", `{"amount":1,"category":"a","date":"2026-02-30"}`, 400)
}

func TestReportEndpoint(t *testing.T) {
	handler := setupAPI(t)
	testutil.ReportKey(t, "2042-06-01", "2042-06-30")
	request(t, handler, "POST", "/api/transactions", `{"amount":125,"category":"a","date":"2042-06-10"}`, 201)
	path := "/api/reports/summary?from=2042-06-01&to=2042-06-30"
	first := decodeResponse[ledger.ReportSummary](t, request(t, handler, "GET", path, "", 200))
	if first.Total != 125 || len(first.Categories) != 1 || first.Categories[0].Category != "a" {
		t.Fatalf("report: %+v", first)
	}
	second := decodeResponse[ledger.ReportSummary](t, request(t, handler, "GET", path, "", 200))
	if second.Total != first.Total {
		t.Fatalf("cache report: %+v", second)
	}
	request(t, handler, "GET", "/api/reports/summary", "", 400)
	request(t, handler, "GET", "/api/reports/summary?from=2042-07-01&to=2042-06-01", "", 400)
	request(t, handler, "POST", path, "{}", 405)
}

func TestDatabaseErrorsAre500(t *testing.T) {
	handler := setupAPI(t)
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/transactions", "/api/budgets", "/api/reports/summary?from=2026-01-01&to=2026-12-31"} {
		result := decodeResponse[map[string]string](t, request(t, handler, "GET", path, "", 500))
		if result["error"] != "internal server error" {
			t.Fatal(result)
		}
	}
	request(t, handler, "POST", "/api/budgets", `{"category":"a","limit":100}`, 500)
	request(t, handler, "POST", "/api/transactions", `{"category":"a","amount":1}`, 500)
}
