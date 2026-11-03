// Package ledger предоставляет интерфейс приложения и фабрику для внешних слоёв.
package ledger

import (
	"context"
	"io"

	"github.com/borisboriov/hello-go/ledger/internal/domain"
)

type Budget = domain.Budget
type Transaction = domain.Transaction
type CategoryTotal = domain.CategoryTotal
type ReportSummary = domain.ReportSummary
type Validatable = domain.Validatable
type ValidationError = domain.ValidationError

var ErrBudgetExceeded = domain.ErrBudgetExceeded
var ErrInvalidReportDates = domain.ErrInvalidReportDates

func CheckValid(v Validatable) error { return domain.CheckValid(v) }

type Service interface {
	SetBudget(context.Context, Budget) error
	ListBudgets(context.Context) ([]Budget, error)
	AddTransaction(context.Context, Transaction) error
	CreateTransaction(context.Context, Transaction) (Transaction, error)
	ListTransactions(context.Context) ([]Transaction, error)
	LoadBudgets(context.Context, io.Reader) error
	GetReportSummary(context.Context, string, string) (ReportSummary, error)
}
