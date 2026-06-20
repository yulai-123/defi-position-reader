package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/yulai-123/defi-position-reader/pkg/adapter"
	"github.com/yulai-123/defi-position-reader/pkg/cache"
	"github.com/yulai-123/defi-position-reader/pkg/core"
)

type outputFormat string

const (
	outputFormatJSON   outputFormat = "json"
	outputFormatTable  outputFormat = "table"
	outputFormatDetail outputFormat = "detail"
)

type traceLevel string

const (
	traceLevelSummary traceLevel = "summary"
	traceLevelCalls   traceLevel = "calls"
	traceLevelRaw     traceLevel = "raw"
)

type outputOptions struct {
	Format     outputFormat `json:"format"`
	Trace      bool         `json:"trace"`
	TraceLevel traceLevel   `json:"traceLevel"`
}

type traceEvent struct {
	Time       time.Time      `json:"time"`
	Layer      string         `json:"layer"`
	Operation  string         `json:"operation"`
	Message    string         `json:"message,omitempty"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

type traceRecorder struct {
	enabled bool
	level   traceLevel
	events  []traceEvent
}

type metadataSnapshot struct {
	Protocol  string             `json:"protocol"`
	Namespace string             `json:"namespace"`
	Status    string             `json:"status"`
	Items     int                `json:"items,omitempty"`
	Age       string             `json:"age,omitempty"`
	Stale     bool               `json:"stale,omitempty"`
	Error     string             `json:"error,omitempty"`
	Metadata  *core.MetadataInfo `json:"metadata,omitempty"`
}

type discoveryStepView struct {
	Step         string `json:"step"`
	Operation    string `json:"operation"`
	Markets      int    `json:"markets,omitempty"`
	Calls        int    `json:"calls,omitempty"`
	Items        int    `json:"items,omitempty"`
	Status       string `json:"status"`
	Notes        string `json:"notes,omitempty"`
	AllowFailure bool   `json:"allowFailure,omitempty"`
}

var knownMetadataNamespaces = map[string][]string{
	"demo":        {"markets"},
	"aave-v3":     {"markets", "lending-reserves", "yield-vaults"},
	"compound-v3": {"markets", "collateral-assets", "reward-configs"},
	"uniswap-v2":  {"markets", "pairs", "farming-pools"},
	"uniswap-v3":  {"markets", "pools"},
}

func parseOutputOptions(formatValue string, traceLevelValue string, trace bool) (outputOptions, error) {
	format := outputFormat(strings.ToLower(strings.TrimSpace(formatValue)))
	if format == "" {
		format = outputFormatJSON
	}
	switch format {
	case outputFormatJSON, outputFormatTable, outputFormatDetail:
	default:
		return outputOptions{}, fmt.Errorf("unknown output format %q", formatValue)
	}

	level := traceLevel(strings.ToLower(strings.TrimSpace(traceLevelValue)))
	if level == "" {
		level = traceLevelSummary
	}
	switch level {
	case traceLevelSummary, traceLevelCalls, traceLevelRaw:
	default:
		return outputOptions{}, fmt.Errorf("unknown trace level %q", traceLevelValue)
	}

	return outputOptions{
		Format:     format,
		Trace:      trace,
		TraceLevel: level,
	}, nil
}

func newTraceRecorder(opts outputOptions) *traceRecorder {
	return &traceRecorder{enabled: opts.Trace, level: opts.TraceLevel}
}

func (r *traceRecorder) Add(layer string, operation string, message string, attributes map[string]any) {
	if r == nil || !r.enabled {
		return
	}
	r.events = append(r.events, traceEvent{
		Time:       time.Now().UTC(),
		Layer:      layer,
		Operation:  operation,
		Message:    message,
		Attributes: attributes,
	})
}

func (r *traceRecorder) Events() []traceEvent {
	if r == nil || len(r.events) == 0 {
		return nil
	}
	events := make([]traceEvent, len(r.events))
	copy(events, r.events)
	return events
}

func writeSyncOutput(out io.Writer, target core.Chain, results []adapter.SyncResult, metadata []metadataSnapshot, opts outputOptions, events []traceEvent) error {
	if opts.Format == outputFormatJSON {
		if opts.Trace {
			return writeJSON(out, map[string]any{
				"chain":    target,
				"results":  results,
				"metadata": metadata,
				"trace":    events,
			})
		}
		return writeJSON(out, results)
	}

	if opts.Format == outputFormatDetail {
		if err := writeSyncDetail(out, target, results, metadata); err != nil {
			return err
		}
		return writeTraceText(out, events)
	}

	if err := writeSyncTable(out, target, results); err != nil {
		return err
	}
	return writeTraceText(out, events)
}

func writeSyncTable(out io.Writer, target core.Chain, results []adapter.SyncResult) error {
	fmt.Fprintf(out, "Sync Metadata: %s (%d)\n", target.DisplayName, target.ID)
	if len(results) == 0 {
		fmt.Fprintln(out, "No metadata synced.")
		return nil
	}

	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PROTOCOL\tNAMESPACE\tVERSION\tITEMS\tUPDATED\tSOURCE")
	for _, result := range results {
		info := result.Metadata
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\n",
			info.Protocol,
			info.Namespace,
			info.Version,
			result.Items,
			formatTime(info.UpdatedAt),
			emptyDash(info.Source),
		)
	}
	return tw.Flush()
}

func writeSyncDetail(out io.Writer, target core.Chain, results []adapter.SyncResult, metadata []metadataSnapshot) error {
	fmt.Fprintf(out, "Sync Metadata\n")
	fmt.Fprintf(out, "Chain: %s (%d)\n", target.DisplayName, target.ID)
	fmt.Fprintf(out, "Results: %d\n\n", len(results))

	for _, result := range results {
		info := result.Metadata
		fmt.Fprintf(out, "- %s / %s\n", info.Protocol, info.Namespace)
		fmt.Fprintf(out, "  Version: %s\n", emptyDash(info.Version))
		fmt.Fprintf(out, "  Items: %d\n", result.Items)
		fmt.Fprintf(out, "  Updated: %s\n", formatTime(info.UpdatedAt))
		fmt.Fprintf(out, "  Source: %s\n", emptyDash(info.Source))
		if steps := discoveryStepsFromResult(result); len(steps) > 0 {
			fmt.Fprintln(out, "  Discovery:")
			writeDiscoverySteps(out, "    ", steps)
		}
	}

	if len(metadata) > 0 {
		fmt.Fprintln(out)
		writeMetadataDetail(out, metadata)
	}
	return nil
}

func writePositionsOutput(out io.Writer, target core.Chain, owner string, positions []core.Position, metadata []metadataSnapshot, opts outputOptions, events []traceEvent) error {
	if opts.Format == outputFormatJSON {
		if opts.Trace {
			return writeJSON(out, map[string]any{
				"chain":     target,
				"owner":     owner,
				"positions": positions,
				"metadata":  metadata,
				"trace":     events,
			})
		}
		return writeJSON(out, positions)
	}

	if opts.Format == outputFormatDetail {
		if err := writePositionsDetail(out, target, owner, positions, metadata); err != nil {
			return err
		}
		return writeTraceText(out, events)
	}

	if err := writePositionsTable(out, target, owner, positions); err != nil {
		return err
	}
	return writeTraceText(out, events)
}

func writePositionsTable(out io.Writer, target core.Chain, owner string, positions []core.Position) error {
	fmt.Fprintf(out, "Positions: %s (%d) / %s\n", target.DisplayName, target.ID, emptyDash(owner))
	if len(positions) == 0 {
		fmt.Fprintln(out, "No positions found.")
		return nil
	}

	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TYPE\tPROTOCOL\tNAME\tSUPPLY/SHARES\tUNDERLYING\tREWARDS\tDEBT\tHEALTH")
	for _, position := range positions {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			position.Type,
			position.Protocol,
			position.DisplayName,
			summaryPrimaryAmounts(position),
			summaryUnderlyingAmounts(position),
			formatAmounts(position.Rewards),
			formatAmounts(position.Debt),
			positionHealth(position),
		)
	}
	return tw.Flush()
}

func writePositionsDetail(out io.Writer, target core.Chain, owner string, positions []core.Position, metadata []metadataSnapshot) error {
	fmt.Fprintf(out, "Positions\n")
	fmt.Fprintf(out, "Chain: %s (%d)\n", target.DisplayName, target.ID)
	fmt.Fprintf(out, "Owner: %s\n", emptyDash(owner))
	fmt.Fprintf(out, "Count: %d\n", len(positions))

	for _, position := range positions {
		fmt.Fprintln(out)
		fmt.Fprintf(out, "Position: %s\n", emptyDash(position.DisplayName))
		fmt.Fprintf(out, "  ID: %s\n", emptyDash(position.ID))
		fmt.Fprintf(out, "  Protocol: %s\n", emptyDash(position.Protocol))
		fmt.Fprintf(out, "  Type: %s\n", emptyDash(string(position.Type)))
		fmt.Fprintf(out, "  Shares:\n")
		writeAmountLines(out, "    ", position.Shares)
		fmt.Fprintf(out, "  Underlying:\n")
		writeAmountLines(out, "    ", position.Underlying)
		fmt.Fprintf(out, "  Rewards:\n")
		writeAmountLines(out, "    ", position.Rewards)
		fmt.Fprintf(out, "  Debt:\n")
		writeAmountLines(out, "    ", position.Debt)
		if len(position.Extra) > 0 {
			fmt.Fprintf(out, "  Extra:\n")
			writeIndentedJSON(out, "    ", position.Extra)
		}
	}

	if len(metadata) > 0 {
		fmt.Fprintln(out)
		writeMetadataDetail(out, metadata)
	}
	return nil
}

func writeExplainOutput(out io.Writer, target core.Chain, descriptors []core.ProtocolDescriptor, metadata []metadataSnapshot, opts outputOptions, events []traceEvent) error {
	if opts.Format == outputFormatJSON {
		payload := map[string]any{
			"chain":     target,
			"protocols": descriptors,
			"metadata":  metadata,
		}
		if opts.Trace {
			payload["trace"] = events
		}
		return writeJSON(out, payload)
	}

	if opts.Format == outputFormatDetail {
		fmt.Fprintf(out, "Explain\n")
		fmt.Fprintf(out, "Chain: %s (%d)\n", target.DisplayName, target.ID)
		fmt.Fprintf(out, "Protocols: %d\n\n", len(descriptors))
		for _, descriptor := range descriptors {
			fmt.Fprintf(out, "- %s (%s)\n", descriptor.Name, descriptor.ID)
			fmt.Fprintf(out, "  Category: %s\n", emptyDash(descriptor.Category))
			fmt.Fprintf(out, "  Description: %s\n", emptyDash(descriptor.Description))
		}
		if len(metadata) > 0 {
			fmt.Fprintln(out)
			writeMetadataDetail(out, metadata)
		}
		return writeTraceText(out, events)
	}

	fmt.Fprintf(out, "Explain: %s (%d)\n", target.DisplayName, target.ID)
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PROTOCOL\tCATEGORY\tMETADATA")
	for _, descriptor := range descriptors {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", descriptor.ID, descriptor.Category, summarizeProtocolMetadata(metadata, descriptor.ID))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	return writeTraceText(out, events)
}

func inspectMetadata(ctx context.Context, store cache.Store, target core.Chain, protocolIDs []string, maxAge time.Duration) []metadataSnapshot {
	out := make([]metadataSnapshot, 0)
	for _, protocolID := range protocolIDs {
		for _, namespace := range metadataNamespaces(protocolID) {
			snapshot := metadataSnapshot{
				Protocol:  protocolID,
				Namespace: namespace,
				Status:    "missing",
			}
			var raw json.RawMessage
			info, err := store.Get(ctx, cache.Key{
				ChainID:   target.ID,
				Protocol:  protocolID,
				Namespace: namespace,
			}, &raw)
			switch {
			case err == nil:
				infoCopy := info
				snapshot.Metadata = &infoCopy
				snapshot.Items = countJSONItems(raw)
				snapshot.Age = formatAge(info.UpdatedAt)
				snapshot.Status = "hit"
				if maxAge > 0 && metadataIsStale(info, maxAge) {
					snapshot.Status = "stale"
					snapshot.Stale = true
				}
			case errors.Is(err, cache.ErrNotFound):
				snapshot.Status = "missing"
			default:
				snapshot.Status = "error"
				snapshot.Error = err.Error()
			}
			out = append(out, snapshot)
		}
	}
	return out
}

func metadataNamespaces(protocolID string) []string {
	namespaces := knownMetadataNamespaces[strings.ToLower(strings.TrimSpace(protocolID))]
	if len(namespaces) == 0 {
		return []string{"markets"}
	}
	out := make([]string, len(namespaces))
	copy(out, namespaces)
	return out
}

func countJSONItems(raw json.RawMessage) int {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err == nil {
		return len(items)
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err == nil {
		return len(object)
	}
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	return 1
}

func metadataIsStale(info core.MetadataInfo, maxAge time.Duration) bool {
	if info.UpdatedAt.IsZero() {
		return true
	}
	return time.Since(info.UpdatedAt) > maxAge
}

func addMetadataTrace(trace *traceRecorder, metadata []metadataSnapshot) {
	for _, snapshot := range metadata {
		attrs := map[string]any{
			"protocol":  snapshot.Protocol,
			"namespace": snapshot.Namespace,
			"status":    snapshot.Status,
		}
		if snapshot.Items > 0 {
			attrs["items"] = snapshot.Items
		}
		if snapshot.Age != "" {
			attrs["age"] = snapshot.Age
		}
		if snapshot.Error != "" {
			attrs["error"] = snapshot.Error
		}
		trace.Add("cache", "metadata.read", fmt.Sprintf("%s/%s %s", snapshot.Protocol, snapshot.Namespace, snapshot.Status), attrs)
	}
}

func addSyncDiscoveryTrace(trace *traceRecorder, results []adapter.SyncResult) {
	if trace == nil || !trace.enabled {
		return
	}
	for _, result := range results {
		steps := discoveryStepsFromResult(result)
		if len(steps) == 0 {
			continue
		}
		if trace.level == traceLevelSummary {
			attrs := discoveryTotals(result.Metadata.Protocol, result.Items, steps)
			trace.Add("adapter", result.Metadata.Protocol+".sync.discovery", "metadata discovery completed", attrs)
			trace.Add("evm", "multicall.aggregate3.summary", "sync discovery contract reads were batched", attrs)
			continue
		}
		for _, step := range steps {
			attrs := map[string]any{
				"protocol": result.Metadata.Protocol,
				"step":     step.Step,
				"status":   step.Status,
			}
			if step.Markets > 0 {
				attrs["markets"] = step.Markets
			}
			if step.Calls > 0 {
				attrs["calls"] = step.Calls
			}
			if step.Items > 0 {
				attrs["items"] = step.Items
			}
			if step.Notes != "" {
				attrs["notes"] = step.Notes
			}
			if step.AllowFailure {
				attrs["allowFailure"] = true
			}
			trace.Add("adapter", result.Metadata.Protocol+".sync."+slugOperation(step.Step), step.Operation, attrs)
			if step.Calls > 0 {
				trace.Add("evm", "multicall.aggregate3", "sync "+step.Step, attrs)
			}
		}
	}
}

func discoveryStepsFromResult(result adapter.SyncResult) []discoveryStepView {
	if len(result.Details) == 0 {
		return nil
	}
	raw, ok := result.Details["discovery"]
	if !ok || raw == nil {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var steps []discoveryStepView
	if err := json.Unmarshal(data, &steps); err != nil {
		return nil
	}
	return steps
}

func writeDiscoverySteps(out io.Writer, prefix string, steps []discoveryStepView) {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%sSTEP\tOPERATION\tMARKETS\tCALLS\tITEMS\tSTATUS\tNOTES\n", prefix)
	for _, step := range steps {
		fmt.Fprintf(tw, "%s%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			prefix,
			step.Step,
			step.Operation,
			formatOptionalInt(step.Markets),
			formatOptionalInt(step.Calls),
			formatOptionalInt(step.Items),
			emptyDash(step.Status),
			emptyDash(step.Notes),
		)
	}
	_ = tw.Flush()
}

func discoveryTotals(protocol string, resultItems int, steps []discoveryStepView) map[string]any {
	totalCalls := 0
	totalItems := 0
	batches := 0
	allowFailure := false
	for _, step := range steps {
		totalCalls += step.Calls
		totalItems += step.Items
		if step.Calls > 0 {
			batches++
		}
		if step.AllowFailure {
			allowFailure = true
		}
	}
	return map[string]any{
		"protocol":         protocol,
		"steps":            len(steps),
		"multicallBatches": batches,
		"multicallCalls":   totalCalls,
		"stageItems":       totalItems,
		"resultItems":      resultItems,
		"allowFailure":     allowFailure,
	}
}

func slugOperation(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer(" ", "-", "/", "-", "_", "-", "+", "-", ",", "")
	return replacer.Replace(value)
}

func addPositionTrace(trace *traceRecorder, positions []core.Position) {
	if trace == nil || !trace.enabled {
		return
	}
	if trace.level == traceLevelSummary {
		addPositionTraceSummary(trace, positions)
		return
	}
	for _, position := range positions {
		switch position.Protocol {
		case "aave-v3":
			switch position.Type {
			case core.PositionTypeLending:
				addAaveLendingTrace(trace, position)
			case core.PositionTypeYield:
				addAaveYieldTrace(trace, position)
			}
		case "compound-v3":
			addCompoundTrace(trace, position)
		}
	}
}

func addPositionTraceSummary(trace *traceRecorder, positions []core.Position) {
	lendingPositions := 0
	yieldPositions := 0
	reserveReads := 0
	compoundYieldPositions := 0
	compoundLendingPositions := 0
	compoundRewardPositions := 0
	compoundCollateralReads := 0
	uniswapLiquidityPositions := 0
	uniswapFarmingPositions := 0
	uniswapRewardPositions := 0
	uniswapV3LiquidityPositions := 0
	uniswapV3RewardTokens := 0
	for _, position := range positions {
		switch position.Protocol {
		case "aave-v3":
			switch position.Type {
			case core.PositionTypeLending:
				lendingPositions++
				reserveReads += extraSliceLen(position.Extra, "reserves")
			case core.PositionTypeYield:
				yieldPositions++
			}
		case "compound-v3":
			switch extraString(position.Extra, "strategy") {
			case "yield":
				compoundYieldPositions++
			case "lending":
				compoundLendingPositions++
				compoundCollateralReads += extraInt(position.Extra, "collateralCount")
			case "reward":
				compoundRewardPositions++
			}
		case "uniswap-v2":
			switch extraString(position.Extra, "strategy") {
			case "farming":
				uniswapFarmingPositions++
			case "farming-reward":
				uniswapRewardPositions++
			default:
				if position.Type == core.PositionTypeLiquidity {
					uniswapLiquidityPositions++
				}
			}
		case "uniswap-v3":
			if position.Type == core.PositionTypeLiquidity {
				uniswapV3LiquidityPositions++
				uniswapV3RewardTokens += len(position.Rewards)
			}
		}
	}
	if lendingPositions > 0 || yieldPositions > 0 {
		attrs := map[string]any{
			"lendingPositions": lendingPositions,
			"yieldPositions":   yieldPositions,
			"reserveReads":     reserveReads,
			"allowFailure":     false,
		}
		trace.Add("adapter", "aave-v3.fetch.summary", "Aave V3 position fetch stages completed", attrs)
		trace.Add("evm", "multicall.aggregate3.summary", "contract reads were batched through Multicall3", attrs)
	}
	if compoundYieldPositions > 0 || compoundLendingPositions > 0 || compoundRewardPositions > 0 {
		attrs := map[string]any{
			"yieldPositions":   compoundYieldPositions,
			"lendingPositions": compoundLendingPositions,
			"rewardPositions":  compoundRewardPositions,
			"collateralReads":  compoundCollateralReads,
			"allowFailure":     "prices and rewards only",
		}
		trace.Add("adapter", "compound-v3.fetch.summary", "Compound V3 Comet position fetch stages completed", attrs)
		trace.Add("evm", "multicall.aggregate3.summary", "Comet user, oracle, collateral and reward reads were batched", attrs)
	}
	if uniswapLiquidityPositions > 0 || uniswapFarmingPositions > 0 || uniswapRewardPositions > 0 {
		attrs := map[string]any{
			"liquidityPositions": uniswapLiquidityPositions,
			"farmingPositions":   uniswapFarmingPositions,
			"rewardPositions":    uniswapRewardPositions,
			"allowFailure":       "feeTo and rewards only",
		}
		trace.Add("adapter", "uniswap-v2.fetch.summary", "Uniswap V2 liquidity and farming position fetch stages completed", attrs)
		trace.Add("evm", "multicall.aggregate3.summary", "LP balances, pair reserves and farming rewards were batched", attrs)
	}
	if uniswapV3LiquidityPositions > 0 {
		attrs := map[string]any{
			"liquidityPositions": uniswapV3LiquidityPositions,
			"rewardTokens":       uniswapV3RewardTokens,
			"allowFailure":       false,
		}
		trace.Add("adapter", "uniswap-v3.fetch.summary", "Uniswap V3 NFT liquidity positions and fees were fetched", attrs)
		trace.Add("evm", "multicall.aggregate3.summary", "NFT positions, pool state and tick fee growth were batched", attrs)
	}
}

func addAaveLendingTrace(trace *traceRecorder, position core.Position) {
	reserveCount := extraSliceLen(position.Extra, "reserves")
	attrs := map[string]any{
		"position":       position.DisplayName,
		"marketId":       extraString(position.Extra, "marketId"),
		"pool":           extraString(position.Extra, "pool"),
		"dataProvider":   extraString(position.Extra, "dataProvider"),
		"reserves":       reserveCount,
		"multicallCalls": reserveCount + 1,
		"allowFailure":   false,
	}
	trace.Add("adapter", "aave-v3.lending.fetch", "Pool.getUserAccountData + DataProvider.getUserReserveData", attrs)
	trace.Add("evm", "multicall.aggregate3", "batch user lending reads", attrs)
}

func addAaveYieldTrace(trace *traceRecorder, position core.Position) {
	attrs := map[string]any{
		"position":                 position.DisplayName,
		"marketId":                 extraString(position.Extra, "marketId"),
		"vault":                    extraString(position.Extra, "vault"),
		"vaultKind":                extraString(position.Extra, "vaultKind"),
		"factory":                  extraString(position.Extra, "factory"),
		"asset":                    extraTokenSymbol(position.Extra["asset"]),
		"aToken":                   extraTokenAddress(position.Extra["aToken"]),
		"claimableRewardsCount":    extraInt(position.Extra, "claimableRewardsCount"),
		"readMethods":              "balanceOf,previewRedeem,maxWithdraw,maxRedeem,getClaimableRewards",
		"allowFailure":             false,
		"usesERC4626PreviewRedeem": true,
	}
	trace.Add("adapter", "aave-v3.yield.fetch", "Vault balance and redeem preview reads", attrs)
	trace.Add("evm", "multicall.aggregate3", "batch user yield reads", attrs)
}

func addCompoundTrace(trace *traceRecorder, position core.Position) {
	strategy := extraString(position.Extra, "strategy")
	if strategy == "" {
		return
	}
	attrs := map[string]any{
		"position":     position.DisplayName,
		"strategy":     strategy,
		"marketId":     extraString(position.Extra, "marketId"),
		"comet":        extraString(position.Extra, "comet"),
		"debankPoolId": extraString(position.Extra, "debankPoolId"),
		"readMethods":  extraString(position.Extra, "readMethods"),
		"allowFailure": compoundAllowFailure(strategy),
	}
	if strategy == "lending" {
		attrs["collaterals"] = extraInt(position.Extra, "collateralCount")
		attrs["isLiquidatable"] = extraString(position.Extra, "isLiquidatable")
	} else if strategy == "reward" {
		attrs["rewards"] = extraString(position.Extra, "rewards")
		attrs["claimableRewardsCount"] = extraInt(position.Extra, "claimableRewardsCount")
	}
	trace.Add("adapter", "compound-v3."+strategy+".fetch", "Comet user position reads", attrs)
	trace.Add("evm", "multicall.aggregate3", "batch Compound V3 "+strategy+" reads", attrs)
}

func compoundAllowFailure(strategy string) string {
	switch strategy {
	case "reward":
		return "getRewardOwed"
	case "yield", "lending":
		return "getPrice"
	default:
		return "getPrice,getRewardOwed"
	}
}

func writeMetadataDetail(out io.Writer, metadata []metadataSnapshot) {
	fmt.Fprintln(out, "Metadata Cache:")
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PROTOCOL\tNAMESPACE\tSTATUS\tITEMS\tUPDATED\tAGE\tVERSION\tSOURCE")
	for _, snapshot := range metadata {
		updated := "-"
		version := "-"
		source := "-"
		if snapshot.Metadata != nil {
			updated = formatTime(snapshot.Metadata.UpdatedAt)
			version = emptyDash(snapshot.Metadata.Version)
			source = emptyDash(snapshot.Metadata.Source)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			snapshot.Protocol,
			snapshot.Namespace,
			snapshot.Status,
			formatOptionalInt(snapshot.Items),
			updated,
			emptyDash(snapshot.Age),
			version,
			source,
		)
	}
	_ = tw.Flush()
}

func writeTraceText(out io.Writer, events []traceEvent) error {
	if len(events) == 0 {
		return nil
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Trace")
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "LAYER\tOPERATION\tDETAIL")
	for _, event := range events {
		detail := event.Message
		if attrs := formatAttributes(event.Attributes); attrs != "" {
			if detail != "" {
				detail += " "
			}
			detail += attrs
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", event.Layer, event.Operation, detail)
	}
	return tw.Flush()
}

func formatAttributes(attrs map[string]any) string {
	if len(attrs) == 0 {
		return ""
	}
	keys := make([]string, 0, len(attrs))
	for key := range attrs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", key, attrs[key]))
	}
	return strings.Join(parts, " ")
}

func writeAmountLines(out io.Writer, prefix string, amounts []core.TokenAmount) {
	if len(amounts) == 0 {
		fmt.Fprintf(out, "%s-\n", prefix)
		return
	}
	for _, amount := range amounts {
		fmt.Fprintf(out, "%s- %s %s raw=%s token=%s\n",
			prefix,
			emptyDash(amount.Formatted),
			emptyDash(amount.Token.Symbol),
			emptyDash(amount.Raw),
			emptyDash(amount.Token.Address),
		)
	}
}

func writeIndentedJSON(out io.Writer, prefix string, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fmt.Fprintf(out, "%s%v\n", prefix, value)
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		fmt.Fprintf(out, "%s%s\n", prefix, line)
	}
}

func summaryPrimaryAmounts(position core.Position) string {
	if position.Type == core.PositionTypeLending {
		return formatAmounts(position.Underlying)
	}
	return formatAmounts(position.Shares)
}

func summaryUnderlyingAmounts(position core.Position) string {
	if position.Type == core.PositionTypeLending {
		return "-"
	}
	return formatAmounts(position.Underlying)
}

func formatAmounts(amounts []core.TokenAmount) string {
	if len(amounts) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(amounts))
	for _, amount := range amounts {
		value := strings.TrimSpace(amount.Formatted)
		if value == "" {
			value = amount.Raw
		}
		parts = append(parts, strings.TrimSpace(value+" "+amount.Token.Symbol))
	}
	return strings.Join(parts, ", ")
}

func positionHealth(position core.Position) string {
	if len(position.Extra) == 0 {
		return "-"
	}
	for _, key := range []string{"healthFactorFormatted", "liquidationHealthFormatted"} {
		value, ok := position.Extra[key]
		if ok && strings.TrimSpace(fmt.Sprint(value)) != "" {
			return fmt.Sprint(value)
		}
	}
	return "-"
}

func extraString(extra map[string]any, key string) string {
	if len(extra) == 0 {
		return ""
	}
	value, ok := extra[key]
	if !ok || value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func extraInt(extra map[string]any, key string) int {
	if len(extra) == 0 {
		return 0
	}
	switch value := extra[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func extraSliceLen(extra map[string]any, key string) int {
	if len(extra) == 0 {
		return 0
	}
	switch value := extra[key].(type) {
	case []map[string]any:
		return len(value)
	case []any:
		return len(value)
	default:
		return 0
	}
}

func extraTokenSymbol(value any) string {
	switch token := value.(type) {
	case core.Token:
		return token.Symbol
	case map[string]any:
		if symbol, ok := token["symbol"].(string); ok {
			return symbol
		}
	}
	return ""
}

func extraTokenAddress(value any) string {
	switch token := value.(type) {
	case core.Token:
		return token.Address
	case map[string]any:
		if address, ok := token["address"].(string); ok {
			return address
		}
	}
	return ""
}

func countPositionTypes(positions []core.Position) map[string]int {
	counts := make(map[string]int)
	for _, position := range positions {
		counts[string(position.Type)]++
	}
	return counts
}

func totalSyncItems(results []adapter.SyncResult) int {
	total := 0
	for _, result := range results {
		total += result.Items
	}
	return total
}

func summarizeProtocolMetadata(metadata []metadataSnapshot, protocolID string) string {
	parts := make([]string, 0)
	for _, snapshot := range metadata {
		if snapshot.Protocol != protocolID {
			continue
		}
		part := snapshot.Namespace + ":" + snapshot.Status
		if snapshot.Items > 0 {
			part += fmt.Sprintf("(%d)", snapshot.Items)
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.UTC().Format(time.RFC3339)
}

func formatAge(updatedAt time.Time) string {
	if updatedAt.IsZero() {
		return ""
	}
	age := time.Since(updatedAt)
	if age < 0 {
		age = 0
	}
	return age.Round(time.Second).String()
}

func formatOptionalInt(value int) string {
	if value == 0 {
		return "-"
	}
	return fmt.Sprint(value)
}

func emptyDash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}
