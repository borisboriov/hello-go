package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/borisboriov/hello-go/ledger/internal/domain"
)

type Service struct {
	repos        domain.Repositories
	transactions domain.TransactionRunner
	reports      domain.ReportRepository
	cache        domain.ReportCache
}

func New(repos domain.Repositories, transactions domain.TransactionRunner, reports domain.ReportRepository, cache domain.ReportCache) *Service {
	return &Service{repos: repos, transactions: transactions, reports: reports, cache: cache}
}

func (s *Service) SetBudget(ctx context.Context, b domain.Budget) error {
	if err := b.Validate(); err != nil {
		return &domain.ValidationError{Err: err}
	}
	return s.repos.Budgets.SetBudget(ctx, b)
}

func (s *Service) ListBudgets(ctx context.Context) ([]domain.Budget, error) {
	return s.repos.Budgets.ListBudgets(ctx)
}

func (s *Service) ListTransactions(ctx context.Context) ([]domain.Transaction, error) {
	return s.repos.Expenses.ListTransactions(ctx)
}

func (s *Service) AddTransaction(ctx context.Context, tx domain.Transaction) error {
	_, err := s.CreateTransaction(ctx, tx)
	return err
}

func (s *Service) CreateTransaction(ctx context.Context, tx domain.Transaction) (domain.Transaction, error) {
	if err := tx.Validate(); err != nil {
		return domain.Transaction{}, &domain.ValidationError{Err: err}
	}
	if tx.Date == "" {
		tx.Date = time.Now().UTC().Format(time.DateOnly)
	}
	var result domain.Transaction
	err := s.transactions.WithinTransaction(ctx, func(repos domain.Repositories) error {
		budget, err := repos.Budgets.FindBudget(ctx, tx.Category)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		if err == nil {
			spent, err := repos.Expenses.SumByCategory(ctx, tx.Category)
			if err != nil {
				return err
			}
			if int64(tx.Amount) > int64(budget.Limit)-spent {
				return domain.ErrBudgetExceeded
			}
		}
		result, err = repos.Expenses.InsertTransaction(ctx, tx)
		return err
	})
	if err != nil {
		return domain.Transaction{}, err
	}
	return result, nil
}

func (s *Service) LoadBudgets(ctx context.Context, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("не удалось прочитать бюджеты: %w", err)
	}
	var loaded []domain.Budget
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("не удалось разобрать JSON бюджетов: %w", err)
	}
	for i, b := range loaded {
		if err := s.SetBudget(ctx, b); err != nil {
			return fmt.Errorf("не удалось загрузить бюджет %d: %w", i+1, err)
		}
	}
	return nil
}

func (s *Service) GetReportSummary(ctx context.Context, from, to string) (domain.ReportSummary, error) {
	start, errFrom := time.Parse(time.DateOnly, from)
	end, errTo := time.Parse(time.DateOnly, to)
	if errFrom != nil || errTo != nil || start.Year() < 1 || end.Year() < 1 || start.After(end) {
		return domain.ReportSummary{}, domain.ErrInvalidReportDates
	}
	key := "report:summary:" + from + ":" + to
	if s.cache != nil {
		result, err := s.cache.Get(ctx, key)
		if err == nil && result.From == from && result.To == to && result.Categories != nil {
			log.Printf("report cache hit key=%s", key)
			return result, nil
		}
	}
	log.Printf("report cache miss key=%s", key)
	result, err := s.reports.GetReportSummary(ctx, from, to)
	if err != nil {
		return domain.ReportSummary{}, err
	}
	if s.cache != nil {
		if err := s.cache.Set(ctx, key, result); err != nil {
			log.Printf("report cache write failed: %v", err)
		}
	}
	return result, nil
}
