package domain

import "context"

type BudgetRepository interface {
	SetBudget(context.Context, Budget) error
	FindBudget(context.Context, string) (Budget, error)
	ListBudgets(context.Context) ([]Budget, error)
}
type ExpenseRepository interface {
	InsertTransaction(context.Context, Transaction) (Transaction, error)
	ListTransactions(context.Context) ([]Transaction, error)
	SumByCategory(context.Context, string) (int64, error)
}
type Repositories struct {
	Budgets  BudgetRepository
	Expenses ExpenseRepository
}

// WithinTransaction передаёт репозитории одной транзакции. Найденный бюджет
// блокируется до завершения callback, ошибка callback отменяет изменения.
type TransactionRunner interface {
	WithinTransaction(context.Context, func(Repositories) error) error
}
type ReportRepository interface {
	GetReportSummary(context.Context, string, string) (ReportSummary, error)
}
type ReportCache interface {
	Get(context.Context, string) (ReportSummary, error)
	Set(context.Context, string, ReportSummary) error
}

// BudgetCache хранит список всех бюджетов под одним ключом. Get возвращает
// ErrCacheMiss, если значения нет; Delete вызывается после изменения бюджета.
type BudgetCache interface {
	Get(context.Context, string) ([]Budget, error)
	Set(context.Context, string, []Budget) error
	Delete(context.Context, string) error
}
