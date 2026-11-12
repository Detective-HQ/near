package domain

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Code is a stable domain error identifier. It is safe to expose in API
// responses and is used for metrics and alerting.
type Code string

const (
	// Request and validation errors.
	CodeValidationFailed   Code = "VALIDATION_FAILED"
	CodeInvalidRequest     Code = "INVALID_REQUEST"
	CodeInvalidJSON        Code = "INVALID_JSON"
	CodeInvalidAmount      Code = "INVALID_AMOUNT"
	CodeAmountTooSmall     Code = "AMOUNT_TOO_SMALL"
	CodeInvalidDecimals    Code = "INVALID_DECIMALS"
	CodeTooManyDecimals    Code = "TOO_MANY_DECIMALS"
	CodeUnknownAsset       Code = "UNKNOWN_ASSET"
	CodeInvalidAsset       Code = "INVALID_ASSET"
	CodeInvalidAssetPair   Code = "INVALID_ASSET_PAIR"
	CodeSameAsset          Code = "SAME_ASSET"
	CodeInvalidRecipient   Code = "INVALID_RECIPIENT"
	CodeInvalidRefund      Code = "INVALID_REFUND_ADDRESS"
	CodeRecipientRequired  Code = "RECIPIENT_REQUIRED"
	CodeRefundRequired     Code = "REFUND_ADDRESS_REQUIRED"
	CodeInvalidSlippage    Code = "INVALID_SLIPPAGE"
	CodeInvalidTxHash      Code = "INVALID_TRANSACTION_HASH"
	CodeInvalidDeposit     Code = "INVALID_DEPOSIT_ADDRESS"
	CodeInvalidDepositMode Code = "INVALID_DEPOSIT_MODE"
	CodeInvalidDeadline    Code = "INVALID_DEADLINE"
	CodeWrongNetwork       Code = "WRONG_NETWORK"
	CodeWalletRejected     Code = "WALLET_REJECTED"
	CodeBodyTooLarge       Code = "BODY_TOO_LARGE"

	// Authentication and authorization.
	CodeUnauthorized Code = "UNAUTHORIZED"
	CodeForbidden    Code = "FORBIDDEN"

	// Resource and lifecycle errors.
	CodeNotFound                   Code = "NOT_FOUND"
	CodeQuoteNotFound              Code = "QUOTE_NOT_FOUND"
	CodeSwapNotFound               Code = "SWAP_NOT_FOUND"
	CodeTerminalState              Code = "TERMINAL_STATE"
	CodeInvalidSwapState           Code = "INVALID_SWAP_STATE"
	CodeIdempotencyKeyRequired     Code = "IDEMPOTENCY_KEY_REQUIRED"
	CodeIdempotencyConflict        Code = "IDEMPOTENCY_CONFLICT"
	CodeDepositAlreadyRecorded     Code = "DEPOSIT_ALREADY_RECORDED"
	CodeTransactionAlreadyRecorded Code = "TRANSACTION_ALREADY_RECORDED"
	CodeCatalogueChanged           Code = "CATALOGUE_CHANGED"

	// Routing and quote errors.
	CodeNoRoute      Code = "NO_ROUTE"
	CodeQuoteExpired Code = "QUOTE_EXPIRED"

	// Provider errors.
	CodeProviderNotConfigured   Code = "PROVIDER_NOT_CONFIGURED"
	CodeProviderAuthFailed      Code = "PROVIDER_AUTH_FAILED"
	CodeProviderRateLimited     Code = "PROVIDER_RATE_LIMITED"
	CodeProviderDelayed         Code = "PROVIDER_DELAYED"
	CodeProviderRefunded        Code = "PROVIDER_REFUNDED"
	CodeProviderInvalidStatus   Code = "PROVIDER_INVALID_STATUS"
	CodeProviderInvalidResponse Code = "PROVIDER_INVALID_RESPONSE"
	CodeUnknownProviderStatus   Code = "UNKNOWN_PROVIDER_STATUS"
	CodeRPCTimeout              Code = "RPC_TIMEOUT"

	// Infrastructure and feature gates.
	CodeMarketProviderUnavailable Code = "MARKET_PROVIDER_UNAVAILABLE"
	CodeDemoDisabled              Code = "DEMO_DISABLED"
	CodeLiveExecutionDisabled     Code = "LIVE_EXECUTION_DISABLED"
	CodeLiveExecutionUnavailable  Code = "LIVE_EXECUTION_UNAVAILABLE"
	CodeRateLimited               Code = "RATE_LIMITED"
	CodeInternal                  Code = "INTERNAL"
	CodeInternalError             Code = "INTERNAL_ERROR"
)

// ErrCodeHTTPStatus is the canonical code-to-status mapping from the handoff
// document, extended with the shared HTTP errors used by the API.
var ErrCodeHTTPStatus = map[Code]int{
	CodeValidationFailed:           http.StatusBadRequest,
	CodeInvalidRequest:             http.StatusBadRequest,
	CodeInvalidJSON:                http.StatusBadRequest,
	CodeInvalidAmount:              http.StatusBadRequest,
	CodeAmountTooSmall:             http.StatusUnprocessableEntity,
	CodeInvalidDecimals:            http.StatusInternalServerError,
	CodeTooManyDecimals:            http.StatusBadRequest,
	CodeUnknownAsset:               http.StatusBadRequest,
	CodeInvalidAsset:               http.StatusBadRequest,
	CodeInvalidAssetPair:           http.StatusBadRequest,
	CodeSameAsset:                  http.StatusBadRequest,
	CodeInvalidRecipient:           http.StatusBadRequest,
	CodeInvalidRefund:              http.StatusBadRequest,
	CodeRecipientRequired:          http.StatusBadRequest,
	CodeRefundRequired:             http.StatusBadRequest,
	CodeInvalidSlippage:            http.StatusBadRequest,
	CodeInvalidTxHash:              http.StatusBadRequest,
	CodeInvalidDeposit:             http.StatusBadRequest,
	CodeInvalidDepositMode:         http.StatusBadRequest,
	CodeInvalidDeadline:            http.StatusBadRequest,
	CodeWrongNetwork:               http.StatusBadRequest,
	CodeWalletRejected:             http.StatusBadRequest,
	CodeBodyTooLarge:               http.StatusRequestEntityTooLarge,
	CodeUnauthorized:               http.StatusUnauthorized,
	CodeForbidden:                  http.StatusForbidden,
	CodeNotFound:                   http.StatusNotFound,
	CodeQuoteNotFound:              http.StatusNotFound,
	CodeSwapNotFound:               http.StatusNotFound,
	CodeTerminalState:              http.StatusConflict,
	CodeInvalidSwapState:           http.StatusConflict,
	CodeIdempotencyKeyRequired:     http.StatusBadRequest,
	CodeIdempotencyConflict:        http.StatusConflict,
	CodeDepositAlreadyRecorded:     http.StatusConflict,
	CodeTransactionAlreadyRecorded: http.StatusConflict,
	CodeCatalogueChanged:           http.StatusConflict,
	CodeNoRoute:                    http.StatusUnprocessableEntity,
	CodeQuoteExpired:               http.StatusGone,
	CodeProviderNotConfigured:      http.StatusServiceUnavailable,
	CodeProviderAuthFailed:         http.StatusServiceUnavailable,
	CodeProviderRateLimited:        http.StatusServiceUnavailable,
	CodeProviderDelayed:            http.StatusAccepted,
	CodeProviderRefunded:           http.StatusOK,
	CodeProviderInvalidStatus:      http.StatusBadGateway,
	CodeProviderInvalidResponse:    http.StatusBadGateway,
	CodeUnknownProviderStatus:      http.StatusBadGateway,
	CodeRPCTimeout:                 http.StatusGatewayTimeout,
	CodeMarketProviderUnavailable:  http.StatusServiceUnavailable,
	CodeDemoDisabled:               http.StatusServiceUnavailable,
	CodeLiveExecutionDisabled:      http.StatusServiceUnavailable,
	CodeLiveExecutionUnavailable:   http.StatusServiceUnavailable,
	CodeRateLimited:                http.StatusTooManyRequests,
	CodeInternal:                   http.StatusInternalServerError,
	CodeInternalError:              http.StatusInternalServerError,
}

// HTTPStatus returns the HTTP status associated with the code.
func (c Code) HTTPStatus() int {
	if status, ok := ErrCodeHTTPStatus[c]; ok {
		return status
	}
	return http.StatusInternalServerError
}

// Error is a typed domain error. Message is safe to return to clients; Cause is
// retained for logs and is never serialized.
type Error struct {
	Code       Code
	Message    string
	HTTPStatus int
	RetryAfter time.Duration
	cause      error
}

// New creates a typed error using the code's canonical HTTP status.
func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message, HTTPStatus: code.HTTPStatus()}
}

// Newf creates a typed error with a formatted message.
func Newf(code Code, format string, args ...any) *Error {
	return New(code, fmt.Sprintf(format, args...))
}

// Wrap attaches an underlying cause for logging without exposing it to clients.
func Wrap(code Code, message string, cause error) *Error {
	return &Error{Code: code, Message: message, HTTPStatus: code.HTTPStatus(), cause: cause}
}

// Error implements error.
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Code)
}

// Unwrap exposes the underlying cause to errors.Is and errors.As.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Is matches errors by domain code, so errors.Is(err, domain.New(code, ""))
// works even when the wrapping message differs.
func (e *Error) Is(target error) bool {
	if e == nil {
		return false
	}
	var other *Error
	if !errors.As(target, &other) || other == nil {
		return false
	}
	return e.Code == other.Code
}

// WithHTTPStatus returns a copy with an overridden HTTP status.
func (e *Error) WithHTTPStatus(status int) *Error {
	if e == nil {
		return nil
	}
	clone := *e
	clone.HTTPStatus = status
	return &clone
}

// WithRetryAfter returns a copy with a Retry-After hint.
func (e *Error) WithRetryAfter(delay time.Duration) *Error {
	if e == nil {
		return nil
	}
	clone := *e
	clone.RetryAfter = delay
	return &clone
}

// ErrorBody is the public JSON shape of a domain error.
type ErrorBody struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

// ErrorResponse is the envelope used by every API error response.
type ErrorResponse struct {
	Error     ErrorBody `json:"error"`
	RequestID string    `json:"requestId,omitempty"`
}

// Response builds the public error envelope.
func (e *Error) Response(requestID string) ErrorResponse {
	if e == nil {
		e = New(CodeInternal, "The request could not be completed.")
	}
	return ErrorResponse{
		Error:     ErrorBody{Code: e.Code, Message: e.Error()},
		RequestID: requestID,
	}
}

// AsError extracts a typed domain error from a wrapped chain.
func AsError(err error) (*Error, bool) {
	if err == nil {
		return nil, false
	}
	var target *Error
	if errors.As(err, &target) && target != nil {
		return target, true
	}
	return nil, false
}

// CodeOf returns the domain code or INTERNAL for untyped errors.
func CodeOf(err error) Code {
	if typed, ok := AsError(err); ok {
		return typed.Code
	}
	return CodeInternal
}

// HTTPStatusOf returns the HTTP status for any error.
func HTTPStatusOf(err error) int {
	if typed, ok := AsError(err); ok {
		if typed.HTTPStatus != 0 {
			return typed.HTTPStatus
		}
		return typed.Code.HTTPStatus()
	}
	return http.StatusInternalServerError
}

// IsCode reports whether err carries the given domain code.
func IsCode(err error, code Code) bool {
	typed, ok := AsError(err)
	return ok && typed.Code == code
}
