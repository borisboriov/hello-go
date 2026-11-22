package service

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/borisboriov/hello-go/ledger/internal/domain"
)

const (
	defaultImportWorkers = 4
	maxImportWorkers     = 32
)

type importJob struct {
	index       int
	transaction domain.Transaction
}

type importResult struct {
	index int
	err   error
}

func (s *Service) ImportTransactions(ctx context.Context, transactions []domain.Transaction, workers int) (domain.BulkImportSummary, error) {
	if workers <= 0 {
		workers = defaultImportWorkers
	}
	workers = min(workers, maxImportWorkers, len(transactions))
	jobs := make(chan importJob)
	results := make(chan importResult)
	collected := make(chan []domain.BulkImportError)
	var accepted, rejected atomic.Int64
	go func() {
		failures := make([]domain.BulkImportError, 0)
		for result := range results {
			if result.err == nil {
				accepted.Add(1)
				continue
			}
			rejected.Add(1)
			failures = append(failures, domain.BulkImportError{Index: result.index, Err: result.err})
		}
		sort.Slice(failures, func(i, j int) bool { return failures[i].Index < failures[j].Index })
		collected <- failures
	}()

	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-jobs:
					if !ok || ctx.Err() != nil {
						return
					}
					_, err := s.createTransaction(ctx, job.transaction)
					// The collector drains until all workers finish, including on cancellation.
					// A committed expense must be counted even if the deadline has passed.
					results <- importResult{index: job.index, err: err}
				}
			}
		}()
	}

sendJobs:
	for index, transaction := range transactions {
		select {
		case <-ctx.Done():
			break sendJobs
		case jobs <- importJob{index: index, transaction: transaction}:
		}
	}
	close(jobs)
	wg.Wait()
	close(results)
	failures := <-collected
	return domain.BulkImportSummary{
		Accepted: accepted.Load(),
		Rejected: rejected.Load(),
		Errors:   failures,
	}, ctx.Err()
}
