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

type LogQuery struct {
	FromBlock uint64
	ToBlock   uint64
	Topics    []any
}

type Log struct {
	Address common.Address
	Topics  []common.Hash
}

type AssetTransferQuery struct {
	FromAddress *common.Address
	ToAddress   *common.Address
	PageKey     string
	MaxCount    uint64
}

type AssetTransferResult struct {
	Transfers []AssetTransfer
	PageKey   string
}

type AssetTransfer struct {
	From                common.Address
	To                  common.Address
	RawContractAddress  common.Address
	RawContractDecimals string
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

func (c *Client) BlockNumber(ctx context.Context) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if c == nil || c.http == nil {
		return 0, fmt.Errorf("evm client is nil")
	}
	if len(c.rpcURLs) == 0 {
		return 0, fmt.Errorf("evm client has no rpc urls")
	}

	var errs []error
	for _, rpcURL := range c.rpcURLs {
		result, err := c.blockNumberOnce(ctx, rpcURL)
		if err == nil {
			return result, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", sanitizeRPCURL(rpcURL), err))
	}
	return 0, fmt.Errorf("eth_blockNumber failed on all configured RPC URLs: %w", errors.Join(errs...))
}

func (c *Client) FilterLogs(ctx context.Context, query LogQuery) ([]Log, error) {
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
		result, err := c.filterLogsOnce(ctx, rpcURL, query)
		if err == nil {
			return result, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", sanitizeRPCURL(rpcURL), err))
	}
	return nil, fmt.Errorf("eth_getLogs failed on all configured RPC URLs: %w", errors.Join(errs...))
}

func (c *Client) AssetTransfers(ctx context.Context, query AssetTransferQuery) (AssetTransferResult, error) {
	if err := ctx.Err(); err != nil {
		return AssetTransferResult{}, err
	}
	if c == nil || c.http == nil {
		return AssetTransferResult{}, fmt.Errorf("evm client is nil")
	}
	if len(c.rpcURLs) == 0 {
		return AssetTransferResult{}, fmt.Errorf("evm client has no rpc urls")
	}

	var errs []error
	for _, rpcURL := range c.rpcURLs {
		result, err := c.assetTransfersOnce(ctx, rpcURL, query)
		if err == nil {
			return result, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", sanitizeRPCURL(rpcURL), err))
	}
	return AssetTransferResult{}, fmt.Errorf("alchemy_getAssetTransfers failed on all configured RPC URLs: %w", errors.Join(errs...))
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

func (c *Client) blockNumberOnce(ctx context.Context, rpcURL string) (uint64, error) {
	encoded, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "eth_blockNumber",
		Params:  []any{},
	})
	if err != nil {
		return 0, fmt.Errorf("marshal eth_blockNumber request: %w", err)
	}

	var resp rpcResponse
	if err := c.doRPC(ctx, rpcURL, encoded, &resp); err != nil {
		return 0, err
	}
	if resp.Error != nil {
		return 0, resp.Error
	}
	if len(resp.Result) == 0 {
		return 0, fmt.Errorf("eth_blockNumber response missing result")
	}

	var result string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return 0, fmt.Errorf("decode eth_blockNumber result: %w", err)
	}
	value, err := hexutil.DecodeUint64(result)
	if err != nil {
		return 0, fmt.Errorf("decode eth_blockNumber hex: %w", err)
	}
	return value, nil
}

func (c *Client) filterLogsOnce(ctx context.Context, rpcURL string, query LogQuery) ([]Log, error) {
	filter := map[string]any{
		"fromBlock": hexutil.EncodeUint64(query.FromBlock),
		"toBlock":   hexutil.EncodeUint64(query.ToBlock),
	}
	if len(query.Topics) > 0 {
		filter["topics"] = query.Topics
	}
	encoded, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "eth_getLogs",
		Params:  []any{filter},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal eth_getLogs request: %w", err)
	}

	var resp rpcResponse
	if err := c.doRPC(ctx, rpcURL, encoded, &resp); err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, resp.Error
	}
	if len(resp.Result) == 0 {
		return nil, fmt.Errorf("eth_getLogs response missing result")
	}

	var raw []rpcLog
	if err := json.Unmarshal(resp.Result, &raw); err != nil {
		return nil, fmt.Errorf("decode eth_getLogs result: %w", err)
	}
	logs := make([]Log, 0, len(raw))
	for _, item := range raw {
		topics := make([]common.Hash, 0, len(item.Topics))
		for _, topic := range item.Topics {
			topics = append(topics, common.HexToHash(topic))
		}
		logs = append(logs, Log{
			Address: common.HexToAddress(item.Address),
			Topics:  topics,
		})
	}
	return logs, nil
}

func (c *Client) assetTransfersOnce(ctx context.Context, rpcURL string, query AssetTransferQuery) (AssetTransferResult, error) {
	maxCount := query.MaxCount
	if maxCount == 0 {
		maxCount = 1000
	}
	params := map[string]any{
		"fromBlock":        "0x0",
		"toBlock":          "latest",
		"category":         []string{"erc20"},
		"withMetadata":     false,
		"excludeZeroValue": true,
		"maxCount":         hexutil.EncodeUint64(maxCount),
		"order":            "desc",
	}
	if query.FromAddress != nil {
		params["fromAddress"] = query.FromAddress.Hex()
	}
	if query.ToAddress != nil {
		params["toAddress"] = query.ToAddress.Hex()
	}
	if strings.TrimSpace(query.PageKey) != "" {
		params["pageKey"] = strings.TrimSpace(query.PageKey)
	}
	encoded, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "alchemy_getAssetTransfers",
		Params:  []any{params},
	})
	if err != nil {
		return AssetTransferResult{}, fmt.Errorf("marshal alchemy_getAssetTransfers request: %w", err)
	}

	var resp rpcResponse
	if err := c.doRPC(ctx, rpcURL, encoded, &resp); err != nil {
		return AssetTransferResult{}, err
	}
	if resp.Error != nil {
		return AssetTransferResult{}, resp.Error
	}
	if len(resp.Result) == 0 {
		return AssetTransferResult{}, fmt.Errorf("alchemy_getAssetTransfers response missing result")
	}

	var raw rpcAssetTransfersResult
	if err := json.Unmarshal(resp.Result, &raw); err != nil {
		return AssetTransferResult{}, fmt.Errorf("decode alchemy_getAssetTransfers result: %w", err)
	}
	out := AssetTransferResult{
		Transfers: make([]AssetTransfer, 0, len(raw.Transfers)),
		PageKey:   raw.PageKey,
	}
	for _, item := range raw.Transfers {
		out.Transfers = append(out.Transfers, AssetTransfer{
			From:                common.HexToAddress(item.From),
			To:                  common.HexToAddress(item.To),
			RawContractAddress:  common.HexToAddress(item.RawContract.Address),
			RawContractDecimals: item.RawContract.Decimal,
		})
	}
	return out, nil
}

func (c *Client) doRPC(ctx context.Context, rpcURL string, encoded []byte, out any) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, rpcURL, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("create rpc request: %w", err)
	}
	httpReq.Header.Set("content-type", "application/json")

	httpResp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("send rpc request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return fmt.Errorf("rpc http status %d", httpResp.StatusCode)
	}
	if err := json.NewDecoder(httpResp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode rpc response: %w", err)
	}
	return nil
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

type rpcLog struct {
	Address string   `json:"address"`
	Topics  []string `json:"topics"`
}

type rpcAssetTransfersResult struct {
	Transfers []rpcAssetTransfer `json:"transfers"`
	PageKey   string             `json:"pageKey,omitempty"`
}

type rpcAssetTransfer struct {
	From        string `json:"from"`
	To          string `json:"to"`
	RawContract struct {
		Address string `json:"address"`
		Decimal string `json:"decimal,omitempty"`
	} `json:"rawContract"`
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
