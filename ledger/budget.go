package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

type Budget struct {
	Category string `json:"category"`
	Limit    int    `json:"limit"` // В копейках, как и сумма транзакции.
}

var budgets = make(map[string]Budget)

// Validate проверяет поля бюджета, не изменяя данные и не логируя ошибки.
func (b Budget) Validate() error {
	if b.Limit <= 0 {
		return errors.New("budget limit must be positive")
	}
	if strings.TrimSpace(b.Category) == "" {
		return errors.New("budget category must not be empty")
	}
	return nil
}

// SetBudget добавляет или обновляет бюджет только после успешной валидации.
func SetBudget(b Budget) error {
	if err := b.Validate(); err != nil {
		return fmt.Errorf("invalid budget: %w", err)
	}
	budgets[b.Category] = b
	return nil
}

// ListBudgets возвращает копию бюджетов в стабильном порядке категорий.
func ListBudgets() []Budget {
	result := make([]Budget, 0, len(budgets))
	for _, b := range budgets {
		result = append(result, b)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Category < result[j].Category })
	return result
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
