package nearoneclick

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"universal-defi-intelligence/internal/swap/domain"
)

func mapHTTPError(status int, header http.Header) error {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return domain.New(domain.CodeProviderAuthFailed,
			"The swap provider rejected server credentials.")
	case status == http.StatusTooManyRequests:
		err := domain.New(domain.CodeProviderRateLimited,
			"The swap provider is rate-limiting requests. Try again shortly.")
		if delay := parseRetryAfter(header.Get("Retry-After"), time.Now()); delay > 0 {
			return err.WithRetryAfter(delay)
		}
		return err
	case status >= 500:
		return domain.New(domain.CodeProviderDelayed,
			"The swap provider is temporarily unavailable.")
	default:
		return domain.New(domain.CodeProviderRequestRejected,
			"The swap provider could not complete this request.")
	}
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if deadline, err := http.ParseTime(value); err == nil {
		if delay := deadline.Sub(now); delay > 0 {
			return delay
		}
	}
	return 0
}
