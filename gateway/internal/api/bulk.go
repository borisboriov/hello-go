package api

import (
	"net/http"
	"strconv"

	"github.com/borisboriov/hello-go/ledger"
)

type bulkImportResponse struct {
	Accepted int64                     `json:"accepted"`
	Rejected int64                     `json:"rejected"`
	Errors   []bulkImportErrorResponse `json:"errors"`
}

type bulkImportErrorResponse struct {
	Index int    `json:"index"`
	Error string `json:"error"`
}

func (h *Handler) importTransactions(w http.ResponseWriter, r *http.Request) {
	var workers int
	if query := r.URL.Query(); query.Has("workers") {
		var err error
		workers, err = strconv.Atoi(query.Get("workers"))
		if err != nil || workers <= 0 {
			writeError(w, http.StatusBadRequest, "workers must be a positive integer")
			return
		}
	}
	var input []CreateTransactionRequest
	if err := decodeJSON(w, r, &input, "array"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if input == nil {
		writeError(w, http.StatusBadRequest, "request body must contain a JSON array")
		return
	}
	transactions := make([]ledger.Transaction, len(input))
	for i, item := range input {
		transactions[i] = item.toDomain()
	}
	summary, err := h.service.ImportTransactions(r.Context(), transactions, workers)
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	response := bulkImportResponse{
		Accepted: summary.Accepted,
		Rejected: summary.Rejected,
		Errors:   make([]bulkImportErrorResponse, 0, len(summary.Errors)),
	}
	for _, failure := range summary.Errors {
		_, message := ledgerErrorResponse(r, failure.Err)
		response.Errors = append(response.Errors, bulkImportErrorResponse{Index: failure.Index, Error: message})
	}
	writeJSON(w, http.StatusOK, response)
}
