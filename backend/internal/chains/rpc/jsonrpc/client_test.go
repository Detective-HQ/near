package jsonrpc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCallSuccess(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request: %s %s", r.Method, r.Header.Get("Content-Type"))
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`))
	}))
	defer server.Close()
	client := &Client{URLs: []string{server.URL}, Timeout: time.Second}
	var out struct {
		OK bool `json:"ok"`
	}
	if err := client.Call(context.Background(), "test", nil, &out); err != nil {
		t.Fatalf("Call(): %v", err)
	}
	if !out.OK {
		t.Fatal("result not decoded")
	}
}

func TestCallFailover(t *testing.T) {
	t.Parallel()
	var calls int
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		http.Error(w, "unavailable", http.StatusBadGateway)
	}))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"ok"}`))
	}))
	defer good.Close()
	client := &Client{URLs: []string{bad.URL, good.URL}}
	var out string
	if err := client.Call(context.Background(), "test", nil, &out); err != nil {
		t.Fatalf("Call(): %v", err)
	}
	if out != "ok" || calls != 2 {
		t.Fatalf("out = %q, calls = %d", out, calls)
	}
}

func TestCallDoesNotFailoverProtocolError(t *testing.T) {
	t.Parallel()
	var secondCalled bool
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"bad params"}}`))
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		secondCalled = true
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":null}`))
	}))
	defer second.Close()
	client := &Client{URLs: []string{first.URL, second.URL}}
	var rpcErr *Error
	err := client.Call(context.Background(), "test", nil, nil)
	if !errors.As(err, &rpcErr) || rpcErr.Code != -32602 {
		t.Fatalf("Call() error = %v", err)
	}
	if secondCalled {
		t.Fatal("protocol error should not fail over")
	}
}

func TestCallErrors(t *testing.T) {
	t.Parallel()
	if err := (&Client{}).Call(context.Background(), "test", nil, nil); !errors.Is(err, ErrNoEndpoints) {
		t.Fatalf("Call(no endpoints) error = %v", err)
	}

	t.Run("response too large", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"`))
			_, _ = w.Write([]byte(strings.Repeat("a", MaxResponseBytes)))
			_, _ = w.Write([]byte(`"}`))
		}))
		defer server.Close()
		client := &Client{URLs: []string{server.URL}}
		if err := client.Call(context.Background(), "test", nil, nil); !errors.Is(err, ErrResponseTooLarge) {
			t.Fatalf("Call() error = %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(200 * time.Millisecond)
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":null}`))
		}))
		defer server.Close()
		client := &Client{URLs: []string{server.URL}, Timeout: 20 * time.Millisecond}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := client.Call(ctx, "test", nil, nil); err == nil {
			t.Fatal("Call() succeeded, want timeout")
		}
	})

	t.Run("bad json", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`not json`))
		}))
		defer server.Close()
		client := &Client{URLs: []string{server.URL}}
		if err := client.Call(context.Background(), "test", nil, nil); err == nil {
			t.Fatal("Call() succeeded, want decode error")
		}
	})
}
