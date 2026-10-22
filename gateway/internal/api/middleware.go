package api

import (
	"log"
	"net/http"
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
