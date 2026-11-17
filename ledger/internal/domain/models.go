package domain

import (
	"errors"
	"strings"
	"time"
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
	if strings.ContainsRune(b.Category, 0) {
		return errors.New("budget category must not contain NUL")
	}
	if strings.TrimSpace(b.Category) == "" {
		return errors.New("budget category must not be empty")
	}
	return nil
}

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
	if strings.ContainsRune(tx.Category, 0) || strings.ContainsRune(tx.Description, 0) {
		return errors.New("transaction text must not contain NUL")
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

type CategoryTotal struct {
	Category string `json:"category"`
	Amount   int64  `json:"amount"`
}

type ReportSummary struct {
	From       string          `json:"from"`
	To         string          `json:"to"`
	Total      int64           `json:"total"`
	Categories []CategoryTotal `json:"categories"`
}

var ErrInvalidReportDates = errors.New("from and to must be YYYY-MM-DD with from <= to")

// Validatable задаёт контракт проверки данных перед сохранением.
type Validatable interface {
	Validate() error
}

// CheckValid проверяет любой тип, реализующий Validatable.
func CheckValid(v Validatable) error {
	return v.Validate()
}

// ValidationError позволяет HTTP-слою отличить неверные данные от сбоя хранения.
type ValidationError struct{ Err error }

func (e *ValidationError) Error() string { return e.Err.Error() }

func (e *ValidationError) Unwrap() error { return e.Err }

var ErrNotFound = errors.New("not found")

// ErrCacheMiss отличает отсутствие ключа в кеше от сбоя самого кеша.
var ErrCacheMiss = errors.New("cache miss")
