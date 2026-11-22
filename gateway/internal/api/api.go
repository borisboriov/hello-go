package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/borisboriov/hello-go/ledger"
)

const contentType = "application/json; charset=utf-8"

type Handler struct{ service ledger.Service }

// NewHandler подключает маршруты и middleware ко всему HTTP-слою.
// Таймаут берётся из REQUEST_TIMEOUT (по умолчанию DefaultRequestTimeout).
func NewHandler(service ledger.Service) http.Handler {
	return NewHandlerWithTimeout(service, RequestTimeoutFromEnv())
}

// NewHandlerWithTimeout позволяет задать таймаут явно (например, в тестах).
func NewHandlerWithTimeout(service ledger.Service, timeout time.Duration) http.Handler {
	h := &Handler{service: service}
	return Logging(Timeout(timeout)(http.HandlerFunc(h.serveHTTP)))
}

func (h *Handler) serveHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", contentType)
	switch r.URL.Path {
	case "/api/transactions":
		switch r.Method {
		case http.MethodPost:
			h.createTransaction(w, r)
		case http.MethodGet:
			h.listTransactions(w, r)
		default:
			methodNotAllowed(w, "GET, POST")
		}
	case "/api/transactions/bulk":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, "POST")
			return
		}
		h.importTransactions(w, r)
	case "/api/budgets":
		switch r.Method {
		case http.MethodPost:
			h.createBudget(w, r)
		case http.MethodGet:
			h.listBudgets(w, r)
		default:
			methodNotAllowed(w, "GET, POST")
		}
	case "/api/reports/summary":
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		report, err := h.service.GetReportSummary(r.Context(), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
		if errors.Is(err, ledger.ErrInvalidReportDates) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err != nil {
			writeLedgerError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, report)
	case "/ping":
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": "pong"})
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func (h *Handler) createTransaction(w http.ResponseWriter, r *http.Request) {
	var request CreateTransactionRequest
	if err := decodeJSON(w, r, &request, "object"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tx := request.toDomain()
	tx, err := h.service.CreateTransaction(r.Context(), tx)
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, transactionResponse(tx))
}

func (h *Handler) listTransactions(w http.ResponseWriter, r *http.Request) {
	transactions, err := h.service.ListTransactions(r.Context())
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	response := make([]TransactionResponse, 0, len(transactions))
	for _, tx := range transactions {
		response = append(response, transactionResponse(tx))
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) createBudget(w http.ResponseWriter, r *http.Request) {
	var request CreateBudgetRequest
	if err := decodeJSON(w, r, &request, "object"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	b := request.toDomain()
	err := h.service.SetBudget(r.Context(), b)
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, budgetResponse(b))
}

func (h *Handler) listBudgets(w http.ResponseWriter, r *http.Request) {
	budgets, err := h.service.ListBudgets(r.Context())
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	response := make([]BudgetResponse, 0, len(budgets))
	for _, b := range budgets {
		response = append(response, budgetResponse(b))
	}
	writeJSON(w, http.StatusOK, response)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any, kind string) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	// Принимается ровно одно JSON-значение без второго значения или мусора.
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("request body must contain a single JSON %s", kind)
	}
	return nil
}

func writeLedgerError(w http.ResponseWriter, r *http.Request, err error) {
	status, message := ledgerErrorResponse(r, err)
	writeError(w, status, message)
}

func ledgerErrorResponse(r *http.Request, err error) (status int, message string) {
	// Драйвер БД не всегда оборачивает ошибку контекста, поэтому проверяем и сам ctx.
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || r.Context().Err() != nil {
		log.Printf("request aborted path=%s: %v", r.URL.Path, err)
		return TimeoutStatus, "request timeout or canceled"
	}
	var validation *ledger.ValidationError
	if errors.As(err, &validation) {
		return http.StatusBadRequest, validation.Error()
	}
	if errors.Is(err, ledger.ErrBudgetExceeded) {
		return http.StatusConflict, "budget exceeded"
	}
	log.Printf("ledger error: %v", err)
	return http.StatusInternalServerError, "internal server error"
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write response: %v", err)
	}
}
