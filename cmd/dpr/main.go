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

	"github.com/yulai-123/defi-position-reader/pkg/adapter"
	"github.com/yulai-123/defi-position-reader/pkg/cache"
	"github.com/yulai-123/defi-position-reader/pkg/chain"
	"github.com/yulai-123/defi-position-reader/pkg/service"
	"github.com/yulai-123/defi-position-reader/protocols/demo"
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
	cacheDir := fs.String("cache-dir", defaultCacheDir, "metadata cache directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	target, ok := chain.ByName(*chainName)
	if !ok {
		return fmt.Errorf("unknown chain %q", *chainName)
	}

	positionService, err := defaultService(*cacheDir)
	if err != nil {
		return err
	}

	result, err := positionService.SyncMetadata(ctx, service.SyncRequest{
		Chain:     target,
		Protocols: parseProtocols(*protocols),
	})
	if err != nil {
		return err
	}
	return writeJSON(out, result)
}

func runPositions(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("positions", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	chainName := fs.String("chain", "ethereum", "chain name: ethereum, arbitrum, base")
	owner := fs.String("address", "", "wallet address")
	protocols := fs.String("protocol", "all", "protocol id or comma separated protocol ids")
	cacheDir := fs.String("cache-dir", defaultCacheDir, "metadata cache directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	target, ok := chain.ByName(*chainName)
	if !ok {
		return fmt.Errorf("unknown chain %q", *chainName)
	}

	positionService, err := defaultService(*cacheDir)
	if err != nil {
		return err
	}

	positions, err := positionService.FetchPositions(ctx, service.FetchRequest{
		Chain:     target,
		Owner:     *owner,
		Protocols: parseProtocols(*protocols),
	})
	if err != nil {
		return err
	}
	return writeJSON(out, positions)
}

func defaultService(cacheDir string) (*service.PositionService, error) {
	registry, err := defaultRegistry()
	if err != nil {
		return nil, err
	}
	store, err := cache.NewSQLiteStore(filepath.Join(cacheDir, defaultCacheDBName))
	if err != nil {
		return nil, err
	}
	return service.NewPositionService(registry, store), nil
}

func defaultRegistry() (*adapter.Registry, error) {
	return adapter.NewRegistry(demo.New())
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
	fmt.Fprintln(out, "  dpr sync-metadata [-chain ethereum] [-protocol demo] [-cache-dir .dpr-cache]")
	fmt.Fprintln(out, "  dpr positions -address 0x... [-chain ethereum] [-protocol demo] [-cache-dir .dpr-cache]")
}
