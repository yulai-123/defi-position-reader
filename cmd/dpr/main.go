package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yulai-123/defi-position-reader/pkg/adapter"
	"github.com/yulai-123/defi-position-reader/pkg/cache"
	"github.com/yulai-123/defi-position-reader/pkg/chain"
	"github.com/yulai-123/defi-position-reader/pkg/core"
	"github.com/yulai-123/defi-position-reader/pkg/service"
	"github.com/yulai-123/defi-position-reader/protocols/aavev3"
	"github.com/yulai-123/defi-position-reader/protocols/compoundv3"
	"github.com/yulai-123/defi-position-reader/protocols/demo"
	"github.com/yulai-123/defi-position-reader/protocols/uniswapv2"
)

const defaultCacheDir = ".dpr-cache"
const defaultCacheDBName = "metadata.sqlite"

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		printUsage(out)
		return nil
	}

	switch args[0] {
	case "chains":
		return runChains(out)
	case "protocols":
		return runProtocols(args[1:], out)
	case "sync-metadata":
		return runSyncMetadata(ctx, args[1:], out)
	case "positions":
		return runPositions(ctx, args[1:], out)
	case "explain":
		return runExplain(ctx, args[1:], out)
	case "help", "-h", "--help":
		printUsage(out)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runChains(out io.Writer) error {
	return writeJSON(out, chain.DefaultChains())
}

func runProtocols(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("protocols", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	chainName := fs.String("chain", "", "filter protocols by chain name")
	if err := fs.Parse(args); err != nil {
		return err
	}

	registry, err := defaultRegistry()
	if err != nil {
		return err
	}

	descriptors := registry.List()
	if *chainName != "" {
		target, ok := chain.ByName(*chainName)
		if !ok {
			return fmt.Errorf("unknown chain %q", *chainName)
		}

		filtered := descriptors[:0]
		for _, descriptor := range descriptors {
			if descriptor.SupportsChain(target.ID) {
				filtered = append(filtered, descriptor)
			}
		}
		descriptors = filtered
	}

	return writeJSON(out, descriptors)
}

func runSyncMetadata(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("sync-metadata", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	chainName := fs.String("chain", "ethereum", "chain name: ethereum, arbitrum, base")
	protocols := fs.String("protocol", "all", "protocol id or comma separated protocol ids")
	owner := fs.String("address", "", "optional wallet address for protocols that support user-scoped metadata sync")
	cacheDir := fs.String("cache-dir", defaultCacheDir, "metadata cache directory")
	format := fs.String("format", "json", "output format: json, table, detail")
	traceEnabled := fs.Bool("trace", false, "show execution trace")
	traceLevel := fs.String("trace-level", "summary", "trace level: summary, calls, raw")
	if err := fs.Parse(args); err != nil {
		return err
	}
	opts, err := parseOutputOptions(*format, *traceLevel, *traceEnabled)
	if err != nil {
		return err
	}

	target, ok := chain.ByName(*chainName)
	if !ok {
		return fmt.Errorf("unknown chain %q", *chainName)
	}

	selectedProtocols := parseProtocols(*protocols)
	resolvedProtocols, err := resolveProtocolIDs(selectedProtocols, target)
	if err != nil {
		return err
	}

	positionService, store, err := defaultService(*cacheDir)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	trace := newTraceRecorder(opts)
	trace.Add("cli", "sync-metadata.request", "starting metadata sync", map[string]any{
		"chain":     target.Name,
		"chainId":   target.ID,
		"owner":     strings.TrimSpace(*owner),
		"protocols": resolvedProtocols,
		"cacheDir":  *cacheDir,
		"format":    opts.Format,
	})

	result, err := positionService.SyncMetadata(ctx, service.SyncRequest{
		Chain:     target,
		Owner:     *owner,
		Protocols: selectedProtocols,
	})
	if err != nil {
		return err
	}
	trace.Add("service", "sync-metadata.complete", "metadata sync completed", map[string]any{
		"results": len(result),
		"items":   totalSyncItems(result),
	})
	addSyncDiscoveryTrace(trace, result)

	metadata := inspectMetadata(ctx, store, target, resolvedProtocols, 0)
	addMetadataTrace(trace, metadata)
	return writeSyncOutput(out, target, result, metadata, opts, trace.Events())
}

func runPositions(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("positions", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	chainName := fs.String("chain", "ethereum", "chain name: ethereum, arbitrum, base")
	owner := fs.String("address", "", "wallet address")
	protocols := fs.String("protocol", "all", "protocol id or comma separated protocol ids")
	cacheDir := fs.String("cache-dir", defaultCacheDir, "metadata cache directory")
	metadataMaxAge := fs.Duration("metadata-max-age", 24*time.Hour, "maximum metadata age before positions fail and ask for sync-metadata")
	format := fs.String("format", "json", "output format: json, table, detail")
	traceEnabled := fs.Bool("trace", false, "show execution trace")
	traceLevel := fs.String("trace-level", "summary", "trace level: summary, calls, raw")
	if err := fs.Parse(args); err != nil {
		return err
	}
	opts, err := parseOutputOptions(*format, *traceLevel, *traceEnabled)
	if err != nil {
		return err
	}

	target, ok := chain.ByName(*chainName)
	if !ok {
		return fmt.Errorf("unknown chain %q", *chainName)
	}

	selectedProtocols := parseProtocols(*protocols)
	resolvedProtocols, err := resolveProtocolIDs(selectedProtocols, target)
	if err != nil {
		return err
	}

	positionService, store, err := defaultService(*cacheDir)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	trace := newTraceRecorder(opts)
	trace.Add("cli", "positions.request", "starting position fetch", map[string]any{
		"chain":          target.Name,
		"chainId":        target.ID,
		"owner":          strings.TrimSpace(*owner),
		"protocols":      resolvedProtocols,
		"cacheDir":       *cacheDir,
		"metadataMaxAge": metadataMaxAge.String(),
		"format":         opts.Format,
	})
	metadata := inspectMetadata(ctx, store, target, resolvedProtocols, *metadataMaxAge)
	addMetadataTrace(trace, metadata)

	positions, err := positionService.FetchPositions(ctx, service.FetchRequest{
		Chain:          target,
		Owner:          *owner,
		Protocols:      selectedProtocols,
		MetadataMaxAge: *metadataMaxAge,
	})
	if err != nil {
		return err
	}
	addPositionTrace(trace, positions)
	trace.Add("service", "positions.complete", "position fetch completed", map[string]any{
		"positions": len(positions),
		"types":     countPositionTypes(positions),
	})
	return writePositionsOutput(out, target, strings.TrimSpace(*owner), positions, metadata, opts, trace.Events())
}

func runExplain(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("explain", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	chainName := fs.String("chain", "ethereum", "chain name: ethereum, arbitrum, base")
	protocols := fs.String("protocol", "all", "protocol id or comma separated protocol ids")
	cacheDir := fs.String("cache-dir", defaultCacheDir, "metadata cache directory")
	format := fs.String("format", "table", "output format: json, table, detail")
	traceEnabled := fs.Bool("trace", false, "show execution trace")
	traceLevel := fs.String("trace-level", "summary", "trace level: summary, calls, raw")
	if err := fs.Parse(args); err != nil {
		return err
	}
	opts, err := parseOutputOptions(*format, *traceLevel, *traceEnabled)
	if err != nil {
		return err
	}

	target, ok := chain.ByName(*chainName)
	if !ok {
		return fmt.Errorf("unknown chain %q", *chainName)
	}

	selectedProtocols := parseProtocols(*protocols)
	descriptors, err := resolveProtocolDescriptors(selectedProtocols, target)
	if err != nil {
		return err
	}

	store, err := defaultStore(*cacheDir)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	protocolIDs := make([]string, 0, len(descriptors))
	for _, descriptor := range descriptors {
		protocolIDs = append(protocolIDs, descriptor.ID)
	}

	trace := newTraceRecorder(opts)
	trace.Add("cli", "explain.request", "explaining protocol metadata", map[string]any{
		"chain":     target.Name,
		"chainId":   target.ID,
		"protocols": protocolIDs,
		"cacheDir":  *cacheDir,
		"format":    opts.Format,
	})
	metadata := inspectMetadata(ctx, store, target, protocolIDs, 0)
	addMetadataTrace(trace, metadata)
	return writeExplainOutput(out, target, descriptors, metadata, opts, trace.Events())
}

func defaultService(cacheDir string) (*service.PositionService, *cache.SQLiteStore, error) {
	registry, err := defaultRegistry()
	if err != nil {
		return nil, nil, err
	}
	store, err := defaultStore(cacheDir)
	if err != nil {
		return nil, nil, err
	}
	return service.NewPositionService(registry, store), store, nil
}

func defaultStore(cacheDir string) (*cache.SQLiteStore, error) {
	return cache.NewSQLiteStore(filepath.Join(cacheDir, defaultCacheDBName))
}

func defaultRegistry() (*adapter.Registry, error) {
	return adapter.NewRegistry(aavev3.New(), compoundv3.New(), demo.New(), uniswapv2.New())
}

func resolveProtocolIDs(protocolIDs []string, target core.Chain) ([]string, error) {
	descriptors, err := resolveProtocolDescriptors(protocolIDs, target)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(descriptors))
	for _, descriptor := range descriptors {
		ids = append(ids, descriptor.ID)
	}
	return ids, nil
}

func resolveProtocolDescriptors(protocolIDs []string, target core.Chain) ([]core.ProtocolDescriptor, error) {
	registry, err := defaultRegistry()
	if err != nil {
		return nil, err
	}
	items, err := registry.Resolve(protocolIDs, target.ID)
	if err != nil {
		return nil, err
	}
	descriptors := make([]core.ProtocolDescriptor, 0, len(items))
	for _, item := range items {
		descriptors = append(descriptors, item.Descriptor())
	}
	return descriptors, nil
}

func parseProtocols(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "all") {
		return nil
	}

	parts := strings.Split(value, ",")
	protocols := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			protocols = append(protocols, part)
		}
	}
	return protocols
}

func writeJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func printUsage(out io.Writer) {
	fmt.Fprintln(out, "DeFi Position Reader")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  dpr chains")
	fmt.Fprintln(out, "  dpr protocols [-chain ethereum]")
	fmt.Fprintln(out, "  dpr sync-metadata [-chain ethereum] [-protocol demo|aave-v3|compound-v3|uniswap-v2] [-address 0x...] [-cache-dir .dpr-cache] [-format json|table|detail] [-trace] [-trace-level summary|calls|raw]")
	fmt.Fprintln(out, "  dpr positions -address 0x... [-chain ethereum] [-protocol demo|aave-v3|compound-v3|uniswap-v2] [-cache-dir .dpr-cache] [-metadata-max-age 24h] [-format json|table|detail] [-trace] [-trace-level summary|calls|raw]")
	fmt.Fprintln(out, "  dpr explain [-chain ethereum] [-protocol demo|aave-v3|compound-v3|uniswap-v2] [-cache-dir .dpr-cache] [-format json|table|detail] [-trace] [-trace-level summary|calls|raw]")
}
