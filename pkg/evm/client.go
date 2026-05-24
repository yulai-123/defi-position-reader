package evm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/yulai-123/defi-position-reader/pkg/core"
)

const defaultHTTPTimeout = 20 * time.Second

type ContractCaller interface {
	CallContract(ctx context.Context, to common.Address, data []byte) ([]byte, error)
}

type Client struct {
	rpcURLs []string
	http    *http.Client
}

func NewClient(rpcURLs ...string) (*Client, error) {
	rpcURLs = normalizeRPCURLs(rpcURLs)
	if len(rpcURLs) == 0 {
		return nil, fmt.Errorf("evm rpc url is empty")
	}
	return &Client{
		rpcURLs: rpcURLs,
		http: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
	}, nil
}

func NewClientFromEnv(chain core.Chain) (*Client, error) {
	rpcURLs := make([]string, 0, 1+len(chain.PublicRPCURLs))
	envName := strings.TrimSpace(chain.RPCUrlEnv)
	if envName != "" {
		rpcURLs = append(rpcURLs, os.Getenv(envName))
	}
	rpcURLs = append(rpcURLs, chain.PublicRPCURLs...)
	if len(normalizeRPCURLs(rpcURLs)) == 0 {
		return nil, fmt.Errorf("chain %q has no RPC URL configured; set %s or provide public RPC fallbacks", chain.Name, envName)
	}
	return NewClient(rpcURLs...)
}

func (c *Client) CallContract(ctx context.Context, to common.Address, data []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c == nil || c.http == nil {
		return nil, fmt.Errorf("evm client is nil")
	}
	if len(c.rpcURLs) == 0 {
		return nil, fmt.Errorf("evm client has no rpc urls")
	}

	var errs []error
	for _, rpcURL := range c.rpcURLs {
		result, err := c.callContractOnce(ctx, rpcURL, to, data)
		if err == nil {
			return result, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", sanitizeRPCURL(rpcURL), err))
	}
	return nil, fmt.Errorf("eth_call failed on all configured RPC URLs: %w", errors.Join(errs...))
}

func (c *Client) callContractOnce(ctx context.Context, rpcURL string, to common.Address, data []byte) ([]byte, error) {
	reqBody := rpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "eth_call",
		Params: []any{
			map[string]string{
				"to":   to.Hex(),
				"data": hexutil.Encode(data),
			},
			"latest",
		},
	}
	encoded, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal eth_call request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, rpcURL, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("create eth_call request: %w", err)
	}
	httpReq.Header.Set("content-type", "application/json")

	httpResp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send eth_call request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return nil, fmt.Errorf("eth_call http status %d", httpResp.StatusCode)
	}

	var resp rpcResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode eth_call response: %w", err)
	}
	if resp.Error != nil {
		return nil, resp.Error
	}
	if len(resp.Result) == 0 {
		return nil, fmt.Errorf("eth_call response missing result")
	}

	var result string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("decode eth_call result: %w", err)
	}
	return common.FromHex(result), nil
}

func normalizeRPCURLs(urls []string) []string {
	out := make([]string, 0, len(urls))
	seen := make(map[string]struct{}, len(urls))
	for _, url := range urls {
		url = strings.TrimSpace(url)
		if url == "" {
			continue
		}
		if _, ok := seen[url]; ok {
			continue
		}
		seen[url] = struct{}{}
		out = append(out, url)
	}
	return out
}

func sanitizeRPCURL(url string) string {
	url = strings.TrimSpace(url)
	if url == "" {
		return "<empty rpc url>"
	}
	if i := strings.IndexByte(url, '?'); i >= 0 {
		url = url[:i] + "?..."
	}
	parts := strings.Split(url, "/")
	if len(parts) > 4 && parts[len(parts)-2] == "v2" {
		parts[len(parts)-1] = "..."
	}
	return strings.Join(parts, "/")
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	if e == nil {
		return "evm rpc error"
	}
	if e.Code == 0 {
		return e.Message
	}
	return fmt.Sprintf("evm rpc error %d: %s", e.Code, e.Message)
}
