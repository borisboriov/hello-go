package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/borisboriov/hello-go/ledger"
	"github.com/borisboriov/hello-go/ledger/testutil"
)

// HTTP-тесты с общим Ledger не вызывают t.Parallel: каждый сценарий очищает
// хранилище перед запуском и после завершения всех своих запросов.
func setupAPI(t *testing.T) http.Handler {
	t.Helper()
	testutil.Database(t)
	if err := ledger.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := ledger.Close(); err != nil {
			t.Error(err)
		}
	})
	return NewHandler()
}

func request(t *testing.T, handler http.Handler, method, path, body string, wantStatus int) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != wantStatus {
		t.Fatalf("%s %s: status=%d, want %d, body=%s", method, path, w.Code, wantStatus, w.Body)
	}
	// Проверяется буквальное значение контракта, а не константа из реализации.
	if got := w.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type=%q", got)
	}
	if !json.Valid(w.Body.Bytes()) {
		t.Fatalf("invalid JSON response: %s", w.Body)
	}
	if wantStatus >= 400 {
		result := decodeResponse[map[string]string](t, w)
		if len(result) != 1 || result["error"] == "" {
			t.Fatalf("invalid error response: %s", w.Body)
		}
	}
	return w
}

func decodeResponse[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var result T
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, w.Body)
	}
	return result
}

func TestEmptyLists(t *testing.T) {
	for _, path := range []string{"/api/transactions", "/api/budgets"} {
		t.Run(path, func(t *testing.T) {
			w := request(t, setupAPI(t), "GET", path, "", 200)
			if strings.TrimSpace(w.Body.String()) != "[]" {
				t.Fatalf("empty list should be [], got %s", w.Body)
			}
		})
	}
}

func TestBudgetEndpoints(t *testing.T) {
	for _, tt := range []struct {
		name   string
		body   string
		update bool
		status int
	}{
		{"ok", `{"category":"еда","limit":5000}`, false, 201},
		{"update", `{"category":"еда","limit":6000}`, true, 201},
		{"zero_limit", `{"category":"еда","limit":0}`, false, 400},
		{"negative_limit", `{"category":"еда","limit":-1}`, false, 400},
		{"empty_category", `{"category":"","limit":5000}`, false, 400},
		{"blank_category", `{"category":"  ","limit":5000}`, false, 400},
		{"invalid_update", `{"category":"еда","limit":0}`, true, 400},
		{"bad_json", `{"category":`, false, 400},
	} {
		t.Run(tt.name, func(t *testing.T) {
			handler := setupAPI(t)
			expected := make([]BudgetResponse, 0)
			if tt.update {
				request(t, handler, "POST", "/api/budgets", `{"category":"еда","limit":5000}`, 201)
				expected = append(expected, BudgetResponse{Category: "еда", Limit: 5000})
			}
			w := request(t, handler, "POST", "/api/budgets", tt.body, tt.status)
			if tt.status == 201 {
				var input CreateBudgetRequest
				if err := json.Unmarshal([]byte(tt.body), &input); err != nil {
					t.Fatal(err)
				}
				want := BudgetResponse{Category: input.Category, Limit: input.Limit}
				if got := decodeResponse[BudgetResponse](t, w); got != want {
					t.Fatalf("budget = %+v, want %+v", got, want)
				}
				expected = []BudgetResponse{want}
			}
			got := decodeResponse[[]BudgetResponse](t, request(t, handler, "GET", "/api/budgets", "", 200))
			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("budgets = %+v, want %+v", got, expected)
			}
		})
	}
}

func TestTransactionEndpoints(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		seed   bool
		status int
	}{
		{"ok", `{"amount":450,"category":"еда","description":"ланч","date":"2025-09-10"}`, false, 201},
		{"exact_limit", `{"amount":4550,"category":"еда","date":"2025-09-10"}`, true, 201},
		{"exceeded", `{"amount":5001,"category":"еда","date":"2025-09-10"}`, false, 409},
		{"cumulative_exceeded", `{"amount":4551,"category":"еда","date":"2025-09-10"}`, true, 409},
		{"without_budget", `{"amount":9000,"category":"книги","date":"2025-09-10"}`, false, 201},
		{"zero_amount", `{"amount":0,"category":"еда"}`, false, 400},
		{"negative_amount", `{"amount":-1,"category":"еда"}`, false, 400},
		{"empty_category", `{"amount":450,"category":""}`, false, 400},
		{"blank_category", `{"amount":450,"category":"  "}`, false, 400},
		{"bad_json", `{"amount":`, false, 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := setupAPI(t)
			request(t, handler, "POST", "/api/budgets", `{"category":"еда","limit":5000}`, 201)
			if tt.seed {
				request(t, handler, "POST", "/api/transactions", `{"amount":450,"category":"еда","date":"2025-09-10"}`, 201)
			}
			before := decodeResponse[[]TransactionResponse](t, request(t, handler, "GET", "/api/transactions", "", 200))
			w := request(t, handler, "POST", "/api/transactions", tt.body, tt.status)
			expected := append([]TransactionResponse{}, before...)
			if tt.status == 201 {
				var input CreateTransactionRequest
				if err := json.Unmarshal([]byte(tt.body), &input); err != nil {
					t.Fatal(err)
				}
				want := TransactionResponse{ID: len(before) + 1, Amount: input.Amount, Category: input.Category, Description: input.Description, Date: input.Date}
				if got := decodeResponse[TransactionResponse](t, w); got != want {
					t.Fatalf("transaction = %+v, want %+v", got, want)
				}
				expected = append([]TransactionResponse{want}, expected...)
			} else if tt.status == 409 {
				if got := decodeResponse[map[string]string](t, w)["error"]; got != "budget exceeded" {
					t.Fatalf("conflict error = %q", got)
				}
			}
			after := decodeResponse[[]TransactionResponse](t, request(t, handler, "GET", "/api/transactions", "", 200))
			if !reflect.DeepEqual(after, expected) {
				t.Fatalf("transactions = %+v, want %+v", after, expected)
			}
		})
	}
}

func TestInvalidJSON(t *testing.T) {
	for _, path := range []string{"/api/transactions", "/api/budgets"} {
		t.Run(path, func(t *testing.T) {
			numberField := "amount"
			if path == "/api/budgets" {
				numberField = "limit"
			}
			for _, tt := range []struct{ name, body string }{
				{"empty", ""}, {"malformed", "{"}, {"array", "[]"}, {"null", "null"},
				{"missing_fields", "{}"}, {"unknown_field", `{"unknown":1}`},
				{"multiple_values", `{} {}`}, {"trailing_garbage", `{} garbage`},
				{"string_number", fmt.Sprintf(`{"%s":"450","category":"еда"}`, numberField)},
				{"fractional_number", fmt.Sprintf(`{"%s":1.5,"category":"еда"}`, numberField)},
				{"too_large", fmt.Sprintf(`{"category":"%s","%s":1}`, strings.Repeat("x", 1<<20), numberField)},
			} {
				t.Run(tt.name, func(t *testing.T) {
					handler := setupAPI(t)
					request(t, handler, "POST", path, tt.body, 400)
					w := request(t, handler, "GET", path, "", 200)
					if strings.TrimSpace(w.Body.String()) != "[]" {
						t.Fatalf("invalid request changed storage: %s", w.Body)
					}
				})
			}
		})
	}
}

func TestRoutes(t *testing.T) {
	for _, tt := range []struct {
		name, method, path, allow string
		status                    int
	}{
		{"not_found", "GET", "/missing", "", 404},
		{"nested_not_found", "GET", "/api/transactions/missing", "", 404},
		{"transaction_method", "DELETE", "/api/transactions", "GET, POST", 405},
		{"budget_method", "PATCH", "/api/budgets", "GET, POST", 405},
		{"ping", "GET", "/ping", "", 200},
		{"ping_method", "POST", "/ping", "GET", 405},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := request(t, NewHandler(), tt.method, tt.path, "", tt.status)
			if got := w.Header().Get("Allow"); got != tt.allow {
				t.Fatalf("Allow=%q, want %q", got, tt.allow)
			}
			if tt.name == "ping" && decodeResponse[map[string]string](t, w)["message"] != "pong" {
				t.Fatalf("incorrect ping: %s", w.Body)
			}
		})
	}
}

func TestConcurrentRequests(t *testing.T) {
	handlers := []http.Handler{setupAPI(t), NewHandler()}
	request(t, handlers[0], "POST", "/api/budgets", `{"category":"еда","limit":20}`, 201)
	var wg sync.WaitGroup
	type result struct {
		index    int
		response *httptest.ResponseRecorder
	}
	results := make(chan result, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := httptest.NewRecorder()
			body := fmt.Sprintf(`{"amount":1,"category":"еда","description":"request-%d","date":"2025-09-10"}`, i)
			handlers[i%2].ServeHTTP(w, httptest.NewRequest("POST", "/api/transactions", strings.NewReader(body)))
			results <- result{i, w}
		}(i)
	}
	wg.Wait()
	close(results)
	ids := make(map[int]bool)
	for result := range results {
		w := result.response
		if w.Code != 201 {
			t.Fatalf("concurrent request failed: status=%d body=%s", w.Code, w.Body)
		}
		tx := decodeResponse[TransactionResponse](t, w)
		if ids[tx.ID] || tx.ID < 1 || tx.ID > 20 || tx.Description != fmt.Sprintf("request-%d", result.index) {
			t.Fatalf("incorrect concurrent response: %+v", tx)
		}
		ids[tx.ID] = true
	}
	saved := decodeResponse[[]TransactionResponse](t, request(t, handlers[0], "GET", "/api/transactions", "", 200))
	if len(saved) != 20 {
		t.Fatalf("saved %d transactions, want 20", len(saved))
	}
	request(t, handlers[0], "POST", "/api/transactions", `{"amount":1,"category":"еда"}`, 409)
}

func TestLedgerErrorResponses(t *testing.T) {
	for _, tt := range []struct {
		name    string
		err     error
		status  int
		message string
	}{
		{"exceeded", ledger.ErrBudgetExceeded, 409, "budget exceeded"},
		{"wrapped_exceeded", fmt.Errorf("wrapped: %w", ledger.ErrBudgetExceeded), 409, "budget exceeded"},
		{"unexpected", errors.New("private internal details"), 500, "internal server error"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeLedgerError(w, tt.err)
			got := decodeResponse[map[string]string](t, w)
			if w.Code != tt.status || len(got) != 1 || got["error"] != tt.message || w.Header().Get("Content-Type") != "application/json; charset=utf-8" {
				t.Fatalf("incorrect error mapping: status=%d body=%s", w.Code, w.Body)
			}
		})
	}
}

func TestLogging(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })
	request(t, NewHandler(), "GET", "/missing", "", 404)
	for _, part := range []string{"method=GET", "path=/missing", "duration="} {
		if !strings.Contains(output.String(), part) {
			t.Fatalf("log missing %q: %s", part, output.String())
		}
	}
}
