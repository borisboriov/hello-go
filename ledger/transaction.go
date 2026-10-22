package ledger

import (
	"errors"
	"fmt"
	"strings"
)

// Transaction представляет транзакцию. Amount хранит сумму в копейках.
type Transaction struct {
	ID          int
	Amount      int
	Category    string
	Description string
	Date        string
}

var transactions = make([]Transaction, 0)

// ErrBudgetExceeded позволяет вызывающему коду распознать превышение бюджета.
var ErrBudgetExceeded = errors.New("budget exceeded")

// Validate проверяет поля транзакции, не изменяя данные и не логируя ошибки.
// Date хранится строкой; проверка её формата в этой задаче не выполняется.
func (tx Transaction) Validate() error {
	if tx.Amount <= 0 {
		return errors.New("transaction amount must be positive")
	}
	if strings.TrimSpace(tx.Category) == "" {
		return errors.New("transaction category must not be empty")
	}
	return nil
}

// AddTransaction валидирует транзакцию, проверяет бюджет и назначает ID.
func AddTransaction(tx Transaction) error {
	if err := tx.Validate(); err != nil {
		return fmt.Errorf("invalid transaction: %w", err)
	}
	if b, ok := budgets[tx.Category]; ok {
		total := tx.Amount
		for _, saved := range transactions {
			if saved.Category == tx.Category {
				total += saved.Amount
			}
		}
		if total > b.Limit {
			return ErrBudgetExceeded
		}
	}
	tx.ID = len(transactions) + 1
	transactions = append(transactions, tx)
	return nil
}

// ListTransactions возвращает копию хранилища.
func ListTransactions() []Transaction {
	result := make([]Transaction, len(transactions))
	copy(result, transactions)
	return result
}
