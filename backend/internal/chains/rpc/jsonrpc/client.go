// Package jsonrpc provides a small JSON-RPC 2.0 client with bounded responses,
// per-attempt timeouts, and endpoint failover.
package jsonrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

// MaxResponseBytes bounds every RPC response body.
const MaxResponseBytes = 2 << 20

var (
	// ErrNoEndpoints is returned when a client has no URLs configured.
	ErrNoEndpoints = errors.New("jsonrpc: no endpoints configured")
	// ErrResponseTooLarge is returned when a response exceeds MaxResponseBytes.
	ErrResponseTooLarge = errors.New("jsonrpc: response too large")
)

// Client calls one or more JSON-RPC endpoints in order until one succeeds.
type Client struct {
	URLs       []string
	HTTPClient *http.Client
	Timeout    time.Duration
	nextID     atomic.Uint64
}

// Error is a JSON-RPC error object.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("jsonrpc: %d %s", e.Code, e.Message)
}

type request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      uint64 `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *Error          `json:"error"`
}

// Call invokes method and unmarshals the result into out. A JSON-RPC error is
// returned as *Error. Transport failures, non-200 statuses, and oversized
// responses fail over to the next endpoint; protocol errors do not.
func (c *Client) Call(ctx context.Context, method string, params any, out any) error {
	if len(c.URLs) == 0 {
		return ErrNoEndpoints
	}
	body, err := json.Marshal(request{
		JSONRPC: "2.0",
		ID:      c.nextID.Add(1),
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return fmt.Errorf("jsonrpc: encode request: %w", err)
	}

	var lastErr error
	for _, endpoint := range c.URLs {
		if err := ctx.Err(); err != nil {
			return err
		}
		result, err := c.callOne(ctx, endpoint, body)
		if err == nil {
			if out == nil || len(result) == 0 || string(result) == "null" {
				return nil
			}
			if err := json.Unmarshal(result, out); err != nil {
				return fmt.Errorf("jsonrpc: decode result: %w", err)
			}
			return nil
		}
		var rpcErr *Error
		if errors.As(err, &rpcErr) {
			return err
		}
		lastErr = err
	}
	return fmt.Errorf("jsonrpc: all endpoints failed: %w", lastErr)
}

func (c *Client) callOne(ctx context.Context, endpoint string, body []byte) (json.RawMessage, error) {
	attemptCtx := ctx
	var cancel context.CancelFunc
	if c.Timeout > 0 {
		attemptCtx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: c.Timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, MaxResponseBytes+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(payload) > MaxResponseBytes {
		return nil, ErrResponseTooLarge
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jsonrpc: HTTP %d: %s", resp.StatusCode, truncate(payload, 256))
	}
	var decoded response
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, fmt.Errorf("jsonrpc: decode response: %w", err)
	}
	if decoded.Error != nil {
		return nil, decoded.Error
	}
	return decoded.Result, nil
}

func truncate(data []byte, max int) string {
	if len(data) <= max {
		return string(data)
	}
	return string(data[:max]) + "..."
}
