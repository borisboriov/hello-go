package api

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"
)

// Logging записывает метод, путь и длительность каждого запроса.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		defer func() {
			log.Printf("method=%s path=%s duration=%s", r.Method, r.URL.Path, time.Since(started))
		}()
		next.ServeHTTP(w, r)
	})
}

// DefaultRequestTimeout — таймаут запроса по умолчанию; переопределяется
// переменной окружения REQUEST_TIMEOUT (формат time.ParseDuration: 2s, 50ms, 1ms).
const DefaultRequestTimeout = 2 * time.Second

// TimeoutStatus — единый код ответа при таймауте или отмене запроса.
const TimeoutStatus = http.StatusGatewayTimeout

// RequestTimeoutFromEnv читает REQUEST_TIMEOUT, при ошибке возвращает значение по умолчанию.
func RequestTimeoutFromEnv() time.Duration {
	raw := os.Getenv("REQUEST_TIMEOUT")
	if raw == "" {
		return DefaultRequestTimeout
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		log.Printf("invalid REQUEST_TIMEOUT=%q, using %s", raw, DefaultRequestTimeout)
		return DefaultRequestTimeout
	}
	return d
}

// Timeout ограничивает время обработки запроса: создаёт context.WithTimeout и
// передаёт его обработчикам через r.WithContext. Обработчики, получив ошибку
// context.DeadlineExceeded/Canceled, отвечают TimeoutStatus с JSON-ошибкой.
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
