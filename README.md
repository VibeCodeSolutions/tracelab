# Tracelab

Cross-platform test-log hub for Android, Linux, and Windows debugging.

Tracelab collects structured logs, crashes, and screenshots from apps under
test (via HTTP/WebSocket) into a single SQLite store on a shared NTFS
partition, so the same history is available regardless of which OS is booted.

## Components

- **`tracelab-hub`** — Go daemon with HTTP `/ingest` + WS `/tail`, SQLite
  store, stacktrace detection, adb bridge.
- **`tracelab` CLI** — `run`, `tail`, `sessions`, `adb` for the terminal.
- **`tracelab-mcp`** — MCP server so Claude Code can query sessions, tail
  logs, inspect crashes, and drive Android devices via adb.
- **Dashboard** *(later)* — web UI for live tail and session browsing.

## Status

Early scaffolding. See plan in this repo / project board.

## Building

Requires Go 1.22+.

    cp tracelab.toml.example tracelab.toml
    make build      # → dist/tracelab-hub
    make run        # runs from source

Cross-compile for Windows:

    GOOS=windows GOARCH=amd64 go build -o dist/tracelab-hub.exe ./cmd/hub

Other targets: `make vet`, `make test`, `make tidy`, `make clean`.

## License

MIT
