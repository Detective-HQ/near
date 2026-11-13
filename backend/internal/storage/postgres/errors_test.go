package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"universal-defi-intelligence/internal/swap/domain"
)

func TestMapError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code domain.Code
	}{
		{name: "no rows", err: pgx.ErrNoRows, code: domain.CodeNotFound},
		{name: "unique violation", err: &pgconn.PgError{Code: "23505"}, code: domain.CodeConflict},
		{name: "foreign key", err: &pgconn.PgError{Code: "23503"}, code: domain.CodeValidationFailed},
		{name: "check violation", err: &pgconn.PgError{Code: "23514"}, code: domain.CodeValidationFailed},
		{name: "invalid text", err: &pgconn.PgError{Code: "22P02"}, code: domain.CodeValidationFailed},
		{name: "numeric range", err: &pgconn.PgError{Code: "22003"}, code: domain.CodeValidationFailed},
		{name: "serialization", err: &pgconn.PgError{Code: "40001"}, code: domain.CodeConflict},
		{name: "deadlock", err: &pgconn.PgError{Code: "40P01"}, code: domain.CodeConflict},
		{name: "query canceled", err: &pgconn.PgError{Code: "57014"}, code: domain.CodeInternal},
		{name: "too many connections", err: &pgconn.PgError{Code: "53300"}, code: domain.CodeInternal},
		{name: "deadline", err: context.DeadlineExceeded, code: domain.CodeInternal},
		{name: "canceled", err: context.Canceled, code: domain.CodeInternal},
		{name: "unknown", err: errors.New("boom"), code: domain.CodeInternal},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mapped := MapError(test.err)
			if !domain.IsCode(mapped, test.code) {
				t.Fatalf("MapError(%v) code = %s, want %s", test.err, domain.CodeOf(mapped), test.code)
			}
		})
	}
}

func TestMapErrorPreservesTypedDomainError(t *testing.T) {
	original := domain.New(domain.CodeSwapNotFound, "Swap not found.")
	mapped := MapError(original)
	if mapped != original {
		t.Fatalf("MapError replaced typed domain error: %v", mapped)
	}
}

func TestIsRetryable(t *testing.T) {
	retryable := []error{
		&pgconn.PgError{Code: "40001"},
		&pgconn.PgError{Code: "40P01"},
		&pgconn.PgError{Code: "53300"},
		&pgconn.PgError{Code: "57P01"},
		&pgconn.PgError{Code: "57P02"},
		context.DeadlineExceeded,
	}
	for _, err := range retryable {
		if !IsRetryable(err) {
			t.Fatalf("IsRetryable(%v) = false, want true", err)
		}
	}

	notRetryable := []error{
		nil,
		&pgconn.PgError{Code: "23505"},
		context.Canceled,
		errors.New("boom"),
	}
	for _, err := range notRetryable {
		if IsRetryable(err) {
			t.Fatalf("IsRetryable(%v) = true, want false", err)
		}
	}
}
