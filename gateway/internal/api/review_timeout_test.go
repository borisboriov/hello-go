package api

import (
	"bufio"
	"context"
	"fmt"
	"github.com/borisboriov/hello-go/ledger"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type lateSuccessService struct{ ledger.Service }

func (lateSuccessService) GetReportSummary(ctx context.Context, from, to string) (ledger.ReportSummary, error) {
	<-ctx.Done()
	// Service logs a cache write error but returns its already computed result.
	return ledger.ReportSummary{From: from, To: to, Total: 123}, nil
}
func TestReviewDeadlineSuccessReturns504(t *testing.T) {
	h := NewHandlerWithTimeout(lateSuccessService{}, 20*time.Millisecond)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/reports/summary?from=2026-01-01&to=2026-01-31", nil))
	if w.Code != http.StatusGatewayTimeout {
		t.Fatalf("deadline reached: want 504, got %d, body=%s", w.Code, w.Body)
	}
}

func TestSlowBodyDeadline(t *testing.T) {
	for _, route := range []string{"POST /api/transactions", "GET /api/budgets", "PUT /api/budgets", "GET /ping"} {
		t.Run(route, func(t *testing.T) {
			server := httptest.NewServer(NewHandlerWithTimeout(lateSuccessService{}, 30*time.Millisecond))
			defer server.Close()
			conn, err := net.Dial("tcp", strings.TrimPrefix(server.URL, "http://"))
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(time.Second))
			fmt.Fprintf(conn, "%s HTTP/1.1\r\nHost: localhost\r\nContent-Length: 100\r\n\r\n{", route)
			response, err := http.ReadResponse(bufio.NewReader(conn), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != 504 {
				t.Fatalf("status=%d", response.StatusCode)
			}
		})
	}
}

func TestBodyKeepAlive(t *testing.T) {
	server := httptest.NewServer(NewHandlerWithTimeout(lateSuccessService{}, time.Second))
	defer server.Close()
	conn, err := net.Dial("tcp", strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	reader := bufio.NewReader(conn)
	for i := 0; i < 2; i++ {
		fmt.Fprint(conn, "GET /ping HTTP/1.1\r\nHost: localhost\r\nContent-Length: 2\r\n\r\n{}")
		response, err := http.ReadResponse(reader, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != 200 {
			t.Fatalf("status=%d err=%v", response.StatusCode, err)
		}
	}
}
