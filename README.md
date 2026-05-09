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

## Storage

Tracelab persists everything to a single SQLite database, by default on the
shared NTFS partition under `/run/media/kaik/AE62672C6266F88B/tracelab/`
(configurable via `tracelab.toml`). The store uses `modernc.org/sqlite`
(pure-Go, CGO-free) so the same binary cross-compiles to Linux and Windows.

Schema (migration `0001_initial`):

- `sessions(id TEXT PK, label, started_at, ended_at)` — one row per
  test/debug run. IDs are 26-char lexicographically sortable hex strings.
- `events(id, session_id FK→sessions ON DELETE CASCADE, ts, source, level, msg, meta)`
  — append-only log/event stream, indexed by `(session_id, ts)`.
- `crashes(id, session_id FK, ts, fingerprint, stacktrace, count)` —
  deduplicated stacktrace clusters per session.
- `screenshots(id, session_id FK, ts, path, trigger)` — paths are
  stored relative to the configured datastore directory.

Migrations are embedded into the binary (`//go:embed`) and applied
idempotently on `Open` via a `schema_migrations` version table; no
external migration tool is required at runtime.

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
