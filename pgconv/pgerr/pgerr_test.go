package pgerr_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/omniaura/go-kit/pgconv/pgerr"
)

func TestIsUniqueViolation(t *testing.T) {
	uniqueErr := &pgconn.PgError{Code: "23505"}
	if !pgerr.IsUniqueViolation(uniqueErr) {
		t.Fatal("IsUniqueViolation() = false for unique violation")
	}

	wrapped := fmt.Errorf("insert user: %w", uniqueErr)
	if !pgerr.IsUniqueViolation(wrapped) {
		t.Fatal("IsUniqueViolation() = false for wrapped unique violation")
	}
}

func TestIsUniqueViolationFalse(t *testing.T) {
	tests := []error{
		nil,
		errors.New("plain error"),
		&pgconn.PgError{Code: "23503"},
	}

	for _, err := range tests {
		if pgerr.IsUniqueViolation(err) {
			t.Fatalf("IsUniqueViolation(%v) = true", err)
		}
	}
}
