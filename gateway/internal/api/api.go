package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/borisboriov/hello-go/ledger"
)

const contentType = "application/json; charset=utf-8"

// NewHandler подключает маршруты и middleware ко всему HTTP-слою.
func NewHandler() http.Handler {
	return Logging(http.HandlerFunc(serveHTTP))
}

func serveHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", contentType)
	switch r.URL.Path {
	case "/api/transactions":
		switch r.Method {
		case http.MethodPost:
			createTransaction(w, r)
		case http.MethodGet:
			listTransactions(w, r)
		default:
			methodNotAllowed(w, "GET, POST")
		}
	case "/api/budgets":
		switch r.Method {
		case http.MethodPost:
			createBudget(w, r)
		case http.MethodGet:
			listBudgets(w, r)
		default:
			methodNotAllowed(w, "GET, POST")
		}
	case "/api/reports/summary":
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		report, err := ledger.GetReportSummary(r.Context(), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
		if errors.Is(err, ledger.ErrInvalidReportDates) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err != nil {
			writeLedgerError(w, err)
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

func createTransaction(w http.ResponseWriter, r *http.Request) {
	var request CreateTransactionRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tx := request.toDomain()
	if err := tx.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tx, err := ledger.CreateTransaction(tx)
	if err != nil {
		writeLedgerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, transactionResponse(tx))
}

func listTransactions(w http.ResponseWriter, r *http.Request) {
	transactions, err := ledger.ListTransactions()
	if err != nil {
		writeLedgerError(w, err)
		return
	}
	response := make([]TransactionResponse, 0, len(transactions))
	for _, tx := range transactions {
		response = append(response, transactionResponse(tx))
	}
	writeJSON(w, http.StatusOK, response)
}

func createBudget(w http.ResponseWriter, r *http.Request) {
	var request CreateBudgetRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	b := request.toDomain()
	if err := b.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	err := ledger.SetBudget(b)
	if err != nil {
		writeLedgerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, budgetResponse(b))
}

func listBudgets(w http.ResponseWriter, r *http.Request) {
	budgets, err := ledger.ListBudgets()
	if err != nil {
		writeLedgerError(w, err)
		return
	}
	response := make([]BudgetResponse, 0, len(budgets))
	for _, b := range budgets {
		response = append(response, budgetResponse(b))
	}
	writeJSON(w, http.StatusOK, response)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	// Принимается ровно один JSON-объект, без второго значения или мусора.
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

func writeLedgerError(w http.ResponseWriter, err error) {
	if errors.Is(err, ledger.ErrBudgetExceeded) {
		writeError(w, http.StatusConflict, "budget exceeded")
		return
	}
	log.Printf("ledger error: %v", err)
	writeError(w, http.StatusInternalServerError, "internal server error")
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
