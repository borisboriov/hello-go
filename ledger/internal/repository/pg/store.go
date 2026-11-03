package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/borisboriov/hello-go/ledger/internal/domain"
)

type querier interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
type Store struct{ db querier }

func New(db *sql.DB) *Store { return &Store{db: db} }

var _ domain.BudgetRepository = (*Store)(nil)
var _ domain.ExpenseRepository = (*Store)(nil)
var _ domain.ReportRepository = (*Store)(nil)

func (s *Store) SetBudget(ctx context.Context, b domain.Budget) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO budgets(category, limit_amount) VALUES($1,$2)
 ON CONFLICT(category) DO UPDATE SET limit_amount=EXCLUDED.limit_amount`, b.Category, money(b.Limit))
	if err != nil {
		return fmt.Errorf("upsert budget: %w", err)
	}
	return nil
}

func (s *Store) FindBudget(ctx context.Context, category string) (domain.Budget, error) {
	b := domain.Budget{Category: category}
	err := s.db.QueryRowContext(ctx, "SELECT (limit_amount*100)::bigint FROM budgets WHERE category=$1 FOR UPDATE", category).Scan(&b.Limit)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Budget{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Budget{}, fmt.Errorf("read budget: %w", err)
	}
	return b, nil
}

func (s *Store) SumByCategory(ctx context.Context, category string) (int64, error) {
	var sum int64
	err := s.db.QueryRowContext(ctx, "SELECT (COALESCE(SUM(amount),0)*100)::bigint FROM expenses WHERE category=$1", category).Scan(&sum)
	if err != nil {
		return 0, fmt.Errorf("read spent: %w", err)
	}
	return sum, nil
}

func (s *Store) InsertTransaction(ctx context.Context, tx domain.Transaction) (domain.Transaction, error) {
	err := s.db.QueryRowContext(ctx, `INSERT INTO expenses(amount, category, description, date)
 VALUES($1,$2,$3,$4::date) RETURNING id`, money(tx.Amount), tx.Category, tx.Description, tx.Date).Scan(&tx.ID)
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("insert expense: %w", err)
	}
	return tx, nil
}

// money переводит копейки в десятичную строку без float64.
func money(cents int) string { return fmt.Sprintf("%d.%02d", cents/100, cents%100) }

func (s *Store) ListBudgets(ctx context.Context) ([]domain.Budget, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT category, (limit_amount*100)::bigint FROM budgets ORDER BY category")
	if err != nil {
		return nil, fmt.Errorf("list budgets: %w", err)
	}
	defer rows.Close()
	result := make([]domain.Budget, 0)
	for rows.Next() {
		var b domain.Budget
		if err := rows.Scan(&b.Category, &b.Limit); err != nil {
			return nil, fmt.Errorf("scan budget: %w", err)
		}
		result = append(result, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read budgets: %w", err)
	}
	return result, nil
}

func (s *Store) ListTransactions(ctx context.Context) ([]domain.Transaction, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, (amount*100)::bigint, category,
        COALESCE(description,''), to_char(date,'YYYY-MM-DD') FROM expenses ORDER BY date DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list transactions: %w", err)
	}
	defer rows.Close()
	result := make([]domain.Transaction, 0)
	for rows.Next() {
		var tx domain.Transaction
		if err := rows.Scan(&tx.ID, &tx.Amount, &tx.Category, &tx.Description, &tx.Date); err != nil {
			return nil, fmt.Errorf("scan transaction: %w", err)
		}
		result = append(result, tx)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read transactions: %w", err)
	}
	return result, nil
}

func (s *Store) GetReportSummary(ctx context.Context, from, to string) (domain.ReportSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT category, (SUM(amount)*100)::bigint
        FROM expenses WHERE date >= $1::date AND date <= $2::date
        GROUP BY category ORDER BY category`, from, to)
	if err != nil {
		return domain.ReportSummary{}, fmt.Errorf("query report: %w", err)
	}
	defer rows.Close()
	result := domain.ReportSummary{From: from, To: to, Categories: make([]domain.CategoryTotal, 0)}
	for rows.Next() {
		var item domain.CategoryTotal
		if err := rows.Scan(&item.Category, &item.Amount); err != nil {
			return domain.ReportSummary{}, fmt.Errorf("scan report: %w", err)
		}
		result.Total += item.Amount
		result.Categories = append(result.Categories, item)
	}
	if err := rows.Err(); err != nil {
		return domain.ReportSummary{}, fmt.Errorf("read report: %w", err)
	}
	return result, nil
}
