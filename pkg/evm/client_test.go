package evm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/yulai-123/defi-position-reader/pkg/core"
)

func TestNewClientNormalizesRPCURLs(t *testing.T) {
	t.Parallel()

	client, err := NewClient(" ", "https://rpc.example", "https://rpc.example", "https://backup.example")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if len(client.rpcURLs) != 2 {
		t.Fatalf("expected 2 normalized RPC URLs, got %#v", client.rpcURLs)
	}
}

func TestNewClientFromEnvUsesPublicFallbacks(t *testing.T) {
	t.Parallel()

	client, err := NewClientFromEnv(core.Chain{
		ID:            1,
		Name:          "ethereum",
		RPCUrlEnv:     "DPR_TEST_EMPTY_RPC_URL",
		PublicRPCURLs: []string{"https://public.example"},
	})
	if err != nil {
		t.Fatalf("new client from env: %v", err)
	}
	if len(client.rpcURLs) != 1 || client.rpcURLs[0] != "https://public.example" {
		t.Fatalf("unexpected RPC URLs: %#v", client.rpcURLs)
	}
}

func TestNewClientFromEnvPrefersConfiguredRPC(t *testing.T) {
	t.Setenv("DPR_TEST_RPC_URL", "https://private.example")
	client, err := NewClientFromEnv(core.Chain{
		ID:            1,
		Name:          "ethereum",
		RPCUrlEnv:     "DPR_TEST_RPC_URL",
		PublicRPCURLs: []string{"https://public.example"},
	})
	if err != nil {
		t.Fatalf("new client from env: %v", err)
	}
	if len(client.rpcURLs) != 2 || client.rpcURLs[0] != "https://private.example" {
		t.Fatalf("unexpected RPC URL priority: %#v", client.rpcURLs)
	}
}

func TestClientCallContractFailsOver(t *testing.T) {
	t.Parallel()

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "busy", http.StatusTooManyRequests)
	}))
	t.Cleanup(failing.Close)

	working := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Method != "eth_call" || len(req.Params) != 2 {
			t.Fatalf("unexpected request: %#v", req)
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1234"}`))
	}))
	t.Cleanup(working.Close)

	client, err := NewClient(failing.URL, working.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.CallContract(context.Background(), common.HexToAddress("0x0000000000000000000000000000000000000001"), []byte{0xaa})
	if err != nil {
		t.Fatalf("call contract: %v", err)
	}
	if string(result) != "\x12\x34" {
		t.Fatalf("unexpected result: %x", result)
	}
}

func TestClientCallContractReturnsJoinedError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "busy", http.StatusTooManyRequests)
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(server.URL, "http://127.0.0.1:1")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = client.CallContract(context.Background(), common.HexToAddress("0x0000000000000000000000000000000000000001"), []byte{0xaa})
	if err == nil {
		t.Fatal("expected call error")
	}
	if !strings.Contains(err.Error(), "failed on all configured RPC URLs") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClientFilterLogsIncludesAddressAndData(t *testing.T) {
	t.Parallel()

	address := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Method != "eth_getLogs" || len(req.Params) != 1 {
			t.Fatalf("unexpected request: %#v", req)
		}
		params, ok := req.Params[0].(map[string]any)
		if !ok {
			t.Fatalf("unexpected params type: %#v", req.Params[0])
		}
		if params["address"] != address.Hex() {
			t.Fatalf("expected address filter %s, got %#v", address.Hex(), params["address"])
		}
		_, _ = w.Write([]byte(`{
			"jsonrpc":"2.0",
			"id":1,
			"result":[{
				"address":"0x00000000000000000000000000000000000000aa",
				"topics":["0x0000000000000000000000000000000000000000000000000000000000000001"],
				"data":"0x1234",
				"blockNumber":"0x10",
				"transactionHash":"0x000000000000000000000000000000000000000000000000000000000000abcd",
				"logIndex":"0x2"
			}]
		}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	logs, err := client.FilterLogs(context.Background(), LogQuery{
		FromBlock: 1,
		ToBlock:   20,
		Addresses: []common.Address{address},
		Topics:    []any{"0x1"},
	})
	if err != nil {
		t.Fatalf("filter logs: %v", err)
	}
	if len(logs) != 1 || logs[0].Address != address || logs[0].BlockNumber != 16 || logs[0].LogIndex != 2 || string(logs[0].Data) != "\x12\x34" {
		t.Fatalf("unexpected logs: %#v", logs)
	}
}
