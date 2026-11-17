package api

import (
	"bytes"
	"context"
	"io"
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
			// Interrupt network body reads when the request is canceled. Wait for
			// the callback before resetting the deadline for keep-alive reuse.
			controller := http.NewResponseController(w)
			finished := make(chan struct{})
			stop := context.AfterFunc(ctx, func() {
				_ = controller.SetReadDeadline(time.Now())
				close(finished)
			})
			defer func() {
				if !stop() {
					<-finished
				}
				_ = controller.SetReadDeadline(time.Time{})
			}()
			// Read the bounded body while the deadline is still active, even for
			// routes that ignore it. Otherwise net/http may drain it after return.
			if r.Body != nil {
				raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
				_ = r.Body.Close()
				if err != nil {
					if ctx.Err() != nil {
						writeError(w, TimeoutStatus, "request timeout or canceled")
					} else {
						writeError(w, http.StatusBadRequest, "invalid request body")
					}
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(raw))
			}
			response := &bufferedResponse{header: make(http.Header)}
			next.ServeHTTP(response, r.WithContext(ctx))
			deadline, _ := ctx.Deadline()
			if ctx.Err() != nil || !time.Now().Before(deadline) {
				writeError(w, TimeoutStatus, "request timeout or canceled")
				return
			}
			for key, values := range response.header {
				w.Header()[key] = values
			}
			status := response.status
			if status == 0 {
				status = http.StatusOK
			}
			w.WriteHeader(status)
			_, _ = w.Write(response.body.Bytes())
		})
	}
}

// API responses are buffered so an expired request cannot publish a success.
// These endpoints return JSON responses and do not stream.
type bufferedResponse struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (w *bufferedResponse) Header() http.Header { return w.header }
func (w *bufferedResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *bufferedResponse) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(data)
}
