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
- **Dashboard** — web UI for live tail, session browsing and crash
  inspection. Phase 2c S1 ships the skeleton (layout + 4-tab navigation
  + embedded htmx/CSS); S2–S5 fill the tab bodies.

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
| GET    | `/events`        | yes  | Forward-cursor read: `?session=<id>&since_seq=<n>&limit=<n>` returns `{events, next_since_seq}` (Phase 2b S4, ADR-008). |
| GET    | `/crashes`       | yes  | Session-scoped crash digest, newest first: `?session=<id>&limit=<n>` returns `{crashes}` (Phase 2b S6, ADR-009). |
| GET    | `/tail`          | yes  | WebSocket fan-out, optional `?session=<id>` filter. |
| GET    | `/dashboard`     | no¹  | Web dashboard (Phase 2c S1 skeleton). Optional `?tab=<slug>` (`live-tail`/`sessions`/`crashes`/`agents`). |
| GET    | `/dashboard/tab/{slug}` | no¹ | Tab body for htmx partial-swap. |
| GET    | `/dashboard/static/*`   | no¹ | Embedded JS/CSS (htmx, dashboard.css). |

¹ Phase 2c S1 leaves the dashboard sub-router *unauthenticated*: browsers
cannot attach an `Authorization:` header to `<script src=…>` / page loads,
and a query-string token contradicts the API rule below. A cookie-based
auth wrap (or a short-lived dashboard session token) is the subject of an
upcoming ADR — see `docs/ARCH.md` ADR-011 *Consequences* and the
ADR-012 follow-up bookmark. Until then the operational assumption is the
Phase-1 default bind `127.0.0.1:8765` (loopback only, single-user dev
host).

The token must be sent as an `Authorization: Bearer <token>` **header** —
the hub does not accept the token as a query string parameter (also not on
`/tail`, where browsers cannot set custom WebSocket headers; use a CLI
client like `websocat -H` instead).

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

## ADB Bridge (optional)

The hub can spawn a background goroutine that runs `adb logcat -v threadtime`
against an Android device and ingests every line as an event with
`source="adb"`.

Unlike `/ingest` — which persists to SQLite first and then publishes to
`/tail` — the adb bridge inverts the order: every line is published to
`/tail` subscribers synchronously (sub-millisecond fan-out, no batching),
while DB writes are batched (whichever comes first: ~50 lines or ~50ms).
This is deliberate. WebSocket fan-out and SQLite persistence are treated
as two independent audit channels: the live stream is preserved even if a
later DB write fails, so forensic consumers attached to `/tail` see the
full sequence regardless of storage hiccups. Trade-off: the DB may lag
the live stream by up to one batch interval — fine for post-mortem
review, intentional for live tailing.

Each reconnect (subprocess EOF, device unplug, daemon bounce) opens a
**new session** so disconnect gaps are visible in the forensic trail
rather than hidden inside one long session. The bridge backs off
between reconnect attempts (1s / 2s / 5s / 10s, then constant 10s).

Configuration block in `tracelab.toml`:

    [adb]
    enabled       = true              # off by default
    device_serial = "emulator-5554"   # empty = pick the only attached device
    tag_filter    = ""                # empty = stream every tag (otherwise <tag>:V *:S)

Logcat priorities map to event levels as:

    V, D  →  debug
    I     →  info
    W     →  warn
    E, F  →  error
    S     →  ignored (it is a logcat filter directive, not a real level)

`meta` carries `{pid, tid, timestamp (RFC3339), level_raw, device_serial}`.

### Smoke

    adb devices                        # confirm the device is attached
    # set [adb].enabled = true and (optionally) device_serial in tracelab.toml
    make run                           # or: ./dist/tracelab-hub

The hub logs `"adb bridge enabled"` on start and `"adb bridge starting"`
once a session is created. To watch the live stream, subscribe to
`/tail`. The session id appears in slog (`session_id=...`); pass it
to `/tail?session=<id>` to filter, or omit `?session=` to watch all
sessions:

    TOKEN=$(awk -F\" '/^token/{print $2}' tracelab.toml)
    websocat -H "Authorization: Bearer $TOKEN" \
        "ws://127.0.0.1:8765/tail"

A new session id is logged after each reconnect — copy the latest one
if you want to follow only the current adb stream.

On shutdown, slog emits the stop-ordering markers in this order:

    "adb bridge stopped" → "websocket hub closed" → "http server stopped"

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
