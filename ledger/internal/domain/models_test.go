package domain

import "testing"

func TestRejectNUL(t *testing.T) {
	for _, value := range []Validatable{Budget{Category: "a\x00", Limit: 1}, Transaction{Category: "a\x00", Amount: 1}, Transaction{Category: "a", Description: "b\x00", Amount: 1}} {
		if value.Validate() == nil {
			t.Fatalf("accepted NUL: %#v", value)
		}
	}
}
