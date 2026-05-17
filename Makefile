.PHONY: test vet race cover run-demo sync-demo positions-demo

CACHE_DIR ?= .dpr-cache
DEMO_ADDRESS ?= 0x0000000000000000000000000000000000000001

test:
	go test ./...

vet:
	go vet ./...

race:
	go test -race ./...

cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out

sync-demo:
	go run ./cmd/dpr sync-metadata -chain ethereum -protocol demo -cache-dir $(CACHE_DIR)

positions-demo:
	go run ./cmd/dpr positions -chain ethereum -protocol demo -address $(DEMO_ADDRESS) -cache-dir $(CACHE_DIR)

run-demo: sync-demo positions-demo
