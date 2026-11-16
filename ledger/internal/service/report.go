package service

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/borisboriov/hello-go/ledger/internal/domain"
)

// HeartbeatInterval задаёт частоту сообщений о ходе расчёта отчёта.
var HeartbeatInterval = 400 * time.Millisecond

// calculateReport считает суммы по категориям параллельно: одна горутина на
// категорию. Все обращения к БД получают ctx, поэтому отмена прерывает SQL.
func (s *Service) calculateReport(ctx context.Context, from, to string) (domain.ReportSummary, error) {
	stopHeartbeat := startHeartbeat(ctx, from, to)
	defer stopHeartbeat()

	categories, err := s.reports.ReportCategories(ctx, from, to)
	if err != nil {
		return domain.ReportSummary{}, err
	}

	// Каждая горутина пишет только в свой индекс: общих записей нет, гонок нет.
	sums := make([]int64, len(categories))
	errs := make([]error, len(categories))
	var wg sync.WaitGroup
	for i, category := range categories {
		wg.Add(1)
		go func(i int, category string) {
			defer wg.Done()
			if err := ctx.Err(); err != nil { // уже отменено: не идём в БД
				errs[i] = err
				return
			}
			sums[i], errs[i] = s.reports.SumByCategoryPeriod(ctx, category, from, to)
		}(i, category)
	}
	wg.Wait()

	if err := ctx.Err(); err != nil {
		log.Printf("report: calculation aborted by ctx.Done: %v", err)
		return domain.ReportSummary{}, err
	}
	result := domain.ReportSummary{From: from, To: to, Categories: make([]domain.CategoryTotal, 0, len(categories))}
	for i, category := range categories {
		if errs[i] != nil {
			return domain.ReportSummary{}, errs[i]
		}
		result.Total += sums[i]
		result.Categories = append(result.Categories, domain.CategoryTotal{Category: category, Amount: sums[i]})
	}
	return result, nil
}

// startHeartbeat запускает фоновую горутину с time.Ticker. Возвращённая
// функция останавливает тикер и дожидается завершения горутины.
func startHeartbeat(ctx context.Context, from, to string) func() {
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		ticker := time.NewTicker(HeartbeatInterval)
		defer ticker.Stop()
		log.Printf("report: calculation started from=%s to=%s", from, to)
		for {
			select {
			case <-ticker.C:
				log.Println("report: calculating...")
			case <-ctx.Done():
				log.Printf("report: heartbeat stopped by ctx.Done: %v", ctx.Err())
				return
			case <-done:
				log.Println("report: heartbeat stopped, calculation finished")
				return
			}
		}
	}()
	return func() {
		close(done)
		<-finished
	}
}
