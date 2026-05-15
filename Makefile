# Tracelab Makefile — Linux dev convenience.
# Cross-compile targets see README.

GO       ?= go
DIST     ?= dist
HUB_BIN  := $(DIST)/tracelab-hub
CLI_BIN  := $(DIST)/tracelab
MCP_BIN  := $(DIST)/tracelab-mcp
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -X main.version=$(VERSION)

.PHONY: build hub cli mcp cli-windows hub-windows mcp-windows mcp-linux run vet test tidy clean

# Default `build` produces all binaries for the local platform (Linux).
build: hub cli mcp

hub: $(DIST)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(HUB_BIN) ./cmd/hub

cli: $(DIST)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(CLI_BIN) ./cmd/cli

# Phase 2b S1: stub MCP server. Final tool surface decided in S2 (ADR-007).
mcp: $(DIST)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(MCP_BIN) ./cmd/mcp

# Cross-compile to Windows amd64. CGO_ENABLED=0 keeps the build pure-Go
# (no MinGW required) — modernc.org/sqlite is already CGO-free, cobra has
# no C deps, so all binaries cross-build cleanly.
hub-windows: $(DIST)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(DIST)/tracelab-hub.exe ./cmd/hub

cli-windows: $(DIST)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(DIST)/tracelab.exe ./cmd/cli

# Explicit Linux build for mcp (mirrors hub/cli's default-target pattern;
# kept symmetrical with mcp-windows for predictable CI matrix usage).
mcp-linux: $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(DIST)/tracelab-mcp-linux ./cmd/mcp

mcp-windows: $(DIST)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(DIST)/tracelab-mcp.exe ./cmd/mcp

run:
	$(GO) run ./cmd/hub

vet:
	$(GO) vet ./...

test:
	$(GO) test -race ./...

tidy:
	$(GO) mod tidy

clean:
	rm -rf $(DIST)

$(DIST):
	mkdir -p $(DIST)
