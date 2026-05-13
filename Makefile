# Tracelab Makefile — Linux dev convenience.
# Cross-compile targets see README.

GO       ?= go
DIST     ?= dist
HUB_BIN  := $(DIST)/tracelab-hub
CLI_BIN  := $(DIST)/tracelab
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -X main.version=$(VERSION)

.PHONY: build hub cli cli-windows hub-windows run vet test tidy clean

# Default `build` produces both binaries for the local platform (Linux).
build: hub cli

hub: $(DIST)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(HUB_BIN) ./cmd/hub

cli: $(DIST)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(CLI_BIN) ./cmd/cli

# Cross-compile to Windows amd64. CGO_ENABLED=0 keeps the build pure-Go
# (no MinGW required) — modernc.org/sqlite is already CGO-free, cobra has
# no C deps, so both binaries cross-build cleanly.
hub-windows: $(DIST)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(DIST)/tracelab-hub.exe ./cmd/hub

cli-windows: $(DIST)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(DIST)/tracelab.exe ./cmd/cli

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
