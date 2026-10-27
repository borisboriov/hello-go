package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type Budget struct {
	Category string `json:"category"`
	Limit    int    `json:"limit"` // В копейках, как и сумма транзакции.
}

// Validate проверяет поля бюджета, не изменяя данные и не логируя ошибки.
func (b Budget) Validate() error {
	if b.Limit <= 0 {
		return errors.New("budget limit must be positive")
	}
	if int64(b.Limit) > 99999999999999 {
		return errors.New("budget limit exceeds NUMERIC(14,2)")
	}
	if strings.TrimSpace(b.Category) == "" {
		return errors.New("budget category must not be empty")
	}
	return nil
}

func SetBudget(b Budget) error {
	if err := b.Validate(); err != nil {
		return fmt.Errorf("invalid budget: %w", err)
	}
	if database == nil {
		return ErrNotInitialized
	}
	ctx, cancel := operationContext()
	defer cancel()
	_, err := database.ExecContext(ctx, `INSERT INTO budgets(category, limit_amount) VALUES($1,$2)
	    ON CONFLICT(category) DO UPDATE SET limit_amount=EXCLUDED.limit_amount`, b.Category, money(b.Limit))
	if err != nil {
		return fmt.Errorf("upsert budget: %w", err)
	}
	return nil
}

func ListBudgets() ([]Budget, error) {
	if database == nil {
		return nil, ErrNotInitialized
	}
	ctx, cancel := operationContext()
	defer cancel()
	rows, err := database.QueryContext(ctx, "SELECT category, (limit_amount*100)::bigint FROM budgets ORDER BY category")
	if err != nil {
		return nil, fmt.Errorf("list budgets: %w", err)
	}
	defer rows.Close()
	result := make([]Budget, 0)
	for rows.Next() {
		var b Budget
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

func LoadBudgets(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("не удалось прочитать бюджеты: %w", err)
	}
	var loaded []Budget
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("не удалось разобрать JSON бюджетов: %w", err)
	}
	for i, b := range loaded {
		if err := SetBudget(b); err != nil {
			return fmt.Errorf("не удалось загрузить бюджет %d: %w", i+1, err)
		}
	}
	return nil
}
