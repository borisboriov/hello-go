package pg

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/borisboriov/hello-go/ledger/internal/domain"
)

type Transactions struct{ db *sql.DB }

func NewTransactions(db *sql.DB) *Transactions { return &Transactions{db: db} }

var _ domain.TransactionRunner = (*Transactions)(nil)

func (t *Transactions) WithinTransaction(ctx context.Context, fn func(domain.Repositories) error) error {
	tx, err := t.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	store := &Store{db: tx}
	if err := fn(domain.Repositories{Budgets: store, Expenses: store}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit expense: %w", err)
	}
	return nil
}
