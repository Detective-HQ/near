package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// MaxHTTPResponseBytes bounds Horizon/Aptos JSON responses.
const MaxHTTPResponseBytes = 2 << 20

// HTTPFetcher performs bounded HTTPS GETs with failover.
type HTTPFetcher struct {
	URLs    []string
	Client  *http.Client
	Timeout time.Duration
}

// StatusError is returned for non-200 HTTP responses.
type StatusError struct {
	StatusCode int
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Body)
}

// NotFound reports whether the response was HTTP 404.
func (e *StatusError) NotFound() bool {
	return e.StatusCode == http.StatusNotFound
}

// Get fetches path from each base URL until one succeeds.
func (f *HTTPFetcher) Get(ctx context.Context, path string, out any) error {
	if len(f.URLs) == 0 {
		return fmt.Errorf("%w: no HTTP endpoints", ErrInvalidRequest)
	}
	var lastErr error
	for _, base := range f.URLs {
		if err := ctx.Err(); err != nil {
			return err
		}
		status, payload, err := f.getOne(ctx, base+path)
		if err == nil {
			if out == nil || len(payload) == 0 || string(payload) == "null" {
				return nil
			}
			if err := json.Unmarshal(payload, out); err != nil {
				return fmt.Errorf("%w: decode response: %v", ErrInvalidResponse, err)
			}
			return nil
		}
		if status != nil && status.StatusCode >= 400 && status.StatusCode < 500 {
			return status
		}
		lastErr = err
	}
	return fmt.Errorf("all HTTP endpoints failed: %w", lastErr)
}

func (f *HTTPFetcher) getOne(ctx context.Context, url string) (*StatusError, []byte, error) {
	attemptCtx := ctx
	var cancel context.CancelFunc
	if f.Timeout > 0 {
		attemptCtx, cancel = context.WithTimeout(ctx, f.Timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "application/json")
	client := f.Client
	if client == nil {
		client = &http.Client{
			Timeout: f.Timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, MaxHTTPResponseBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if len(payload) > MaxHTTPResponseBytes {
		return nil, nil, fmt.Errorf("%w: response exceeds %d bytes", ErrInvalidResponse, MaxHTTPResponseBytes)
	}
	if resp.StatusCode != http.StatusOK {
		status := &StatusError{StatusCode: resp.StatusCode, Body: truncateBody(payload)}
		return status, nil, status
	}
	return nil, payload, nil
}

func truncateBody(body []byte) string {
	const max = 256
	if len(body) <= max {
		return string(body)
	}
	return string(body[:max]) + "..."
}
