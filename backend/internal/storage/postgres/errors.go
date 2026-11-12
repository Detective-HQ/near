// Package postgres contains the PostgreSQL connection pool, transaction helper,
// embedded migration runner, and database error mapping.
package postgres

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"universal-defi-intelligence/internal/swap/domain"
)

// PostgreSQL SQLSTATE codes that the storage layer translates into stable
// domain errors. Keeping the mapping in one place means a raw driver error can
// never leak through the API.
const (
	sqlStateUniqueViolation       = "23505"
	sqlStateForeignKeyViolation   = "23503"
	sqlStateCheckViolation        = "23514"
	sqlStateInvalidText           = "22P02"
	sqlStateNumericOutOfRange     = "22003"
	sqlStateInvalidDatetime       = "22007"
	sqlStateDatetimeFieldOverflow = "22008"
	sqlStateSerializationFailure  = "40001"
	sqlStateDeadlockDetected      = "40P01"
	sqlStateQueryCanceled         = "57014"
	sqlStateTooManyConnections    = "53300"
	sqlStateAdminShutdown         = "57P01"
	sqlStateCrashShutdown         = "57P02"
)

// MapError converts a pgx/pgconn error into a typed domain error.
//
// It is deliberately conservative: an unrecognised driver error becomes
// INTERNAL and keeps the original cause for logs. Domain errors already in the
// chain are returned unchanged so callers can add resource-specific codes such
// as QUOTE_NOT_FOUND and SWAP_NOT_FOUND.
func MapError(err error) error {
	if err == nil {
		return nil
	}

	var typed *domain.Error
	if errors.As(err, &typed) && typed != nil {
		return err
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case sqlStateUniqueViolation:
			return domain.Wrap(domain.CodeConflict, "The record conflicts with an existing record.", err)
		case sqlStateForeignKeyViolation:
			return domain.Wrap(domain.CodeValidationFailed, "A referenced record does not exist.", err)
		case sqlStateCheckViolation, sqlStateInvalidText, sqlStateNumericOutOfRange,
			sqlStateInvalidDatetime, sqlStateDatetimeFieldOverflow:
			return domain.Wrap(domain.CodeValidationFailed, "The record failed a database validation rule.", err)
		case sqlStateSerializationFailure, sqlStateDeadlockDetected:
			return domain.Wrap(domain.CodeConflict, "The database transaction conflicted with another transaction; retry the operation.", err).
				WithRetryAfter(50 * time.Millisecond)
		case sqlStateQueryCanceled:
			return domain.Wrap(domain.CodeInternal, "The database query was cancelled.", err)
		case sqlStateTooManyConnections, sqlStateAdminShutdown, sqlStateCrashShutdown:
			return domain.Wrap(domain.CodeInternal, "The database is temporarily unavailable.", err).
				WithRetryAfter(time.Second)
		}
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Wrap(domain.CodeNotFound, "The requested record was not found.", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return domain.Wrap(domain.CodeInternal, "The database operation timed out.", err)
	}
	if errors.Is(err, context.Canceled) {
		return domain.Wrap(domain.CodeInternal, "The database operation was cancelled.", err)
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return domain.Wrap(domain.CodeInternal, "The database operation timed out.", err)
	}

	return domain.Wrap(domain.CodeInternal, "The database operation failed.", err)
}

// IsRetryable reports whether the error represents a transient database
// conflict that is safe to retry with the same transaction.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case sqlStateSerializationFailure, sqlStateDeadlockDetected,
			sqlStateTooManyConnections, sqlStateAdminShutdown, sqlStateCrashShutdown:
			return true
		}
	}
	return errors.Is(err, context.DeadlineExceeded)
}
