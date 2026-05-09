# Tracelab Makefile — Linux dev convenience.
# Cross-compile targets see README.

GO       ?= go
DIST     ?= dist
HUB_BIN  := $(DIST)/tracelab-hub
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -X main.version=$(VERSION)

.PHONY: build run vet test tidy clean

build: $(DIST)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(HUB_BIN) ./cmd/hub

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
