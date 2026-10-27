package ledger

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Transaction представляет транзакцию. Amount хранит сумму в копейках.
type Transaction struct {
	ID          int
	Amount      int
	Category    string
	Description string
	Date        string
}

// ErrBudgetExceeded позволяет вызывающему коду распознать превышение бюджета.
var ErrBudgetExceeded = errors.New("budget exceeded")

// Validate проверяет поля транзакции, не изменяя данные и не логируя ошибки.
// Пустая дата при сохранении заменяется текущей датой UTC.
func (tx Transaction) Validate() error {
	if tx.Amount <= 0 {
		return errors.New("transaction amount must be positive")
	}
	if int64(tx.Amount) > 99999999999999 {
		return errors.New("transaction amount exceeds NUMERIC(14,2)")
	}
	if strings.TrimSpace(tx.Category) == "" {
		return errors.New("transaction category must not be empty")
	}
	if tx.Date != "" {
		date, err := time.Parse(time.DateOnly, tx.Date)
		if err != nil || date.Year() < 1 {
			return errors.New("transaction date must be YYYY-MM-DD")
		}
	}
	return nil
}

func AddTransaction(tx Transaction) error {
	_, err := CreateTransaction(tx)
	return err
}

// CreateTransaction возвращает вставленную запись с ID из RETURNING.
func CreateTransaction(tx Transaction) (Transaction, error) {
	if err := tx.Validate(); err != nil {
		return Transaction{}, fmt.Errorf("invalid transaction: %w", err)
	}
	if database == nil {
		return Transaction{}, ErrNotInitialized
	}
	if tx.Date == "" {
		tx.Date = time.Now().UTC().Format(time.DateOnly)
	}
	ctx, cancel := operationContext()
	defer cancel()
	dbtx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return Transaction{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer dbtx.Rollback()
	// Блокируем бюджет до конца транзакции, чтобы расходы не проверяли один остаток.
	var limit int64
	err = dbtx.QueryRowContext(ctx, "SELECT (limit_amount*100)::bigint FROM budgets WHERE category=$1 FOR UPDATE", tx.Category).Scan(&limit)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Transaction{}, fmt.Errorf("read budget: %w", err)
	}
	if err == nil {
		var spent int64
		if err := dbtx.QueryRowContext(ctx, "SELECT (COALESCE(SUM(amount),0)*100)::bigint FROM expenses WHERE category=$1", tx.Category).Scan(&spent); err != nil {
			return Transaction{}, fmt.Errorf("read spent: %w", err)
		}
		if int64(tx.Amount) > limit-spent {
			return Transaction{}, ErrBudgetExceeded
		}
	}
	if err := dbtx.QueryRowContext(ctx, `INSERT INTO expenses(amount, category, description, date)
        VALUES($1,$2,$3,$4::date) RETURNING id`, money(tx.Amount), tx.Category, tx.Description, tx.Date).Scan(&tx.ID); err != nil {
		return Transaction{}, fmt.Errorf("insert expense: %w", err)
	}
	if err := dbtx.Commit(); err != nil {
		return Transaction{}, fmt.Errorf("commit expense: %w", err)
	}
	return tx, nil
}

func ListTransactions() ([]Transaction, error) {
	if database == nil {
		return nil, ErrNotInitialized
	}
	ctx, cancel := operationContext()
	defer cancel()
	rows, err := database.QueryContext(ctx, `SELECT id, (amount*100)::bigint, category,
        COALESCE(description,''), to_char(date,'YYYY-MM-DD') FROM expenses ORDER BY date DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list transactions: %w", err)
	}
	defer rows.Close()
	result := make([]Transaction, 0)
	for rows.Next() {
		var tx Transaction
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
