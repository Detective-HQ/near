package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"universal-defi-intelligence/internal/chains/registry"
	"universal-defi-intelligence/internal/chains/rpc/jsonrpc"
	"universal-defi-intelligence/internal/encoding/base58"
)

func rpcServer(t *testing.T, respond func(method string, params json.RawMessage) (any, *jsonrpc.Error)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		result, rpcErr := respond(req.Method, req.Params)
		response := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		if rpcErr != nil {
			response["error"] = rpcErr
		} else {
			response["result"] = result
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
}

func TestEVMVerifier(t *testing.T) {
	t.Parallel()
	txHash := "0x" + strings.Repeat("ab", 32)
	to := "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed"
	data := "0xa9059cbb" + strings.Repeat("0", 24) + strings.Repeat("11", 32) + strings.Repeat("0", 63) + "1"
	server := rpcServer(t, func(method string, _ json.RawMessage) (any, *jsonrpc.Error) {
		switch method {
		case "eth_getTransactionReceipt":
			return map[string]any{
				"transactionHash": txHash,
				"blockNumber":     "0x64",
				"status":          "0x1",
				"to":              to,
				"input":           data,
			}, nil
		case "eth_blockNumber":
			return "0x6e", nil
		default:
			return nil, &jsonrpc.Error{Code: -32601, Message: "method not found"}
		}
	})
	defer server.Close()
	verifier := &EVMVerifier{RPC: &jsonrpc.Client{URLs: []string{server.URL}}}

	result, err := verifier.Verify(context.Background(), Request{
		TxHash:       "0x" + strings.ToUpper(strings.TrimPrefix(txHash, "0x")),
		ExpectedTo:   to,
		ExpectedData: data,
	})
	if err != nil {
		t.Fatalf("Verify(): %v", err)
	}
	if !result.Found || !result.Success || result.Confirmations != 11 || result.Block != 100 || !strings.EqualFold(result.To, to) {
		t.Fatalf("result = %+v", result)
	}

	result, err = verifier.Verify(context.Background(), Request{TxHash: txHash, ExpectedTo: to, ExpectedData: data, MinConfirmations: 12})
	if !errors.Is(err, ErrInsufficientConfirmations) || result.Confirmations != 11 {
		t.Fatalf("Verify(insufficient) = %+v, %v", result, err)
	}
	if _, err := verifier.Verify(context.Background(), Request{TxHash: txHash, ExpectedTo: "0x0000000000000000000000000000000000000000"}); !errors.Is(err, ErrVerificationMismatch) {
		t.Fatalf("Verify(to mismatch) error = %v", err)
	}
	if _, err := verifier.Verify(context.Background(), Request{TxHash: txHash, ExpectedData: "0xdeadbeef"}); !errors.Is(err, ErrVerificationMismatch) {
		t.Fatalf("Verify(data mismatch) error = %v", err)
	}
	if _, err := verifier.Verify(context.Background(), Request{TxHash: "bad"}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("Verify(bad hash) error = %v", err)
	}
}

func TestEVMVerifierStatuses(t *testing.T) {
	t.Parallel()
	txHash := "0x" + strings.Repeat("ab", 32)
	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		server := rpcServer(t, func(string, json.RawMessage) (any, *jsonrpc.Error) { return nil, nil })
		defer server.Close()
		verifier := &EVMVerifier{RPC: &jsonrpc.Client{URLs: []string{server.URL}}}
		if _, err := verifier.Verify(context.Background(), Request{TxHash: txHash}); !errors.Is(err, ErrTxNotFound) {
			t.Fatalf("Verify() error = %v", err)
		}
	})
	t.Run("failed", func(t *testing.T) {
		t.Parallel()
		server := rpcServer(t, func(string, json.RawMessage) (any, *jsonrpc.Error) {
			return map[string]any{"transactionHash": txHash, "blockNumber": "0x1", "status": "0x0", "input": "0x"}, nil
		})
		defer server.Close()
		verifier := &EVMVerifier{RPC: &jsonrpc.Client{URLs: []string{server.URL}}}
		result, err := verifier.Verify(context.Background(), Request{TxHash: txHash})
		if !errors.Is(err, ErrTransactionFailed) || result.Success {
			t.Fatalf("Verify() = %+v, %v", result, err)
		}
	})
}

func TestSolanaVerifier(t *testing.T) {
	t.Parallel()
	signature := base58.Encode(make([]byte, 64))
	var slotCalls int
	server := rpcServer(t, func(method string, _ json.RawMessage) (any, *jsonrpc.Error) {
		switch method {
		case "getSignatureStatuses":
			return map[string]any{"value": []any{map[string]any{
				"slot":               100,
				"confirmations":      nil,
				"err":                nil,
				"confirmationStatus": "finalized",
			}}}, nil
		case "getSlot":
			slotCalls++
			return 120, nil
		default:
			return nil, &jsonrpc.Error{Code: -32601, Message: "method not found"}
		}
	})
	defer server.Close()
	verifier := &SolanaVerifier{RPC: &jsonrpc.Client{URLs: []string{server.URL}}}
	result, err := verifier.Verify(context.Background(), Request{TxHash: signature, MinConfirmations: 10})
	if err != nil {
		t.Fatalf("Verify(): %v", err)
	}
	if !result.Found || !result.Success || !result.Finalized || result.Block != 100 {
		t.Fatalf("result = %+v", result)
	}
	if slotCalls != 0 {
		t.Fatalf("getSlot called %d times for finalized status", slotCalls)
	}
	if _, err := verifier.Verify(context.Background(), Request{TxHash: "bad"}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("Verify(bad signature) error = %v", err)
	}
}

func TestSolanaVerifierConfirmationsAndFailure(t *testing.T) {
	t.Parallel()
	signature := base58.Encode(make([]byte, 64))
	t.Run("confirmed computes confirmations", func(t *testing.T) {
		t.Parallel()
		confirmations := uint64(3)
		server := rpcServer(t, func(method string, _ json.RawMessage) (any, *jsonrpc.Error) {
			switch method {
			case "getSignatureStatuses":
				return map[string]any{"value": []any{map[string]any{
					"slot":               100,
					"confirmations":      confirmations,
					"err":                nil,
					"confirmationStatus": "confirmed",
				}}}, nil
			case "getSlot":
				return 120, nil
			}
			return nil, nil
		})
		defer server.Close()
		verifier := &SolanaVerifier{RPC: &jsonrpc.Client{URLs: []string{server.URL}}}
		result, err := verifier.Verify(context.Background(), Request{TxHash: signature})
		if err != nil || result.Confirmations != 3 || result.Finalized {
			t.Fatalf("Verify() = %+v, %v", result, err)
		}
		result, err = verifier.Verify(context.Background(), Request{TxHash: signature, MinConfirmations: 4})
		if !errors.Is(err, ErrInsufficientConfirmations) || result.Confirmations != 3 {
			t.Fatalf("Verify(min) = %+v, %v", result, err)
		}
	})
	t.Run("transaction error", func(t *testing.T) {
		t.Parallel()
		server := rpcServer(t, func(string, json.RawMessage) (any, *jsonrpc.Error) {
			return map[string]any{"value": []any{map[string]any{
				"slot":               100,
				"confirmations":      1,
				"err":                map[string]any{"InstructionError": []any{0, "Custom"}},
				"confirmationStatus": "confirmed",
			}}}, nil
		})
		defer server.Close()
		verifier := &SolanaVerifier{RPC: &jsonrpc.Client{URLs: []string{server.URL}}}
		result, err := verifier.Verify(context.Background(), Request{TxHash: signature})
		if !errors.Is(err, ErrTransactionFailed) || result.Success {
			t.Fatalf("Verify() = %+v, %v", result, err)
		}
	})
	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		server := rpcServer(t, func(string, json.RawMessage) (any, *jsonrpc.Error) {
			return map[string]any{"value": []any{nil}}, nil
		})
		defer server.Close()
		verifier := &SolanaVerifier{RPC: &jsonrpc.Client{URLs: []string{server.URL}}}
		if _, err := verifier.Verify(context.Background(), Request{TxHash: signature}); !errors.Is(err, ErrTxNotFound) {
			t.Fatalf("Verify() error = %v", err)
		}
	})
}

func TestStellarVerifier(t *testing.T) {
	t.Parallel()
	hash := strings.Repeat("ab", 32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/transactions/"+hash:
			_, _ = w.Write([]byte(`{"hash":"` + hash + `","successful":true,"ledger":100,"source_account":"GBGLLK7WVV47X5NLXTFPZQTJ3BONEZI62S4ILNMGT4SBV3PQUW5CTECA"}`))
		case r.URL.Path == "/ledgers":
			_, _ = w.Write([]byte(`{"_embedded":{"records":[{"sequence":110}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	verifier := &StellarVerifier{HTTP: &HTTPFetcher{URLs: []string{server.URL}, Timeout: time.Second}}
	result, err := verifier.Verify(context.Background(), Request{TxHash: hash, MinConfirmations: 5})
	if err != nil {
		t.Fatalf("Verify(): %v", err)
	}
	if !result.Found || !result.Success || !result.Finalized || result.Confirmations != 11 || result.Block != 100 {
		t.Fatalf("result = %+v", result)
	}
	if _, err := verifier.Verify(context.Background(), Request{TxHash: "bad"}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("Verify(bad hash) error = %v", err)
	}
}

func TestStellarVerifierStatuses(t *testing.T) {
	t.Parallel()
	hash := strings.Repeat("ab", 32)
	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
		defer server.Close()
		verifier := &StellarVerifier{HTTP: &HTTPFetcher{URLs: []string{server.URL}}}
		if _, err := verifier.Verify(context.Background(), Request{TxHash: hash}); !errors.Is(err, ErrTxNotFound) {
			t.Fatalf("Verify() error = %v", err)
		}
	})
	t.Run("failed", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"hash":"` + hash + `","successful":false,"ledger":100}`))
		}))
		defer server.Close()
		verifier := &StellarVerifier{HTTP: &HTTPFetcher{URLs: []string{server.URL}}}
		result, err := verifier.Verify(context.Background(), Request{TxHash: hash})
		if !errors.Is(err, ErrTransactionFailed) || result.Success {
			t.Fatalf("Verify() = %+v, %v", result, err)
		}
	})
}

func TestAptosVerifier(t *testing.T) {
	t.Parallel()
	hash := "0x" + strings.Repeat("ab", 32)
	recipient := "0x1"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/transactions/by_hash/"+hash:
			_, _ = w.Write([]byte(`{"hash":"` + hash + `","success":true,"version":"100","type":"user_transaction","payload":{"type":"entry_function_payload","function":"0x1::aptos_account::transfer","arguments":["` + recipient + `","1000"]}}`))
		case r.URL.Path == "/":
			_, _ = w.Write([]byte(`{"ledger_version":"110"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	verifier := &AptosVerifier{HTTP: &HTTPFetcher{URLs: []string{server.URL}, Timeout: time.Second}}
	result, err := verifier.Verify(context.Background(), Request{TxHash: hash, ExpectedTo: recipient, MinConfirmations: 5})
	if err != nil {
		t.Fatalf("Verify(): %v", err)
	}
	if !result.Found || !result.Success || result.Confirmations != 11 || result.Block != 100 || result.To == "" {
		t.Fatalf("result = %+v", result)
	}
	if _, err := verifier.Verify(context.Background(), Request{TxHash: hash, ExpectedTo: "0x2"}); !errors.Is(err, ErrVerificationMismatch) {
		t.Fatalf("Verify(mismatch) error = %v", err)
	}
	if _, err := verifier.Verify(context.Background(), Request{TxHash: "bad"}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("Verify(bad hash) error = %v", err)
	}
}

func TestAptosVerifierStatuses(t *testing.T) {
	t.Parallel()
	hash := "0x" + strings.Repeat("ab", 32)
	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
		defer server.Close()
		verifier := &AptosVerifier{HTTP: &HTTPFetcher{URLs: []string{server.URL}}}
		if _, err := verifier.Verify(context.Background(), Request{TxHash: hash}); !errors.Is(err, ErrTxNotFound) {
			t.Fatalf("Verify() error = %v", err)
		}
	})
	t.Run("failed", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"hash":"` + hash + `","success":false,"version":"100"}`))
		}))
		defer server.Close()
		verifier := &AptosVerifier{HTTP: &HTTPFetcher{URLs: []string{server.URL}}}
		result, err := verifier.Verify(context.Background(), Request{TxHash: hash})
		if !errors.Is(err, ErrTransactionFailed) || result.Success {
			t.Fatalf("Verify() = %+v, %v", result, err)
		}
	})
}

func TestNEARVerifier(t *testing.T) {
	t.Parallel()
	hash := base58.Encode([]byte(strings.Repeat("a", 32)))
	sender := "alice.near"
	server := rpcServer(t, func(method string, params json.RawMessage) (any, *jsonrpc.Error) {
		if method != "tx" {
			return nil, &jsonrpc.Error{Code: -32601, Message: "method not found"}
		}
		var decoded map[string]string
		_ = json.Unmarshal(params, &decoded)
		if decoded["sender_account_id"] != sender {
			t.Errorf("sender param = %q", decoded["sender_account_id"])
		}
		return map[string]any{
			"final_execution_status": "FINAL",
			"status":                 map[string]any{"SuccessValue": ""},
			"transaction":            map[string]any{"signer_id": sender, "receiver_id": "bob.near"},
		}, nil
	})
	defer server.Close()
	verifier := &NEARVerifier{RPC: &jsonrpc.Client{URLs: []string{server.URL}}}
	result, err := verifier.Verify(context.Background(), Request{TxHash: hash, Sender: sender, ExpectedTo: "bob.near", MinConfirmations: 12})
	if err != nil {
		t.Fatalf("Verify(): %v", err)
	}
	if !result.Found || !result.Success || !result.Finalized || result.To != "bob.near" {
		t.Fatalf("result = %+v", result)
	}
	if _, err := verifier.Verify(context.Background(), Request{TxHash: hash, Sender: sender, ExpectedTo: "carol.near"}); !errors.Is(err, ErrVerificationMismatch) {
		t.Fatalf("Verify(mismatch) error = %v", err)
	}
	if _, err := verifier.Verify(context.Background(), Request{TxHash: hash}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("Verify(no sender) error = %v", err)
	}
}

func TestNEARVerifierStatuses(t *testing.T) {
	t.Parallel()
	hash := base58.Encode([]byte(strings.Repeat("a", 32)))
	t.Run("failure", func(t *testing.T) {
		t.Parallel()
		server := rpcServer(t, func(string, json.RawMessage) (any, *jsonrpc.Error) {
			return map[string]any{"final_execution_status": "FINAL", "status": map[string]any{"Failure": map[string]any{}}}, nil
		})
		defer server.Close()
		verifier := &NEARVerifier{RPC: &jsonrpc.Client{URLs: []string{server.URL}}}
		result, err := verifier.Verify(context.Background(), Request{TxHash: hash, Sender: "alice.near"})
		if !errors.Is(err, ErrTransactionFailed) || result.Success {
			t.Fatalf("Verify() = %+v, %v", result, err)
		}
	})
	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		server := rpcServer(t, func(string, json.RawMessage) (any, *jsonrpc.Error) {
			return nil, &jsonrpc.Error{Code: -32000, Message: "Transaction does not exist"}
		})
		defer server.Close()
		verifier := &NEARVerifier{RPC: &jsonrpc.Client{URLs: []string{server.URL}}}
		if _, err := verifier.Verify(context.Background(), Request{TxHash: hash, Sender: "alice.near"}); !errors.Is(err, ErrTxNotFound) {
			t.Fatalf("Verify() error = %v", err)
		}
	})
	t.Run("not final", func(t *testing.T) {
		t.Parallel()
		server := rpcServer(t, func(string, json.RawMessage) (any, *jsonrpc.Error) {
			return map[string]any{"final_execution_status": "EXECUTED", "status": map[string]any{"SuccessValue": ""}}, nil
		})
		defer server.Close()
		verifier := &NEARVerifier{RPC: &jsonrpc.Client{URLs: []string{server.URL}}}
		if _, err := verifier.Verify(context.Background(), Request{TxHash: hash, Sender: "alice.near", MinConfirmations: 1}); !errors.Is(err, ErrInsufficientConfirmations) {
			t.Fatalf("Verify() error = %v", err)
		}
	})
}

func TestMultiChainVerifierDispatch(t *testing.T) {
	t.Parallel()
	reg, err := registry.Load()
	if err != nil {
		t.Fatalf("registry.Load(): %v", err)
	}
	txHash := "0x" + strings.Repeat("ab", 32)
	server := rpcServer(t, func(method string, _ json.RawMessage) (any, *jsonrpc.Error) {
		switch method {
		case "eth_getTransactionReceipt":
			return map[string]any{"transactionHash": txHash, "blockNumber": "0x1", "status": "0x1", "input": "0x"}, nil
		case "eth_blockNumber":
			return "0x1", nil
		}
		return nil, &jsonrpc.Error{Code: -32601, Message: "method not found"}
	})
	defer server.Close()
	verifier := &MultiChainVerifier{
		Registry:  reg,
		Endpoints: map[string][]string{"ethereum": {server.URL}},
	}
	if _, err := verifier.Verify(context.Background(), Request{Chain: "ethereum", TxHash: txHash}); err != nil {
		t.Fatalf("Verify(): %v", err)
	}
	if _, err := verifier.Verify(context.Background(), Request{Chain: "unknown", TxHash: txHash}); !errors.Is(err, ErrUnsupportedChain) {
		t.Fatalf("Verify(unknown) error = %v", err)
	}
}
