package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/borisboriov/hello-go/ledger"
)

func request(t *testing.T, handler http.Handler, method, path, body string, wantStatus int) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != wantStatus {
		t.Fatalf("%s %s: status=%d, want %d, body=%s", method, path, w.Code, wantStatus, w.Body)
	}
	if got := w.Header().Get("Content-Type"); got != contentType {
		t.Fatalf("Content-Type=%q", got)
	}
	if !json.Valid(w.Body.Bytes()) {
		t.Fatalf("invalid JSON response: %s", w.Body)
	}
	if wantStatus >= 400 {
		var result map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || len(result) != 1 || result["error"] == "" {
			t.Fatalf("invalid error response: %s", w.Body)
		}
	}
	return w
}

func TestAPI(t *testing.T) {
	handler := NewHandler()
	for _, path := range []string{"/api/transactions", "/api/budgets"} {
		w := request(t, handler, "GET", path, "", 200)
		if strings.TrimSpace(w.Body.String()) != "[]" {
			t.Fatalf("empty list should be [], got %s", w.Body)
		}
	}

	for _, path := range []string{"/api/transactions", "/api/budgets"} {
		for _, body := range []string{"", "{", "[]", "null", "{}", `{"unknown":1}`, `{} {}`, `{} garbage`} {
			request(t, handler, "POST", path, body, 400)
		}
	}
	for _, body := range []string{
		`{"amount":0,"category":"еда"}`,
		`{"amount":-1,"category":"еда"}`,
		`{"amount":1,"category":"  "}`,
		`{"amount":"450","category":"еда"}`,
		`{"amount":1.5,"category":"еда"}`,
	} {
		request(t, handler, "POST", "/api/transactions", body, 400)
	}
	for _, body := range []string{
		`{"limit":0,"category":"еда"}`,
		`{"limit":-1,"category":"еда"}`,
		`{"limit":1,"category":"  "}`,
	} {
		request(t, handler, "POST", "/api/budgets", body, 400)
	}
	request(t, handler, "POST", "/api/budgets", `{"category":"еда","limit":400}`, 201)
	w := request(t, handler, "POST", "/api/budgets", `{"category":"еда","limit":500}`, 201)
	var b BudgetResponse
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil || b.Category != "еда" || b.Limit != 500 {
		t.Fatalf("incorrect budget response: %s", w.Body)
	}
	// Неверное обновление не должно менять существующий лимит.
	request(t, handler, "POST", "/api/budgets", `{"category":"еда","limit":0}`, 400)
	w = request(t, handler, "POST", "/api/transactions", `{"amount":450,"category":"еда","description":"ланч","date":"2025-09-10"}`, 201)
	var tx TransactionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &tx); err != nil || tx.ID != 1 || tx.Amount != 450 || tx.Category != "еда" || tx.Description != "ланч" || tx.Date != "2025-09-10" {
		t.Fatalf("incorrect transaction response: %s", w.Body)
	}
	w = request(t, handler, "POST", "/api/transactions", `{"amount":51,"category":"еда"}`, 409)
	if strings.TrimSpace(w.Body.String()) != `{"error":"budget exceeded"}` {
		t.Fatalf("incorrect conflict response: %s", w.Body)
	}
	request(t, handler, "POST", "/api/transactions", `{"amount":50,"category":"еда"}`, 201)
	request(t, handler, "POST", "/api/transactions", `{"amount":1,"category":"еда"}`, 409)
	request(t, handler, "POST", "/api/transactions", `{"amount":1000,"category":"книги"}`, 201)
	w = request(t, handler, "GET", "/api/transactions", "", 200)
	var transactions []TransactionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &transactions); err != nil || len(transactions) != 3 {
		t.Fatalf("incorrect transactions list: %s", w.Body)
	}
	for i, saved := range transactions {
		if saved.ID != i+1 {
			t.Fatalf("ID=%d, want %d", saved.ID, i+1)
		}
	}
	w = request(t, handler, "GET", "/api/budgets", "", 200)
	var budgets []BudgetResponse
	if err := json.Unmarshal(w.Body.Bytes(), &budgets); err != nil || len(budgets) != 1 || budgets[0] != b {
		t.Fatalf("incorrect budgets list: %s", w.Body)
	}
	request(t, handler, "GET", "/missing", "", 404)
	request(t, handler, "GET", "/api/transactions/missing", "", 404)
	w = request(t, handler, "DELETE", "/api/transactions", "", 405)
	if w.Header().Get("Allow") != "GET, POST" {
		t.Fatal("missing Allow header")
	}
	request(t, handler, "PATCH", "/api/budgets", "", 405)
	request(t, handler, "GET", "/ping", "", 200)
	request(t, handler, "POST", "/ping", "", 405)
	request(t, handler, "POST", "/api/transactions", `{"category":"`+strings.Repeat("x", 1<<20)+`","amount":1}`, 400)

	t.Run("parallel requests", func(t *testing.T) {
		// Несколько Handler используют одно хранилище и один общий mutex.
		handlers := []http.Handler{handler, NewHandler()}
		var wg sync.WaitGroup
		results := make(chan *httptest.ResponseRecorder, 20)
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				w := httptest.NewRecorder()
				body := fmt.Sprintf(`{"amount":1,"category":"parallel-%d"}`, i)
				handlers[i%len(handlers)].ServeHTTP(w, httptest.NewRequest("POST", "/api/transactions", strings.NewReader(body)))
				results <- w
			}(i)
		}
		wg.Wait()
		close(results)
		ids := make(map[int]bool)
		for w := range results {
			var tx TransactionResponse
			if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &tx) != nil || ids[tx.ID] || tx.ID <= 3 {
				t.Fatalf("incorrect concurrent response: status=%d, body=%s", w.Code, w.Body)
			}
			ids[tx.ID] = true
		}
	})
}

func TestLedgerErrorResponses(t *testing.T) {
	for _, tt := range []struct {
		err    error
		status int
		body   string
	}{
		{fmt.Errorf("wrapped: %w", ledger.ErrBudgetExceeded), 409, `{"error":"budget exceeded"}`},
		{errors.New("private internal details"), 500, `{"error":"internal server error"}`},
	} {
		w := httptest.NewRecorder()
		writeLedgerError(w, tt.err)
		if w.Code != tt.status || strings.TrimSpace(w.Body.String()) != tt.body || w.Header().Get("Content-Type") != contentType {
			t.Fatalf("incorrect error mapping: status=%d, body=%s", w.Code, w.Body)
		}
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
