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

## API

The hub exposes a small JSON HTTP API on `[server].bind:[server].port`
(default `127.0.0.1:8765`). All endpoints except `/healthz` require a
`Authorization: Bearer <token>` header matching `[auth].token`. Generate
the token with `openssl rand -hex 32`.

| Method | Path             | Auth | Purpose                                   |
|--------|------------------|------|-------------------------------------------|
| GET    | `/healthz`       | no   | Liveness probe — `{"status":"ok"}`.       |
| POST   | `/session/start` | yes  | Start a session, returns `session_id`.    |
| POST   | `/session/end`   | yes  | Mark a session ended (204 on success).    |
| POST   | `/ingest`        | yes  | Batch-insert events (202 on accept).      |
| GET    | `/sessions`      | yes  | List recent sessions (`?limit=N`).        |

Examples (assuming `TOKEN=$(cat .token)`):

    curl http://127.0.0.1:8765/healthz

    curl -H "Authorization: Bearer $TOKEN" \
         -X POST http://127.0.0.1:8765/session/start \
         -d '{"label":"manual-smoke"}'

    curl -H "Authorization: Bearer $TOKEN" \
         -X POST http://127.0.0.1:8765/ingest \
         -d '{"session_id":"<id>","events":[
              {"source":"app","level":"INFO","msg":"hello"},
              {"source":"app","level":"ERROR","msg":"boom","meta":{"k":"v"}}
            ]}'

    curl -H "Authorization: Bearer $TOKEN" \
         -X POST http://127.0.0.1:8765/session/end \
         -d '{"session_id":"<id>"}'

    curl -H "Authorization: Bearer $TOKEN" \
         "http://127.0.0.1:8765/sessions?limit=20"

Event timestamps (`ts`) are optional unix-nanoseconds; the hub fills in
`time.Now()` when omitted. `meta` is opaque JSON.

## Building

Requires Go 1.25+ (pulled in by `modernc.org/sqlite` ≥ 1.50; the toolchain
self-upgrades automatically with `GOTOOLCHAIN=auto`).

    cp tracelab.toml.example tracelab.toml
    make build      # → dist/tracelab-hub
    make run        # runs from source

Cross-compile for Windows:

    GOOS=windows GOARCH=amd64 go build -o dist/tracelab-hub.exe ./cmd/hub

Other targets: `make vet`, `make test`, `make tidy`, `make clean`.

## License

MIT
