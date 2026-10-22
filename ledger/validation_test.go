package main

import (
	"strings"
	"testing"
)

func TestCheckValid(t *testing.T) {
	tests := []struct {
		name    string
		value   Validatable
		wantErr string
	}{
		{"valid transaction", Transaction{Amount: 1, Category: "Книги"}, ""},
		{"zero amount", Transaction{Amount: 0, Category: "Книги"}, "amount"},
		{"negative amount", Transaction{Amount: -1, Category: "Книги"}, "amount"},
		{"empty transaction category", Transaction{Amount: 1}, "category"},
		{"blank transaction category", Transaction{Amount: 1, Category: " \t\n"}, "category"},
		{"valid budget", Budget{Limit: 1, Category: "Книги"}, ""},
		{"zero limit", Budget{Limit: 0, Category: "Книги"}, "limit"},
		{"negative limit", Budget{Limit: -1, Category: "Книги"}, "limit"},
		{"empty budget category", Budget{Limit: 1}, "category"},
		{"blank budget category", Budget{Limit: 1, Category: " \t\n"}, "category"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckValid(tt.value)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("CheckValid() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("CheckValid() = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}
