package main

import (
	"encoding/json"
	"fmt"
	"io"
)

type Budget struct {
	Category string `json:"category"`
	Limit    int    `json:"limit"` // В копейках, как и сумма транзакции.
}

var budgets = make(map[string]Budget)

func SetBudget(b Budget) {
	budgets[b.Category] = b
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
	for _, b := range loaded {
		SetBudget(b)
	}
	return nil
}
