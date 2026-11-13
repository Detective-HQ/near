package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCodeHTTPStatus(t *testing.T) {
	tests := []struct {
		code Code
		want int
	}{
		{CodeValidationFailed, http.StatusBadRequest},
		{CodeWalletRejected, http.StatusBadRequest},
		{CodeUnauthorized, http.StatusUnauthorized},
		{CodeForbidden, http.StatusForbidden},
		{CodeNotFound, http.StatusNotFound},
		{CodeQuoteExpired, http.StatusGone},
		{CodeIdempotencyConflict, http.StatusConflict},
		{CodeBodyTooLarge, http.StatusRequestEntityTooLarge},
		{CodeNoRoute, http.StatusUnprocessableEntity},
		{CodeRateLimited, http.StatusTooManyRequests},
		{CodeInternal, http.StatusInternalServerError},
		{CodeUnknownProviderStatus, http.StatusBadGateway},
		{CodeProviderAuthFailed, http.StatusServiceUnavailable},
		{CodeRPCTimeout, http.StatusGatewayTimeout},
		{CodeProviderDelayed, http.StatusAccepted},
		{CodeProviderRefunded, http.StatusOK},
	}

	for _, test := range tests {
		t.Run(string(test.code), func(t *testing.T) {
			if got := test.code.HTTPStatus(); got != test.want {
				t.Fatalf("%s.HTTPStatus() = %d, want %d", test.code, got, test.want)
			}
		})
	}

	if got := Code("NOT_A_REAL_CODE").HTTPStatus(); got != http.StatusInternalServerError {
		t.Fatalf("unknown code status = %d, want 500", got)
	}
}

func TestErrorIsMatchesByCode(t *testing.T) {
	err := New(CodeNoRoute, "no route")
	if !errors.Is(err, New(CodeNoRoute, "different message")) {
		t.Fatal("expected errors.Is to match the same code")
	}
	if errors.Is(err, New(CodeQuoteExpired, "no route")) {
		t.Fatal("did not expect a different code to match")
	}
}

func TestWrapPreservesCause(t *testing.T) {
	cause := errors.New("socket closed")
	err := Wrap(CodeRPCTimeout, "upstream timed out", cause)
	if !errors.Is(err, cause) {
		t.Fatal("expected wrapped cause to be discoverable")
	}
	if !IsCode(err, CodeRPCTimeout) {
		t.Fatal("expected RPC code")
	}
	if err.Error() != "upstream timed out" {
		t.Fatalf("Error() = %q, want the safe message only", err.Error())
	}
}

func TestCodeOfAndHTTPStatusOfUntypedError(t *testing.T) {
	err := errors.New("boom")
	if got := CodeOf(err); got != CodeInternal {
		t.Fatalf("CodeOf(untyped) = %s, want INTERNAL", got)
	}
	if got := HTTPStatusOf(err); got != http.StatusInternalServerError {
		t.Fatalf("HTTPStatusOf(untyped) = %d, want 500", got)
	}
	if AsErrorOfNil() {
		t.Fatal("nil error must not produce a typed error")
	}
}

func AsErrorOfNil() bool {
	_, ok := AsError(nil)
	return ok
}

func TestErrorMetadataCopiesDoNotMutateOriginal(t *testing.T) {
	original := New(CodeProviderRateLimited, "slow down")
	withRetry := original.WithRetryAfter(30 * time.Second)
	if original.RetryAfter != 0 {
		t.Fatal("WithRetryAfter mutated the original")
	}
	if withRetry.RetryAfter != 30*time.Second {
		t.Fatalf("RetryAfter = %s, want 30s", withRetry.RetryAfter)
	}
	if withRetry == original {
		t.Fatal("expected a copy")
	}

	overridden := original.WithHTTPStatus(http.StatusTooManyRequests)
	if original.HTTPStatus != http.StatusServiceUnavailable {
		t.Fatal("WithHTTPStatus mutated the original")
	}
	if overridden.HTTPStatus != http.StatusTooManyRequests {
		t.Fatalf("overridden status = %d, want 429", overridden.HTTPStatus)
	}
}

func TestErrorResponseJSON(t *testing.T) {
	err := New(CodeNoRoute, "No executable route found.")
	response := err.Response("req-123")
	encoded, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		t.Fatalf("marshal: %v", marshalErr)
	}
	want := `{"error":{"code":"NO_ROUTE","message":"No executable route found."},"requestId":"req-123"}`
	if string(encoded) != want {
		t.Fatalf("JSON = %s, want %s", encoded, want)
	}
}

func TestErrorEmptyMessageFallsBackToCode(t *testing.T) {
	err := New(CodeInternal, "")
	if err.Error() != "INTERNAL" {
		t.Fatalf("Error() = %q, want INTERNAL", err.Error())
	}
}

func TestNewf(t *testing.T) {
	err := Newf(CodeInvalidAmount, "amount %s is invalid", "1e18")
	if err.Error() != "amount 1e18 is invalid" {
		t.Fatalf("Error() = %q", err.Error())
	}
}

func TestIsCodeWithWrappedError(t *testing.T) {
	typed := New(CodeQuoteExpired, "expired")
	wrapped := fmt.Errorf("handler: %w", typed)
	if !IsCode(wrapped, CodeQuoteExpired) {
		t.Fatal("expected wrapped typed error to match")
	}
	if IsCode(wrapped, CodeNoRoute) {
		t.Fatal("did not expect a different code to match")
	}
}

func TestResponseNeverIncludesCause(t *testing.T) {
	err := Wrap(CodeInternal, "The request could not be completed.", errors.New("database password leaked"))
	encoded, marshalErr := json.Marshal(err.Response("req-1"))
	if marshalErr != nil {
		t.Fatalf("marshal: %v", marshalErr)
	}
	if strings.Contains(string(encoded), "database password") {
		t.Fatalf("response leaked the cause: %s", encoded)
	}
}

func TestNilErrorMethodsAreSafe(t *testing.T) {
	var err *Error
	if got := err.Error(); got != "" {
		t.Fatalf("nil Error() = %q, want empty", got)
	}
	if err.Unwrap() != nil {
		t.Fatal("nil Unwrap() should be nil")
	}
	if err.Is(New(CodeInternal, "")) {
		t.Fatal("nil error should not match a code")
	}
	if err.WithHTTPStatus(http.StatusBadRequest) != nil {
		t.Fatal("nil WithHTTPStatus() should return nil")
	}
	if err.WithRetryAfter(time.Second) != nil {
		t.Fatal("nil WithRetryAfter() should return nil")
	}
	response := err.Response("req-1")
	if response.Error.Code != CodeInternal {
		t.Fatalf("nil Response() code = %s, want INTERNAL", response.Error.Code)
	}
}

func TestHTTPStatusOfTypedErrors(t *testing.T) {
	withStatus := &Error{Code: CodeNoRoute, Message: "no route", HTTPStatus: http.StatusTeapot}
	if got := HTTPStatusOf(withStatus); got != http.StatusTeapot {
		t.Fatalf("HTTPStatusOf(explicit) = %d, want 418", got)
	}

	withoutStatus := &Error{Code: CodeNoRoute, Message: "no route"}
	if got := HTTPStatusOf(withoutStatus); got != http.StatusUnprocessableEntity {
		t.Fatalf("HTTPStatusOf(canonical) = %d, want 422", got)
	}
}

func TestIsIgnoresNonDomainTarget(t *testing.T) {
	err := New(CodeInternal, "boom")
	if err.Is(errors.New("boom")) {
		t.Fatal("Is should not match a non-domain error")
	}
}
