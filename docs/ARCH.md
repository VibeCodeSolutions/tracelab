# Tracelab — Architecture Decisions

> Living document. Phase 1 decisions are codified in commit history + README.
> This file records explicit Phase-2+ architecture choices that affect more
> than one package or future phases.

## Phase 2 — Tool-Chain (CLI, MCP, Dashboard)

Roadmap: `~/.claude/plans/tracelab-phase-2-roadmap.md`.
Phase split: 2a CLI → 2b MCP → 2c Dashboard (linear, per-phase QS).

### Phase 2a — `tracelab` CLI

#### ADR-001: CLI framework = `spf13/cobra`

**Decision:** Use `github.com/spf13/cobra` for command structure.

**Why:**
- De-facto standard for Go CLIs with sub-commands (`kubectl`, `gh`, `hugo`).
- Mature sub-command hierarchy fits `tracelab {run,tail,sessions,adb}` natively.
- Generates good `--help` output and shell completion.

**Considered & rejected:**
- stdlib `flag` — too primitive for nested sub-commands.
- `urfave/cli` — smaller surface, but less idiomatic for the `cmd verb` style we want.

**Cost:** ~10 transitive deps. Acceptable — already comparable surface to `chi`.

#### ADR-002: Config = shared `tracelab.toml`, new `[cli]` section

**Decision:** CLI reads the same `tracelab.toml` the hub uses. New optional
`[cli]` section for CLI-only knobs. Server URL + token are derived from the
existing `[server]` and `[auth]` sections — no duplication.

**Why:**
- Single source of truth on a dev/test host (typically same machine runs hub
  and CLI). Token rotation happens in one place.
- Aligns with Phase-1 decision that `tracelab.toml` is the canonical config.

**Discovery order:**
1. `--config <path>` flag
2. `$TRACELAB_CONFIG`
3. `./tracelab.toml` (cwd)
4. `$XDG_CONFIG_HOME/tracelab/tracelab.toml`
5. `~/.config/tracelab/tracelab.toml`

**Per-invocation overrides:** `--url`, `--token`, env vars `TRACELAB_URL` /
`TRACELAB_TOKEN`.

**New `[cli]` section (initial keys):**
```toml
[cli]
default_format = "table"   # table | json
color          = "auto"    # auto | always | never
tail_buffer    = 1024
```

**Considered & rejected:**
- Separate `.tracelab-cli.toml` — violates single-source-of-truth, forces
  duplicated token on rotation.

#### ADR-003: Shared client package `internal/client/`

**Decision:** Extract a reusable Hub client into `internal/client/`. Both
the CLI (2a) and the MCP server (2b) consume it.

**Initial surface:**
```go
type Config struct {
    BaseURL string
    Token   string
    Timeout time.Duration
}

func New(cfg Config) (*Client, error)

func (*Client) Health(ctx) error
func (*Client) StartSession(ctx, label string) (id string, err error)
func (*Client) EndSession(ctx, id string) error
func (*Client) Ingest(ctx, id string, events []Event) (accepted int, err error)
func (*Client) ListSessions(ctx, limit int) ([]Session, error)
func (*Client) Tail(ctx, sessionFilter string, onEvent func(Event)) error
```

**Why:**
- DRY between CLI and MCP server (both speak the same HTTP+WS API).
- Bearer-auth, retry/backoff, and JSON marshalling live in one place.
- Testable against `httptest.Server` without touching CLI or MCP code.

**Type sharing with the hub:** initial approach — `internal/client/` defines
its own DTOs (mirror of hub response shapes). Avoids importing `internal/store/`
into a client package and keeps Phase-1 packages untouched. If divergence
shows up, lift the DTOs into `internal/types/` later.

**WebSocket lib:** `gorilla/websocket` (same as hub — already a repo dep).

#### Code layout (new)

```
cmd/cli/         # new — tracelab binary
internal/client/ # new — shared HTTP+WS client
```

No package moves in Phase 2a. Hub code stays untouched.

#### Sub-sprint cut (proposed)

| Sprint | Scope | Notes |
|---|---|---|
| **S1 — Skeleton** | `cmd/cli/main.go`, cobra root + sub-cmd stubs, Makefile cross-compile target | no logic yet, just `--help` works |
| **S2 — Client package** | `internal/client/` HTTP endpoints (no `Tail` yet), unit-tests via `httptest` | foundations for S3+S4 |
| **S3 — `sessions` sub-cmd** | list sessions with `--limit`, `--format=table|json` | first end-to-end use of S2 |
| **S4 — `tail` sub-cmd** | WS-loop in client (`Tail`), CLI consumer with `--session=<id>`, color by level, SIGINT clean close | finishes the read-side |
| **S5 — `adb` sub-cmd** | hub-mediated (ADR-004 = Option B): new endpoints `GET /adb/devices`, `POST /adb/start`, `POST /adb/stop` + CLI thin client | Hub schema-change, decided 2026-05-14 |
| ~~S6 — `run` sub-cmd~~ | **dropped (ADR-005 = Option C, decided 2026-05-14)** — `tracelab-hub` ist Daemon-Start, CLI bleibt purer Consumer |

S1–S4 are well-defined and can proceed once ADR-001/-002/-003 are approved.
S5 and S6 each require an explicit decision before they enter implementation.

### Open ADRs — Auto-Stop before S5 / S6

#### ADR-004: `tracelab adb` scope — Option B (Admin-decided 2026-05-14)

The hub today has an internal ADB bridge (`internal/adb/` + bridge goroutine
in the daemon). It is **not** exposed via HTTP — there is no `/adb/devices`
or `/adb/start` endpoint.

The CLI's `adb` sub-command therefore has two plausible shapes:

**Option A — local ADB, no hub involvement**
- CLI imports `internal/adb/` directly.
- `tracelab adb devices` runs `adb devices` locally and prints the list.
- `tracelab adb tail <serial>` could stream logcat directly (bypassing the hub).
- Pro: no hub change, simple.
- Con: doesn't help when CLI and hub run on different machines; doesn't drive
  the hub's bridge (so the hub-recorded session is unaffected).

**Option B — hub-mediated, new endpoints**
- New endpoints: `GET /adb/devices`, `POST /adb/start`, `POST /adb/stop`.
- Hub becomes the single point that talks to `adb`; CLI is a thin client.
- Pro: works across machines, hub session integrity preserved, MCP server
  (2b) can drive the same endpoints.
- Con: **schema/API change to a Phase-1-merged surface** — explicit
  Auto-Stop trigger per plan briefing.

**Recommendation (belanna):** **Option B**, because (a) the MCP server in
Phase 2b will want exactly these endpoints to let Claude Code drive ADB, and
(b) adding them now means we design CLI and MCP consumers against the same
surface from day one. The API change is small and additive (no breakage of
existing endpoints).

**Decision (2026-05-14):** **Option B.** Reason confirmed with Admin: the hub
is the single sammelpunkt for all debug streams in the product vision —
local ADB at the CLI would bypass the hub-recording for ADB sessions and
break the „all debugs land at one point"-principle. Schema-Change at the
Phase-1-merged hub is the explicit Auto-Stop cost; Admin grün given.
S5 implements three new additive Hub-endpoints + CLI thin client.

#### ADR-005: `tracelab run` semantics — Option C (Admin-decided 2026-05-14)

What does `tracelab run` do?

**Option A — foreground wrapper**
- `tracelab run` execs `tracelab-hub` as a foreground process (or imports
  the hub `main` and runs it in-process).
- Pro: trivial, no new lifecycle management.
- Con: blurs the line between CLI and hub binary; user could just run
  `tracelab-hub` directly.

**Option B — daemon control plane**
- `tracelab run start|stop|status` manages a background hub process.
- Linux: writes a PID file under `$XDG_RUNTIME_DIR/tracelab.pid`, sends
  SIGTERM on `stop`.
- Windows: spawns a detached process, uses a state file in `%LOCALAPPDATA%`.
- No systemd unit shipped by default (kept optional).
- Pro: makes `tracelab` a full operator tool.
- Con: cross-platform daemon management is non-trivial; risks duplicating
  what systemd/launchd already do.

**Option C — drop `run` from Phase 2a**
- Document `tracelab-hub` as the way to start the daemon (already works).
- `tracelab` becomes purely a consumer (tail, sessions, adb).
- Pro: smallest scope, fastest Phase 2a; can revisit `run` later if there
  is real demand.
- Con: DoD as written in WORKLOG #010 includes `run`. Needs Admin OK to drop.

**Recommendation (belanna):** **Option C** for Phase 2a, with the
understanding that we revisit after CLI+MCP are in users' hands. Daemon
management is a separate problem from log consumption, and option B is
roughly its own sprint.

**Decision (2026-05-14):** **Option C.** Admin grün gegeben — `run` aus
Phase 2a gestrichen, S6 wird nicht implementiert. `tracelab-hub` bleibt
der Daemon-Start, CLI ist purer Consumer (`sessions`/`tail`/`adb`). Bei
realem Bedarf nach Daemon-Management später eigener Sprint mit Option B —
für jetzt ist die Trennung CLI=Consumer + Hub=Daemon sauberer. DoD von
AUFTRAG #010 entsprechend angepasst (siehe WORKLOG).

---

## API Conventions (cross-phase)

Conventions that apply to every new Hub HTTP/WS endpoint. Established during
Phase 1 (`/ingest`) and reaffirmed in Phase 2a S5 (`/adb/start`, `/adb/stop`).
New endpoints must follow these — divergence needs an ADR.

### Idempotent state-mutating endpoints → 200 OK + discriminator body

Endpoints that drive a state machine (start/stop, enable/disable, attach/detach)
return **HTTP 200** in both the "did-something" and the "already-there" case,
and put the actual outcome in a JSON `status` field on the body.

**Canonical shape:**

```json
POST /adb/start  { "device_serial": "ABC123" }
→ 200 OK
  { "status": "started" }         // bridge transitioned: stopped → running
  { "status": "already_running" } // bridge was already running for this serial

POST /adb/stop   { "device_serial": "ABC123" }
→ 200 OK
  { "status": "stopped" }
  { "status": "not_running" }
```

**Why not 409 / 404 for the no-op case:**

- Scripted operators want **`ensure-running`** and **`ensure-stopped`** semantics.
  They branch on the body (`status == "started" || "already_running"` → success),
  not on the HTTP status code. Mapping the no-op case to 409/404 forces every
  caller to special-case non-2xx as "actually fine", which is exactly the kind
  of fragile shell glue we are trying to spare downstream tooling.
- This mirrors the Phase-1 `/ingest` pattern: it always returns 202, never 4xx
  for "session was already there" — the body carries the nuance.
- HTTP status codes are reserved for **real failures**: auth (401), bad input
  (400), server errors (5xx). State-machine no-ops are not errors.

**Discriminator naming:** `status` is a flat string at the top level of the
response body. Values are lowercase, snake_case, past-tense verbs for
transitions (`started`, `stopped`) and present-tense state for no-ops
(`already_running`, `not_running`). When a third state appears, add it to the
endpoint's enum — never silently extend to a different shape.

**Client-side mapping:** the shared client package (`internal/client/`) folds
discriminator values to `nil` errors when the higher-level intent (ensure-X)
is satisfied. The discriminator value is **not** surfaced as a return value —
this keeps the MCP-server surface (Phase 2b) free of "did the no-op happen?"
state that callers would have to thread through.

### Bearer auth on every authenticated endpoint

All authenticated endpoints require `Authorization: Bearer <token>` matching
`[auth].token` in `tracelab.toml`. There is no per-route exception. The hub
returns 401 with an empty body on missing/wrong token — no error message that
could leak the expected shape of the token.

### Single JSON helpers (`writeJSON` / `decodeJSON`)

All handlers use the shared `writeJSON` (sets `Content-Type: application/json`,
encodes, logs encoder errors) and `decodeJSON` (rejects unknown fields). Any
deviation needs a comment explaining why — the convention exists so error
shapes stay byte-identical across endpoints.

---

## Phase 2b — `tracelab-mcp` MCP server

Roadmap: `~/.claude/plans/tracelab-phase-2b-mcp.md` (sub-plan of the Phase-2
roadmap). Sub-sprint cut: S1 skeleton + ARCH · S2 tool-schema-surface-cut ·
S3 sessions · S4 tail · S5 adb · S6 crashes.

The MCP server reuses `internal/client/` end-to-end — it must not re-implement
the hub HTTP/WS API. The 4 tools (sessions / tail / crashes / adb) are thin
adapters from MCP tool calls to `internal/client/` methods.

#### ADR-006: MCP library = `github.com/mark3labs/mcp-go`

**Decision:** Use `github.com/mark3labs/mcp-go` as the Go MCP-server library.

**Why:**
- Most mature MCP server implementation for Go at the time of the decision
  (2026-05-15). Active maintenance, broad community adoption, used by
  several published Go-based MCP servers.
- Full server-side surface: tool registration with JSON-Schema validation,
  resource handling, prompt registration, stdio + streamable HTTP transport.
- Idiomatic Go API (handler funcs taking `context.Context` and typed
  request/response structs) — fits the existing Tracelab style.

**Considered & rejected:**
- **`metoro-io/mcp-golang`** — smaller and less feature-complete at the time
  of the decision; in particular the streaming / long-running-tool story is
  thinner, which matters for the `tail` tool (see ADR-007 S2 placeholder).
- **Hand-rolled JSON-RPC over stdio** — MCP is JSON-RPC 2.0 plus a
  non-trivial message envelope (initialize handshake, capability
  negotiation, tool/resource lifecycle). Hand-rolling means re-implementing
  spec coverage and keeping it in sync with upstream MCP releases — that
  workload competes with Tracelab's actual goal (logs/sessions/adb), so it
  loses on opportunity cost.
- **No official Anthropic Go SDK** as of 2026-05-15 — Anthropic ships
  TypeScript and Python SDKs; Go is community-maintained territory.

**Cost:** one new top-level transitive dependency. `mcp-go` itself has a
small dep footprint (mainly `gorilla/mux` or stdlib `net/http`, both already
acceptable for the project). To be confirmed by `go mod tidy` diff during
S1 implementation; if the surface bloats noticeably the decision is
revisited.

**Open question (resolved in S2):** which MCP-go primitive expresses the
`tail` stream — a single long-running tool call, a resource subscription,
or a sequence of tool calls returning incremental chunks. Deferred to
ADR-007 in S2 (explicit Auto-Stop per plan briefing).

#### ADR-007: Tool surface (Admin-confirmed 2026-05-15)

> **Status:** Admin-confirmed 2026-05-15 (alle drei Sub-Entscheidungen
> + 6-Tools-Tabelle ohne Korrekturen durchgewinkt).

The MCP server exposes a tool set that mirrors the CLI consumption
patterns, with two pragmatic adjustments for the MCP transport: `tail`
becomes a **polling tool** rather than a streaming subscription, and
`crashes` is folded into Phase 2b via the same additive-hub-endpoint
pattern used in Phase 2a S5 (ADR-004 Option B).

**Three sub-decisions:**

##### Tool naming — `<verb>_<noun>` without `tracelab_` prefix

**Decision:** Lowercase snake_case `<verb>_<noun>`. Examples:
`sessions_list`, `tail_since`, `adb_devices`, `crashes_list`.

**Why:** Established MCP-ecosystem convention. The official MCP server
examples (filesystem, github, slack) all expose unprefixed tool names —
the server-side name handles namespacing on the consumer side, so a
`tracelab_` prefix would duplicate that. Claude Code disambiguates by
server identity (`tracelab-mcp.sessions_list`), not by tool-name
prefix, so the prefix would just make every tool call longer without
adding information.

**Considered & rejected:**
- `tracelab_<verb>_<noun>` — redundant (see above).
- `<noun>.<verb>` (dot notation, e.g. `sessions.list`) — not idiomatic
  in MCP and not supported by mcp-go's tool name validator (snake_case
  is the de-facto rule).

##### `tail` shape — polling tool with cursor (Option C)

**Decision:** `tail_since(session, since_seq?, limit?)` returns
`{ events: Event[], next_since_seq: number }`. Claude Code calls it
repeatedly with the previous `next_since_seq` to walk the event log
forward in time.

**Why:** mcp-go v0.45.0 does **not** support either of the two
streaming alternatives in a load-bearing way:

- Streaming tool content (partial-result protocol) — not in v0.45.0.
  Only `ProgressNotification` exists, which is an out-of-band progress
  signal for long-running tools, not a content-streaming channel.
- Resource subscription (`resources/subscribe` +
  `notifications/resources/updated`) — the types are defined in
  `mcp/types.go`, and the server advertises `Subscribe: true` if its
  capability flag is set, but `server/server.go` v0.45.0 has **no
  `handleSubscribe` handler** for the request. The plumbing for
  client-driven subscription is not wired through. Treating the lib
  as if it were means building on a wire protocol that the server
  cannot honour, which is exactly the kind of fragile coupling we
  avoid.

Polling with a cursor is also the better match for MCP consumers in
practice: Claude Code runs one stateless tool call per conversation
frame, so it does not need a long-lived WebSocket — it needs "give me
everything since seq=X", which is what `tail_since` answers in one
round trip.

**Considered & rejected:**
- (a) Streaming tool call — see above, no v0.45.0 mechanism.
- (b) Resource subscription — handler gap in v0.45.0, would not
  actually deliver notifications. Revisit if/when mcp-go ships the
  subscribe handler (post-v0.50.0 candidate).

##### Auth — server-start-time bearer load via `internal/cliconfig/`

**Decision:** On server start, the MCP server resolves
`tracelab.toml` through the same 5-step discovery as the CLI (ADR-002),
extracts `[auth].token`, and constructs an `internal/client.Client`
with that bearer. The token is **not** re-read per tool call. If the
token is missing or equals the literal `"CHANGEME"`, the server
refuses to start with a clear error message naming the discovery
order and the file it last looked at.

**Why:** The bearer is a deployment-time secret, not a per-call
secret — re-loading on every tool call would just burn syscalls
without buying rotation safety (the daemon restart is the rotation
trigger anyway). Mirroring the CLI's discovery path keeps the
configuration story single-sourced (ADR-002).

**Considered & rejected:**
- On-demand token load per tool call — over-engineering, no rotation
  scenario justifies it; the cost is per-call file I/O and noisier
  error surfaces.
- Token via MCP client init parameter — would force Claude Code (or
  whoever drives the MCP server) to know the bearer, defeating the
  point of `tracelab.toml` as the single config source.

##### Per-tool surface

| Tool | Input | Output (top-level shape) | Hub endpoint / Client method | Bearer | Hub-Schema-Change |
|---|---|---|---|---|---|
| `sessions_list` | `{ limit?: number, since?: string }` | `{ sessions: Session[] }` | `GET /sessions` / `client.ListSessions` (existing) | yes | **no** |
| `tail_since` | `{ session: string, since_seq?: number, limit?: number }` | `{ events: Event[], next_since_seq: number }` | `GET /events?session=…&since_seq=…&limit=…` (**new, S4**) / `client.EventsSince` (**new, S4**) | yes | **YES — Auto-Stop before S4** |
| `adb_devices` | `{}` | `{ devices: ADBDevice[] }` | `GET /adb/devices` / `client.ADBDevices` (existing) | yes | no |
| `adb_start` | `{ device_serial: string, tag_filter?: string }` | `{ status: "started" \| "already_running" }` | `POST /adb/start` / `client.ADBStart` (existing) | yes | no |
| `adb_stop` | `{ device_serial: string }` | `{ status: "stopped" \| "not_running" }` | `POST /adb/stop` / `client.ADBStop` (existing) | yes | no |
| `crashes_list` | `{ session_id: string, limit?: number }` | `{ crashes: CrashEvent[] }` | `GET /crashes?session=…&limit=…` (**new, S6**) / `client.CrashesList` (**new, S6**) | yes | **YES — Auto-Stop before S6** |

**Two Auto-Stops in Phase 2b now confirmed:**
- **S4 (`tail_since`):** Hub needs `GET /events?session=…&since_seq=…&limit=…`
  endpoint, additive on top of the existing event store. Same pattern
  as ADR-004 Option B (Phase-2a S5). Admin-confirm required before
  hub touch.
- **S6 (`crashes_list`):** Hub needs `GET /crashes?session=…&limit=…`
  endpoint. `crashes` table + UpsertCrash already exist in
  `internal/store/`; only the HTTP wrapper is missing
  (`internal/store/sqlite.go:397` documents the gap explicitly).
  Admin-confirm required before hub touch (already registered in the
  plan briefing).

**Sub-sprint impact:** the original S1-S6 cut (S1 skeleton, S2 surface,
S3 sessions, S4 tail, S5 adb, S6 crashes) holds; S4 and S6 now both
carry a small additive hub change as their first step. S3 and S5
remain pure-MCP-layer because they reuse existing client methods 1:1.

#### ADR-008: `tail_since` Hub-`/events` endpoint shape (Admin-confirmed 2026-05-15 via #021 briefing)

> **Status:** Admin-confirmed via #021 plan briefing (Hub-Schema-Change
> Auto-Stop für S4 hat grünes Licht, ADR-008 ist die konkrete
> Schema-Entscheidung dafür — additiv, keine bestehenden Endpoints
> berührt).

ADR-007 pinned `tail_since(session, since_seq?, limit?) → { events: Event[], next_since_seq: number }`
as the MCP tool shape. ADR-008 fixes the concrete hub-side schema this
tool calls into. **All decisions in this ADR are additive**: no existing
endpoint changes shape, no existing column changes, no client-side
breaking field shifts.

##### Decision 1 — `events.id` is the opaque cursor; no new `seq` column

`since_seq` (MCP-tool input) and `next_since_seq` (MCP-tool output) are
**opaque int64 cursors that map 1:1 to `events.id`** — the existing
`INTEGER PRIMARY KEY AUTOINCREMENT` column on the `events` table. No
schema migration adds a per-session `seq` column.

**Why:**
- `events.id` is already globally monotonic per AUTOINCREMENT semantics
  in SQLite (SQLite documents AUTOINCREMENT as "monotonically increasing,
  never reused" — exactly the property a forward-only cursor needs).
- Per-session-monotonic and globally-monotonic both work as opaque
  cursors when the query is `WHERE session_id = ? AND id > ?` —
  consumers only ever compare cursor values within one session's
  response stream.
- A per-session `seq` column would require either a migration backfill
  (touching every existing event row) or a write-time trigger
  (per-insert `MAX(seq)+1` SELECT under transaction). Both add cost for
  zero new capability — the opaque cursor reads identically to callers.
- `int64` SQLite ROWID range is 2^63 — practically unbounded for log
  events.

**Considered & rejected:**
- New per-session `seq INTEGER` column with migration 0003 backfill +
  ingest-time UPSERT — implementation cost (backfill ordering, transaction
  cost in `InsertEvents`) vs. zero observable benefit. The opaque-cursor
  contract is what callers consume; the underlying column identity is an
  implementation detail.
- `ts.UnixNano()` as cursor — ambiguous on identical-nanosecond inserts
  (the ingest batch can carry many events with the same `ts`),
  re-orders if a late-arriving event with an older ts gets inserted, and
  cannot be safely `WHERE ts > X` without a secondary tiebreaker on
  `id`. Not robust for a forward-only cursor.

##### Decision 2 — `GET /events?session=<id>&since_seq=<n>&limit=<n>` shape

**Endpoint:** `GET /events`, bearer-protected, registered inside the
existing 30s-timeout sub-group in `internal/http/server.go` (same group
as `/sessions`, `/ingest`, `/adb/*`).

**Query parameters:**

| Param | Required | Type | Default | Cap |
|---|---|---|---|---|
| `session` | yes | string | — | — |
| `since_seq` | no | int64 | 0 (= return from earliest) | — |
| `limit` | no | int | 500 | 5000 |

**Response (200 OK):**

```json
{
  "events": [
    { "seq_id": 42, "session_id": "...", "ts": 1700..., "source": "...",
      "level": "...", "msg": "...", "meta": {...} }
  ],
  "next_since_seq": 42
}
```

**Cursor semantics:**

- `events` contains rows with `events.id > since_seq`, ordered ascending
  by `events.id`. The relation is **strict greater-than** (not >=) — the
  caller's last-seen cursor is the lower exclusive bound, so a naive
  loop `since_seq := next_since_seq` never re-reads a row.
- `next_since_seq` is the **maximum `events.id` actually returned**, or
  `since_seq` (the caller's input) when the result is empty. The
  "stable on empty" property lets callers loop without special-casing
  empty pages — the cursor advances only when there is new data, and
  remains a valid resume point indefinitely.

**Default / cap rationale (Decision 4 below carries the full why):**
500 default trades round-trip count against MCP-payload size; 5000 cap
keeps a single response well under the mcp-go default 10 MiB stdio
frame limit even with verbose `meta` payloads.

**Error shapes:**

| Status | Cause | Body |
|---|---|---|
| 400 | `session` missing | `{"error":"session required"}` |
| 400 | unparseable `since_seq` / `limit` | `{"error":"invalid since_seq: ..."}` / similar |
| 401 | missing/wrong bearer | (empty, hub-wide convention) |
| 500 | store query failure | `{"error":"internal"}` (h.internalError) |

Unknown session ID is **not** a 404 — it returns `{ "events": [],
"next_since_seq": <input since_seq> }`. Reason: session existence is
already discoverable via `GET /sessions`; `/events` is a forward-only
cursor read, not a lookup. Returning `[]` keeps the polling loop
trivial (no branch on "session doesn't exist yet" vs "no new events").

**Considered & rejected:**
- POST with JSON body — every other read endpoint in the hub is GET +
  query string (`/sessions?limit=N`); deviating here would force the
  client package to grow a second code path. Query-string ints are
  fine for the int64 cursor (Go's `strconv.ParseInt` handles 19-digit
  literals).
- WebSocket-backed `/events` stream — duplicates the existing `/tail`
  WS surface. MCP polling does not benefit from a long-lived
  connection; ADR-007 explicitly chose polling over subscription
  because mcp-go v0.45.0 has no working resource-subscribe handler.
  A new WS endpoint here would carry the chi-`middleware.Timeout`
  incompatibility (Phase-1 S4 finding) without any consumer benefit.
- `ts`-based filter (`?since_ts=<unix-ns>`) — see Decision 1 rejection;
  not robust for forward-only cursors.

##### Decision 3 — `client.Event.SeqID int64` additive field, `omitempty`

`internal/client/types.go` `Event` struct grows one field:

```go
SeqID int64 `json:"seq_id,omitempty"`
```

**Why `omitempty`:** the field is populated only on `/events` responses
(and could be populated by `/tail` later if the WS surface ever needs
it, but that is not in scope for S4). The existing `/ingest` request
path and the existing `/tail` response path never set this field —
`omitempty` ensures the wire format on those code paths is byte-identical
to pre-S4 traffic, so no other consumer (including the Phase-2a CLI
running against an older hub during a rolling upgrade) sees any drift.

**Considered & rejected:**
- Separate `TailEventCursored` type — duplicates the Event surface for
  a single int64 field. The package already mirrors `/ingest` and
  `/tail` in one `Event`; mirroring `/events` in a parallel type would
  force MCP-side handlers to type-juggle for no semantic gain.
- Rename `Event.ID` (currently store-only) and reuse — `internal/client`'s
  `Event` has no `ID` field today; the `int64` `ID` on the store-side
  `Event` is internal. Adding `SeqID` keeps the client/store separation
  intact (client struct is a wire mirror; store struct is row-shape).

##### Decision 4 — Defaults: `limit` default 500, cap 5000; no new index in migration 0003

**Default 500 / cap 5000:** the MCP-tool consumer (Claude Code) calls
`tail_since` once per conversation frame and wants enough of the event
log to make progress without overwhelming the model context. 500 events
at ~200 bytes average payload is roughly a 100 KB JSON response — well
under stdio-transport limits, well over a single "I need to see what
happened" window. 5000 is the cap because even verbose `meta`-heavy
payloads (~2 KB each) keep one response under 10 MiB; the
mcp-go default stdio reader is configured for this envelope.

**No new index (no migration 0003):**

The query is `SELECT ... FROM events WHERE session_id = ? AND id > ? ORDER BY id ASC LIMIT ?`.
The existing `idx_events_session_ts (session_id, ts)` does **not** help
this query (wrong column order for the `id`-cursor predicate). However,
`events.id` is the SQLite ROWID (INTEGER PRIMARY KEY AUTOINCREMENT
aliases the row's internal ROWID), so:

- `events.id > ?` is a B-tree range scan on the **table itself** — no
  index lookup needed.
- The `WHERE session_id = ?` filter is applied per row during the scan.

For modest fanout (one session = one device's logcat / one test run),
the rows for a session are clustered together in id-order — the global
`id > ?` start point lands near the session's events, and the per-row
`session_id` filter discards the few interleaved rows from concurrent
sessions. EXPLAIN QUERY PLAN on a 10k-event test DB shows
`SEARCH events USING INTEGER PRIMARY KEY (rowid>?)` — the cheapest plan
SQLite has.

A composite `idx_events_session_id_id (session_id, id)` would let the
planner skip directly to a session's slice. **It is not worth adding in
S4** because:

1. The current plan scales linearly with the number of *interleaved*
   events from other sessions between the cursor and the target — not
   total event count. In tracelab's usage (one hub, low-dozen concurrent
   sessions, logcat-rate ingest), this is bounded.
2. SQLite indexes cost write-time (every `INSERT` touches the index)
   and on-disk size; for an additive-only event stream, the trade-off
   tilts negative until we see measured slowdown.
3. The migration can be added later (additive 0003) without breaking
   anything — a future "tail latency is bad on a 10M-event archive"
   finding triggers it. **Tripwire:** when the EXPLAIN cost or a
   measured P95 latency for `/events` crosses an actionable threshold,
   open ADR-009 + Migration 0003.

**Considered & rejected:**
- Add `idx_events_session_id_id` proactively in S4 — see write-cost
  argument above; premature optimisation without measured pressure.
- Make `limit` default unlimited — single payload could exceed
  stdio-transport buffers and silently truncate; opt-in caller-knob
  is safer.

##### Wire compatibility statement

S4 adds one new endpoint (`GET /events`), one new field on
`client.Event` (`SeqID`, omitempty), and one new public client method
(`EventsSince`). It changes **no existing endpoint, no existing column,
no existing client method**. A pre-S4 client running against a post-S4
hub keeps working byte-identically on `/ingest`, `/tail`, `/sessions`,
`/healthz`, `/session/*`, `/adb/*`. A post-S4 client running against a
pre-S4 hub gets a 404 on `/events` calls — captured cleanly by the
existing `*HTTPError` non-2xx surface in `internal/client/client.go`.

#### ADR-009: `crashes_list` Hub-`/crashes` endpoint shape (Admin-confirmed 2026-05-15 via #023 briefing)

> **Status:** Admin-confirmed via #023 plan briefing (zweite Hub-Schema-
> Mutation in Phase 2b, additiv analog ADR-008-Pattern, kein
> bestehendes Schema/Endpoint berührt).

ADR-007 pinned `crashes_list(session_id, limit?) → { crashes: CrashEvent[] }`
as the MCP tool shape. ADR-009 fixes the concrete hub-side schema this
tool calls into. **All decisions in this ADR are additive**: no existing
endpoint changes shape, no existing column changes, no client-side
breaking field shifts. Schema-wise, `crashes` is already complete from
P1-S5 (id, session_id, ts, fingerprint, stacktrace, count) — no
migration 0003 is required.

##### Decision 1 — Reuse `Store.CrashesBySession` with additive `limit int` parameter

`internal/store/sqlite.go` already has
`CrashesBySession(ctx, sessionID) ([]CrashRow, error)` from P1-S5, with
a doc comment explicitly stating "Used by tests and the future /crashes
API". S6 extends the signature additively to
`(ctx, sessionID, limit int) ([]CrashRow, error)` to match the
`limit`-knob exposed by the MCP tool and the HTTP endpoint.

**Default-limit semantics:** `limit <= 0` falls back to a 500 default
inside the store (same envelope as `EventsSince` from ADR-008). The
store does **not** cap; the HTTP layer caps at 5000 (Decision 2).

**Why:**
- One method, one query. A second method (e.g. `CrashesBySessionLimited`)
  would duplicate the SELECT body for a one-line difference and force
  every caller to choose between two near-identical APIs.
- The additive widening is source-compatible with the bare-minimum
  patch: every existing call site passes `0` (or `..., 0`) and gets
  identical behaviour to pre-S6 (no implicit cap, but in practice the
  default 500 fires).
- The default of 500 mirrors ADR-008 Decision 4 — operators only need
  to remember one envelope for "an MCP tool reads a session-scoped log
  table".

**Considered & rejected:**
- Separate method `CrashesBySessionLimited` — code duplication for
  zero new capability. The signature mutation is mechanical for the
  existing 7 test call sites (3 in `internal/store/sqlite_test.go`, 4
  in `internal/http/ingest_crash_test.go`), all of which are
  exercising the un-limited semantic that maps to `limit = 0`.
- Client-side `LIMIT` slicing — bandwidth and DB-time wasted on rows
  the consumer will discard. For a session with thousands of crashes
  (unusual but possible if the AUT crashes on every test loop) the
  hub-side cap is the only safety net.

##### Decision 2 — `GET /crashes?session=<id>&limit=<n>` shape

**Endpoint:** `GET /crashes`, bearer-protected, registered inside the
existing 30s-timeout sub-group in `internal/http/server.go` (same group
as `/sessions`, `/ingest`, `/events`, `/adb/*`).

**Query parameters:**

| Param | Required | Type | Default | Cap |
|---|---|---|---|---|
| `session` | yes | string | — | — |
| `limit` | no | int | 500 | 5000 |

**Response (200 OK):**

```json
{
  "crashes": [
    { "id": 17, "session_id": "...", "ts": 1700..., "fingerprint": "...",
      "stacktrace": "...", "count": 3 }
  ]
}
```

**Ordering:** newest first — `ORDER BY ts DESC, id DESC`. Mirrors the
existing store-side `CrashesBySession` order. Crashes are typically
inspected from "most recent first" (e.g. "what just broke?"), so the
hub returns rows in the same order an interactive debugger expects.
Unlike `/events`, this is **not** a forward cursor — the operation is
"list crashes for a session", a single-shot read with an optional cap.

**Error shapes:**

| Status | Cause | Body |
|---|---|---|
| 400 | `session` missing | `{"error":"session required"}` |
| 400 | unparseable `limit` | `{"error":"invalid limit: ..."}` |
| 401 | missing/wrong bearer | (empty, hub-wide convention) |
| 500 | store query failure | `{"error":"internal"}` (h.internalError) |

**Limit silent-cap:** values above 5000 are silently clamped to 5000
(same envelope as `/events`). Negative or zero `limit` is a 400 — the
operator probably typo'd; failing fast avoids silent default-fallback
confusion. Unknown session ID returns 200 with `{ "crashes": [] }`
(consistent with the rest of the read API — crash lookup is not an
existence probe).

**Considered & rejected:**
- POST with JSON body — see ADR-008 Decision 2; every read endpoint
  in the hub is GET + query string for consistency.
- Cursor pagination (analogous to `/events?since_seq=`) — crashes
  carry an `events.id`-style monotonic `id` column already, so a
  forward cursor would be technically feasible. **Rejected** because
  the consumer use case differs: events are a streaming log (callers
  poll repeatedly to follow new lines), crashes are a digest (callers
  fetch once per session to triage failures). Newest-first + `limit`
  is the matching shape; if a future use case demands cursor-walk
  through millions of dedup'd crashes per session, the additive
  `?since_id=` extension is straightforward.

##### Decision 3 — `client.CrashEvent` is a fresh DTO (greenfield)

`internal/client/types.go` grows one new type:

```go
type CrashEvent struct {
    ID          int64  `json:"id,omitempty"`
    SessionID   string `json:"session_id,omitempty"`
    TS          int64  `json:"ts"`
    Fingerprint string `json:"fingerprint"`
    Stacktrace  string `json:"stacktrace"`
    Count       int    `json:"count"`
}
```

**Why fresh DTO:** no existing endpoint in the hub serialises crashes
(P1-S5 only writes them, no read API existed). S6 is the first
exposure on the wire, so there is no pre-existing shape to be
compatible with. `omitempty` on `ID` and `SessionID` mirrors the
`Event` DTO from ADR-008 — consumers that don't care about the
server-side identity can ignore both fields, but cursor-style
extensions (Decision 2's rejected `?since_id=`) remain feasible
without a breaking change.

**Field semantics:**
- `ID` is the SQLite ROWID of the crash row. Opaque cursor placeholder
  for future pagination; today nice-to-have for clients that want to
  pin a specific crash for follow-up (e.g. fetch the related events
  around the same `ts`).
- `TS` is unix-nanoseconds (same envelope as `Event.TS`).
- `Fingerprint` is the SHA256-top-3-frames hash from P1-S5 (hex-16).
- `Stacktrace` is the raw stack as stored at first detection.
- `Count` is the dedup count — the same `(session_id, fingerprint)`
  bumps `count++` rather than inserting duplicate rows.

**Wire-compatibility statement for S6:** no existing endpoint or
client type is touched. Pre-S6 clients running against a post-S6 hub
continue to work byte-identically. Post-S6 clients running against a
pre-S6 hub get a 404 on `/crashes` calls — captured cleanly by the
existing `*HTTPError` non-2xx surface.

**Considered & rejected:**
- Reuse `store.CrashRow` at the client boundary — re-exposes a
  store-internal struct on the public client surface, breaks the
  layering rule that `internal/client` owns its own wire types
  (established in ADR-003). Same anti-pattern that ADR-008 Decision 3
  rejected for `Event`.
- Embed the full `crash.DetectResult` (language, normalized frames) —
  these are detect-time artefacts, not persisted columns. The
  persisted view is what `/crashes` should surface; richer detail
  would belong on a per-crash `/crashes/{id}` endpoint, out of scope
  for S6.

##### Decision 4 — No new index; existing `idx_crashes_session_ts` covers the query

The query is `SELECT ... FROM crashes WHERE session_id = ? ORDER BY
ts DESC, id DESC LIMIT ?`. Migration `0001_initial.up.sql` already
declares:

```sql
CREATE INDEX idx_crashes_session_ts ON crashes(session_id, ts);
```

This composite index covers:
- The equality predicate `WHERE session_id = ?` (leading column).
- The `ORDER BY ts DESC` (trailing column, SQLite walks the B-tree
  backwards).
- The `id DESC` tiebreaker, applied per row inside the indexed group.

EXPLAIN QUERY PLAN on the query reads `SEARCH crashes USING INDEX
idx_crashes_session_ts (session_id=?)` — the cheapest plan SQLite
has for a session-scoped, ts-ordered scan. **No migration 0003 is
required.**

A composite `idx_crashes_session_id_ts_id` (adding the `id`
tiebreaker) would let the planner skip the per-row id-sort inside
each `(session_id, ts)` group. **It is not worth adding in S6**
because:

1. Multiple crashes with identical nanosecond `ts` are vanishingly
   rare in practice (the dedup-upsert mechanism collapses repeats
   into one row with `count++`); the per-group tiebreaker sort never
   exceeds a handful of rows.
2. SQLite indexes cost write-time on every `INSERT`/`UPDATE` (and
   the crashes table is hit on every `UpsertCrash`); for the
   measured workload the trade-off tilts negative.
3. **Tripwire:** if `/crashes` P95 latency drifts past an actionable
   threshold on a real-world test archive, open ADR-010 +
   Migration 0003 with measured EXPLAIN and timing evidence.

Same disposition pattern as ADR-008 Decision 4 — additive,
measurable, defer-until-pressure.

**Considered & rejected:**
- Add `idx_crashes_session_id_ts_id` proactively — see write-cost
  argument above; premature optimisation.
- Drop `idx_crashes_fingerprint` since `/crashes` doesn't use it —
  out of scope; the index serves the future "find related crashes
  across sessions" use case and is cheap to keep.

##### Wire compatibility statement

S6 adds one new endpoint (`GET /crashes`), one new public client type
(`CrashEvent`), one new public client method (`CrashesList`), and
extends the store-internal `CrashesBySession` signature by one trailing
parameter (`limit int`). It changes **no existing endpoint, no
existing column, no existing client method, no existing wire format**.
A pre-S6 client running against a post-S6 hub keeps working
byte-identically on `/ingest`, `/tail`, `/sessions`, `/events`,
`/healthz`, `/session/*`, `/adb/*`. A post-S6 client running against
a pre-S6 hub gets a 404 on `/crashes` calls — captured cleanly by the
existing `*HTTPError` non-2xx surface.

## Phase 2c — Dashboard

Plan: `~/.claude/plans/tracelab-phase-2c-dashboard.md`. Scope confirmed
2026-05-16 (Admin „y" on Stack `htmx + html/template + SSE-or-WS`,
embedded in `tracelab-hub` as `/dashboard` route, 5 sub-sprints S1–S5).

#### ADR-011: Dashboard render stack and embedding (Admin-confirmed 2026-05-16 via #025 briefing)

> **Status:** Accepted (Admin-confirmed 2026-05-16, plan-briefing for
> Phase 2c sub-sprint S1). This ADR is the foundation layer for all
> subsequent dashboard work (S2 live-tail, S3 sessions browser, S4 crash
> inspector, S5 polish + agents-tab stub). No existing endpoint is
> touched; the dashboard is purely additive surface (`GET /dashboard*`).

The Tracelab hub is a Go daemon with a small HTTP API and a WS-based
live-tail. Phase 2c adds a browser-facing dashboard on top of it. ADR-011
fixes the rendering stack, the host binary, and the package layout — the
three choices that constrain every subsequent dashboard sub-sprint.

##### Decision 1 — Render stack = `htmx` + `html/template`

Server-rendered HTML via the stdlib `html/template` package, with
`htmx` (1.9.x line, local-vendored — see Decision 3) driving partial
swaps for tab navigation and the live-tail stream. No client-side JS
framework. No build step beyond `go build`.

**Why:**
- The dashboard is a server-driven inspection UI, not an offline-capable
  SPA. `htmx` handles the interactive bits (tab swaps, live-stream
  append, form submits) with HTML attributes — fits a server-rendering
  model natively.
- `html/template` is stdlib: zero new dependency, context-aware escape,
  composable via `{{template "name" .}}`. Parsed once at handler init
  (`template.ParseFS` over the embedded `go:embed` FS), shared across
  requests.
- The repo is CGO-free and ships as a single binary. Adding a Node-
  based bundler (esbuild, webpack, vite) would fork the build pipeline,
  break the "one `go build` → one binary" story, and force every
  contributor to install a JS toolchain — an immediate violation of the
  Phase-1 stack-marker.

**Considered & rejected:**
- `a-h/templ` (type-safe templates with Go generics) — Considered for
  the compile-time field checks and the IDE story. Rejected because it
  adds a code-generation step (`templ generate`) before every `go
  build`, splitting the build pipeline. For an MVP dashboard with a
  handful of templates, `html/template` reaches feature parity at
  zero generator-tooling cost.
- Vue / Svelte / React SPA with REST/JSON backend — Considered for the
  richer interactivity story (charts, virtual scrolling). Rejected on
  three counts: (i) introduces Node toolchain into a CGO-free Go repo,
  (ii) forces a doubled build pipeline (one for Go binary, one for
  static assets), (iii) Cross-Compile story degrades — Windows
  contributors would need both Go and Node working. The MVP scope
  (live-tail, session table, crash list) does not require an SPA.

##### Decision 2 — Host = embedded in `tracelab-hub` as `/dashboard` route

The dashboard runs inside the existing `tracelab-hub` binary, registered
as a sub-router on the chi router built by `internal/http.New`. No
separate `tracelab-dashboard` binary, no second daemon process.

**Why:**
- The dashboard reads the same SQLite store the hub already owns. A
  second daemon would need either its own DB connection (multi-writer
  contention) or an HTTP read-through to the hub (extra hop, extra
  latency, doubled auth surface).
- Live-tail is a direct `ws.Hub` subscriber today (`internal/ws.Hub`).
  Embedding the dashboard inside the hub keeps that fan-out in-process;
  a separate dashboard daemon would have to re-subscribe over WS,
  buying nothing but extra moving parts.
- Operationally, one daemon means one config file (`tracelab.toml`),
  one auth token, one systemd unit, one Windows-service entry. The
  Phase-1 single-binary lifecycle stays intact.

**Considered & rejected:**
- Separate `cmd/dashboard` binary speaking HTTP to the hub — Considered
  for the cleaner separation-of-concerns (read-only consumer). Rejected
  because it doubles the daemon-lifecycle surface (two `signal.NotifyContext`
  shutdown sequences, two `/healthz` endpoints to monitor), adds an
  internal HTTP hop for every dashboard page render, and would force a
  second instance of the bearer-token discovery (config file? env var?
  pipe from the hub?). For a single-user dev tool with a single SQLite
  backing store, the separation is not paying for itself.

##### Decision 3 — Asset embedding via `//go:embed`

Templates and static assets (htmx.min.js, dashboard.css) ship inside the
`tracelab-hub` binary via `//go:embed`. The `web/` top-level package
exposes the embed.FS objects; `internal/dashboard` parses them at init
time and serves them from in-memory.

**Why:**
- The single-binary distribution model that Phase 1 established
  (`tracelab-hub` on Linux, `tracelab-hub.exe` on Windows, both
  produced by `go build` without any external resource directory) must
  survive Phase 2c. `//go:embed` is the stdlib mechanism for exactly
  this case.
- htmx is distributed as a single ~14kB minified JS file. Vendoring it
  (i.e. committing it under `web/static/`) means contributors get a
  reproducible build offline, no CDN fetch at runtime, no CSP
  exemption for third-party origins, no privacy leak to a CDN provider.
- `template.ParseFS(embedFS, "templates/*.gohtml")` is the documented
  stdlib pattern — no third-party wrapper needed.

**Considered & rejected:**
- Loose-file serving from a `--web-root` flag — Considered for the
  "edit a template, reload the page, no rebuild" dev workflow.
  Rejected because it forks the distribution into "binary + files
  directory", breaks single-binary install, and creates two failure
  modes ("did the binary find the templates? did the files directory
  ship?"). For dev-time iteration the standard Go workflow (`go run
  ./cmd/hub`, edit, re-run) is adequate given how small the templates
  are.
- CDN-hosted htmx — Considered for the smaller binary size (~14 kB
  saved). Rejected for offline-build reproducibility, privacy
  (every dashboard render would beacon to a third-party CDN), and
  Content-Security-Policy hygiene — the dashboard should not require a
  `script-src` exemption for a third-party origin.

##### Decision 4 — Package layout: new top-level `web/`, handlers in `internal/dashboard/`

A new top-level `web/` package owns the templates and static assets, and
declares the `//go:embed` filesystems. A new internal package
`internal/dashboard/` owns the HTTP handlers (layout render, tab render,
static serve). The route registration sits inside the existing
`internal/http.New` constructor, in a new sub-group analogous to the
existing bearer-30s group.

```
tracelab/
├── web/                          ← new top-level (assets only, no Go logic)
│   ├── embed.go                  ← //go:embed declarations + exported FS handles
│   ├── templates/
│   │   ├── base.gohtml           ← layout: <head>, header, tab-nav, content slot, footer
│   │   ├── tab_live_tail.gohtml  ← S1 placeholder (S2 fills in)
│   │   ├── tab_sessions.gohtml   ← S1 placeholder (S3 fills in)
│   │   ├── tab_crashes.gohtml    ← S1 placeholder (S4 fills in)
│   │   └── tab_agents.gohtml     ← S1 placeholder, "Phase 2d coming soon"
│   └── static/
│       ├── htmx.min.js           ← vendored, version pinned in file comment
│       └── dashboard.css         ← minimal layout/typography
├── internal/dashboard/           ← new internal package (Go logic only)
│   ├── handler.go                ← Handler type, LayoutHandler, TabHandler, StaticHandler
│   └── handler_test.go
└── internal/http/server.go       ← adds `/dashboard*` sub-router wiring
```

**Why this split:**
- `web/` as a top-level package is the conventional Go location for
  embedded UI assets (`grafana/grafana`, `pocketbase/pocketbase`,
  `kubernetes/dashboard` all follow this pattern). It signals "this
  directory is not Go business logic, it's the UI artefact tree" — the
  embed.FS objects exported from `web/embed.go` are the public surface.
- `internal/dashboard/` as the handler package keeps the HTTP-layer
  Go code on the same side of the `internal/` boundary as the rest of
  the daemon's request-handling logic. Handlers depend on the `web`
  package for the embedded FS, never the other way round.
- Sub-router registration in `internal/http/server.go` mirrors the
  existing bearer-30s group and the `/adb/*` registration pattern from
  ADR-004 Option B — the operator's mental model ("`/dashboard*` is
  registered in the same constructor as everything else") stays
  uniform.

**Considered & rejected:**
- Templates and handlers in one package (`internal/dashboard/templates/*`
  inline-embedded) — Rejected because it conflates the "asset tree"
  and the "Go logic tree" inside a single internal package. The
  current split makes "which files do designers/UI contributors
  touch?" obvious: `web/`. Go contributors touch `internal/dashboard/`.
- Templates under `internal/dashboard/templates/` (no top-level `web/`)
  — Rejected because the static assets (htmx, CSS) are not "internal
  to the dashboard package" semantically; they're the binary's user-
  facing artefact tree. Top-level `web/` is the idiomatic home.

##### Consequences

- **Single build pipeline preserved.** `go build ./cmd/hub` still
  produces a self-contained binary. No Node toolchain. CGO stays off
  (the `web/` package is pure-Go: `//go:embed` declarations only, no C
  bindings). Cross-Compile via `make hub-windows` keeps working
  byte-identically.
- **One new top-level directory `web/`.** First non-`cmd`/non-`internal`
  Go-level addition since Phase 1. Documented in this ADR and in
  `README.md`'s component list (M-followup if not in S1 trivial-touch).
- **htmx version is pinned at vendor time.** A bump means a new commit
  touching `web/static/htmx.min.js` plus a verification smoke. No
  silent upgrades. Version comment in the file documents the source URL
  and the SHA256.
- **Auth posture: permanently Loopback-only (Admin-Confirm 2026-05-16).**
  ADR-011 originally deferred the dashboard auth model to a follow-up
  ADR. Admin confirmed via AskUserQuestion-block on 2026-05-16
  (#026 plan-briefing) that the dashboard sub-router stays
  **permanently Loopback-only** — no cookie-wrap, no reverse-proxy,
  no short-lived session-token layer. Operational assumption: the
  hub binds to `127.0.0.1:<port>` (single-user dev host), and the
  dashboard inherits that binding. Browsers on the same machine
  reach `/dashboard` without an `Authorization` header; remote
  clients are physically unreachable. Should a future deployment
  scenario need remote dashboard access (multi-user, reverse-proxy,
  TLS terminator), that is a **separate ADR** (e.g. an ADR-XYZ
  Cookie-Wrap), explicitly *not* a follow-up to ADR-011 — the
  Loopback-only decision is a deliberate posture, not a TODO.

#### ADR-012: Dashboard live-tail mechanism — SSE on `/dashboard/stream` (Accepted 2026-05-16)

> **Status:** Accepted (Admin-Confirm 2026-05-16 via AskUserQuestion-block,
> Chakotay; Lead-Empfehlung Ballard P2c-S1 übernommen). S2 implements
> the decision in `internal/dashboard/stream.go` + `web/templates/tab_live_tail.gohtml`.

S1 (this sub-sprint) lays the dashboard foundation but does **not**
implement live-tail. S2 picks one of the two options below and wires it
into `tab_live_tail.gohtml`. This ADR exists to make the trade-off
explicit and to capture Ballard's lead recommendation for Admin's
decision.

##### Context

The hub already broadcasts live events through `internal/ws.Hub`:
`/ingest` and the adb bridge publish, `/tail` is a `gorilla/websocket`
endpoint that fan-outs to subscribers with optional `?session=<id>`
filter. The dashboard's live-tail tab needs to surface the same
stream inside the browser, with append-on-arrival rendering and a
session-filter dropdown (Phase 2c S2 DoD).

##### Options

**Option A — Server-Sent Events on a new `/dashboard/stream` endpoint.**
A new HTTP handler under `/dashboard/stream?session=<id>` opens an SSE
connection. Internally, the handler subscribes to the same `ws.Hub` the
existing `/tail` uses and pipes events as `data: <json>\n\n` frames. htmx
has built-in SSE support (`hx-ext="sse"`, `sse-connect`, `sse-swap`) —
the template binds the event stream to an append-target with one
attribute.

**Option B — WebSocket reuse via the existing `/tail` endpoint.**
The dashboard JS opens a `WebSocket("ws://…/tail?session=<id>")` directly
from the browser. A small (~30 line) inline script appends each incoming
message to the live-tail container. No new server endpoint; the existing
`/tail` Hub-subscriber infrastructure is reused as-is.

##### Trade-off table

| Criterion | Option A — SSE on new endpoint | Option B — WS reuse of `/tail` |
|---|---|---|
| **Bearer auth from browser** | Trivial: SSE is a normal HTTP GET, the cookie/header model from ADR-011-followup applies directly. `EventSource` API can carry cookies. | Hard: browsers cannot set `Authorization:` headers on `WebSocket(…)` constructors. Requires either bearer-in-query-string (`/tail?token=…`, currently rejected by the hub per README) or a cookie-based auth wrap. Forces an auth-model change before S2 can ship. |
| **Reconnect behaviour** | Browser-built-in: `EventSource` auto-reconnects with backoff. Server can emit `id:` and clients resume via `Last-Event-ID`. | Manual: dashboard JS must implement `onclose`-driven reconnect loop. ~20 extra lines of inline JS, easy to get wrong (exponential backoff, jitter). |
| **Code reuse from existing hub** | Medium: needs a new handler that bridges `ws.Hub` subscriber → `http.ResponseWriter` flush loop. Maybe 60–80 lines of new Go, but conceptually a thin shim. | High: zero new server code — reuses `ws.Handler(cfg.Hub, logger)` as-is. The Hub already does slow-subscriber drop, heartbeat ping/pong, graceful close on shutdown. |
| **Browser support** | Universal (EventSource ≥ IE10-equivalent). htmx `hx-ext="sse"` extension is standard. | Universal (WebSocket support has been universal since ≥ 2013). |
| **Stream direction** | One-way server→client (matches live-tail use case exactly). | Bidirectional (which we don't need; the browser never sends back). |
| **Backpressure / slow subscriber** | Must be implemented in the new bridge handler: when the SSE writer's flush blocks, drop or close. Mirrors the existing `ws.Hub` slow-subscriber-drop. | Already implemented in `ws.Hub` (per Phase-1 ADR-implicit decision): slow subscribers get their channel closed; the publish path never stalls. Free for the dashboard. |
| **Protocol overhead** | `text/event-stream` + JSON payloads. Slightly more bytes than raw WS frames (the `data:` prefix and double-newline framing). Negligible at the volumes Tracelab handles. | Binary or text WS frames. Marginally more efficient. |
| **Server-side cost** | One additional handler + one additional Hub-subscriber goroutine per dashboard tab. Same lifecycle pattern as `/tail`. | Zero additional handler. One additional Hub-subscriber goroutine per dashboard tab (same as Option A — both subscribe to the Hub). |
| **htmx integration** | First-class: `<div hx-ext="sse" sse-connect="/dashboard/stream?session=…" sse-swap="message" hx-swap="beforeend">`. ~3 attributes, zero JS. | Requires custom inline JS to bridge `WebSocket.onmessage` → DOM append. Not htmx-idiomatic. |
| **Operational debuggability** | `curl -N http://…/dashboard/stream?session=…` works for smoke. Browser DevTools "Network → EventStream" tab shows frames. | `websocat ws://…/tail?session=…` works for smoke. Browser DevTools "Network → WS" tab shows frames. |

##### Ballard's recommendation: **Option A (SSE on new `/dashboard/stream`)**

**Why:**
1. The auth story collapses. SSE rides on plain HTTP, which means the
   forthcoming dashboard auth model (bearer-via-cookie or short-lived
   session-token cookie) covers `/dashboard/stream` with the same
   middleware that covers `/dashboard` itself. Option B forces a
   second auth path (query-string token, against the README's explicit
   "no token in query string" rule, or a cookie-wrap layer that has to
   also cover the existing `/tail` for backwards-compat).
2. The dashboard JS shrinks to zero. Option A is `<div hx-ext="sse"
   sse-connect=…>`, three htmx attributes. Option B is ~30 lines of
   inline reconnect/append logic — bigger surface to maintain, easier
   to regress.
3. Live-tail is intrinsically one-way (server → browser). SSE is the
   stdlib match; WebSocket's bidirectional capability is overhead we
   don't use.
4. Reconnect-on-network-blip is free with SSE (the browser does it).
   The existing `/tail` endpoint is consumed today by `tracelab tail`
   (CLI) and by future MCP tools — adding a reconnect-loop JS
   subscriber to it changes the consumer expectations subtly. SSE on a
   new endpoint keeps `/tail` as the unchanged "raw fan-out" surface.
5. The new-handler cost is 60–80 lines of Go that mirror the `ws.Hub`
   subscriber pattern already proven in `internal/ws/handler.go`. Not
   free, but bounded.

**Counter-weight:** Option B is genuinely lighter on server code in S2
itself (zero new lines). The trade is "save 60 lines of Go in S2 vs.
keep both auth paths and inline-JS-reconnect forever." The recommendation
takes the long-term-maintenance trade.

##### Decision

**Option A — Server-Sent Events on a new `/dashboard/stream` endpoint.**

Admin-confirmed 2026-05-16 via AskUserQuestion-block (Chakotay),
Lead-Empfehlung Ballard P2c-S1 übernommen ohne Korrektur.

##### Consequences

- **New endpoint `GET /dashboard/stream?session=<id>`** registered as a
  sub-router entry under `/dashboard*` in `internal/http/server.go` —
  outside the bearer-group (consistent with the rest of `/dashboard*`,
  see ADR-011 *Consequences*: Loopback-only auth posture). Handler
  lives in `internal/dashboard/stream.go` (new file, separate from
  `handler.go` to keep the streaming lifecycle code isolated from the
  request/response layout handlers).
- **Subscriber-Bridge to `ws.Hub`** uses the existing
  `Hub.Subscribe(sessionFilter) (<-chan Event, func())` API
  **as-is** — no additive widening required. The S1-era audit
  ("`ws.Hub.Subscribe` SSE-bridge tauglich?") settled in S2 as: yes,
  trivially. The Subscribe API returns a typed event channel plus
  an idempotent cancel; the SSE handler ranges over the channel,
  encodes `data: <json>\n\n`, flushes after every event, and calls
  the cancel func on `r.Context().Done()` for clean teardown.
- **Slow-subscriber drop is inherited** from `ws.Hub.Publish` (the
  non-blocking send loop with `default: drop`). The SSE handler does
  not need to re-implement drop semantics in the bridge layer — if the
  Hub drops an event for this subscriber's channel, the SSE writer
  never sees it. The browser thus experiences "occasionally missing
  events under sustained backpressure", exactly the same observability
  contract as `/tail` WS consumers. This matches the Plan-Briefing's
  default-decision ("drop-events analog ws.Hub") without an
  additional Auto-Stop.
- **Heartbeat ticker = 15s**, writing the SSE comment line
  `: heartbeat\n\n` (a comment frame, ignored by the EventSource
  consumer but keeps proxies and `net.Conn` read deadlines from
  closing an idle stream). 15s is well under the conventional 30s
  proxy idle-timeout floor and the browser's own 90s
  `connectionTimeout` for `EventSource`; configurable via a
  package-level `HeartbeatInterval` var (tests shrink it).
- **`/tail` WS endpoint is unchanged.** CLI consumers (`tracelab tail`)
  and the MCP server keep talking to `/tail` byte-identically. SSE on
  `/dashboard/stream` is purely additive surface — the existing
  Hub-subscriber count grows by one per open dashboard tab, same
  cost envelope as a CLI tail consumer.
- **No new dependency.** SSE is plain HTTP — `http.Flusher`
  type-asserted from the `http.ResponseWriter`, `text/event-stream`
  content-type, and `data:`-prefixed frames. The stdlib `net/http`
  layer handles everything. The htmx SSE extension (`htmx-ext-sse`)
  ships as a separate vendored `web/static/htmx-ext-sse.js` file
  (htmx v2.0.4 does **not** include the SSE extension in the core
  bundle — confirmed via release notes and source inspection), SHA-pinned
  in `web/static/htmx-ext-sse.version.txt` like the htmx core itself.

##### Considered & rejected (regardless of A/B)

- **Polling `/events?since_seq=` from the dashboard.** Considered for
  the "no streaming infrastructure at all" simplicity. Rejected
  because the polling cadence has to be tight (≤ 500 ms) for live-tail
  to feel live; that's worse load on the hub than one persistent
  subscriber, and the user-experience still lags by half the poll
  interval. The hub already broadcasts in real time via `ws.Hub` —
  refusing to consume that for dashboard tail would leave performance
  on the table.
- **gRPC-Web streaming.** Considered for completeness. Rejected
  because the repo has no gRPC surface today; introducing it for one
  dashboard endpoint is a stack expansion with no other consumer.

---

## Phase 2d — Agent-Ingest Layer

Plan: `~/.claude/plans/tracelab-phase-2d-agents.md`. Scope confirmed
2026-05-17 (Admin „y" on Multi-Ingest pattern: SDK-Hooks + Transcript-Tail
+ MCP-Push, 6 sub-sprints S0+S1–S5). S0 (this section) is a pure-doc
sprint: pipeline shape, schema, endpoint surface, and ADR-013. No
handler code, no MCP tool, no test. S1 implements the first ingest
source (SDK-Hooks) once the schema is admin-approved.

Phase 2d adds a second data domain to the hub: the **AI-agent
observability domain** (skill spawns, token usage, verdicts, mailbox
edges between agents), alongside the existing **app-log domain**
(sessions/events/crashes/screenshots from Phase 1). The two domains
coexist in the same SQLite store, share the same daemon lifecycle, and
surface side-by-side in the dashboard (Phase 2c added the agents-tab
stub in S5 — Phase 2d S4 fills it).

### Architecture — three ingest sources, one persistence shape

```
                            ┌────────────────────────────────────────┐
                            │           tracelab-hub (Daemon)        │
                            │                                        │
   Claude Code Worker ─────►│  SDK-Hooks (push)                      │
   PostToolUse/Stop hook    │    └─► POST /agents/ingest             │
   HTTP-POST                │        source=sdk-hook                 │
                            │        ─►  store.agent_*               │
                            │                                        │
   ~/.claude/projects/      │  Transcript-Tail (pull)                │
   */*.jsonl ──────────────►│    └─► hub-internal tail-goroutine     │
   (file mtime watch)       │        (analog adb logcat-bridge)      │
                            │        ─►  POST /agents/ingest         │
                            │            source=transcript           │
                            │        ─►  store.agent_*               │
                            │                                        │
   tracelab-mcp ───────────►│  MCP-Push (active)                     │
   agent_event tool         │    └─► POST /agents/ingest             │
   from-worker call         │        source=mcp-push                 │
                            │        ─►  store.agent_*               │
                            │                                        │
                            │                  ▼                     │
                            │  Persistence (4 tables, dedup-on-      │
                            │  UNIQUE-tuple per source)              │
                            │    agent_spawns                        │
                            │    agent_tokens          ──┐           │
                            │    agent_verdicts          ├─► Hub.    │
                            │    agent_mailbox_edges   ──┘  Publish  │
                            │                                ─► WS   │
                            │                                ─► SSE  │
                            │                                        │
   Browser ─HTTP───────────►│  /dashboard (Phase 2c) + Agents tab    │
                            │   ↑ reads agent_* via Phase-2d         │
                            │     read endpoints                     │
                            └────────────────────────────────────────┘
                                          │
                                          ▼
                            SQLite (NTFS-shared, same store as
                            sessions/events/crashes/screenshots —
                            no second DB, no second daemon)
```

Three ingest modes coexist as concurrent writers to the same four-table
schema; idempotency keeps duplicates from compounding (see §Schema and
ADR-013 below).

| Source | Mode | Latency | Robustness | Trigger / Carrier |
|---|---|---|---|---|
| **SDK-Hooks** | push | low (sub-second) | depends on hook configuration | Claude Code worker's `PostToolUse` / `Stop` hooks shell out to `curl` or a tiny CLI helper that POSTs to `/agents/ingest` |
| **Transcript-Tail** | pull | mid (1–5 s tail cycle) | robust against hook gaps — works even on workers without hook config | hub-internal goroutine tails `~/.claude/projects/*/*.jsonl` analogous to the existing adb logcat bridge (`internal/adb/`), parses spawn / token / verdict events, POSTs internally |
| **MCP-Push** | active | low (sub-second) | explicit, workflow-instrumented | `agent_event` tool in `tracelab-mcp` (Phase 2d S3) called by the worker itself from inside the conversation — atomic with the action it reports |

### Tripwire pattern — primary-vs-fallback by event type

Each of the three sources has a domain where it is the **primary** writer
(low latency or atomically authoritative) and another where it acts as a
**fallback** that fills the gap when the primary is silent. The hub does
not reject duplicates — the UNIQUE-tuple constraints in §Schema collapse
them to one row, with a per-source breakdown preserved in
`agent_tokens.source` for forensic comparison.

| Event class | Primary source | Why primary | Fallback(s) | Fallback trigger |
|---|---|---|---|---|
| **Spawn-begin / spawn-end** (lifecycle edges) | **MCP-Push** | The worker itself emits the event the instant the spawn begins, atomically with the action — no observer-loss window. | Transcript-Tail (catches the spawn from the `.jsonl` even if the worker never called the tool); SDK-Hooks (catches via `PostToolUse` for tool-invoking spawns) | MCP-Push heartbeat silent > 60 s for a known-active project |
| **Token-usage deltas** | **SDK-Hooks** | The `Stop` hook fires with the final usage block in scope; one hook = one full token event, no parsing of streamed deltas. | Transcript-Tail (reads the `.jsonl` `usage` blocks post-hoc) | SDK-Hook heartbeat silent > 60 s OR per-project hook config absent (detected by Transcript-Tail seeing usage rows that have no matching SDK-Hook row within 60 s) |
| **Verdict + Lerneffekt** (QS-output) | **MCP-Push** | The QS-skill (tuvok / icheb / carey) emits the verdict via `agent_event` as part of its own workflow — atomic with the QS-skill's other actions and naturally typed. | Transcript-Tail (last-resort extraction from the QS-skill's `.jsonl` message body via a regex on the verdict marker) | MCP-Push call for a verdict was never made within the QS-skill's spawn-end window |
| **Mailbox-edges** (spawn / return / escalate / delegate) | **MCP-Push** | Cross-agent edges are created at the moment of mailbox-routing — the routing skill (chakotay / lead) emits the edge atomically. | Transcript-Tail (infers edges from `.jsonl` cross-references between worker sessions); SDK-Hooks (infers from `Stop` events that follow a spawn-call) | MCP-Push call absent within the spawning conversation's frame |

**Fallback-switch criterion:** the hub tracks a per-source last-write
timestamp per project. When the primary source's last-write is older
than 60 s for an actively-running spawn (detected by `agent_spawns`
rows with `ended_at IS NULL`), the hub logs a `WARN` and the fallback
source's row becomes the surviving record under the UNIQUE-tuple.
ADR-013 §Consequences spells out the operational contract.

**Pattern inheritance:** this is the same tripwire model used in Phase
2b for the `/crashes` index-design decision (ADR-009 §Tripwire) and in
Phase 1 for the slow-subscriber-drop in `ws.Hub.Publish` — a measurable
condition that, when crossed, opens an explicit ADR or escalates to a
fallback path, rather than silently degrading. Phase 2d's tripwire
extends the pattern from a single-source-degradation alarm to a
multi-source-redundancy switch.

### Schema (migration `0003_agents_schema.up.sql`)

The schema lives in `migrations/0003_agents_schema.up.sql` (top-level
`migrations/` directory, deliberately **outside** the
`internal/store/migrations/` embed-FS so that S0 ships the schema as a
review artefact without auto-applying it on the next `Open` —
Auto-Stop-pflicht for admin-approval; S1 moves the file into
`internal/store/migrations/` and adjusts the migration-count tests in
the same commit as the first ingest handler).
Four tables, all foreign-keyed back to `agent_spawns.id` (the global
identifier). `agent_spawns.session_ref` is a nullable FK into
`sessions.id` (Phase 1) — spawns triggered outside an app-session
context (e.g. a standalone QS-skill invocation) have a NULL ref and
that is correct.

**`agent_spawns`** — one row per skill / subagent invocation. The
spawn lifecycle row.

| Column | Type | Notes |
|---|---|---|
| `id` | TEXT PRIMARY KEY | ULID-shaped 26-char string (consistent with `sessions.id`; the Phase-1 `newSessionID()` helper in `internal/store/sqlite.go` already produces this shape with no external dep). Globally unique across all sources — the writer supplies it. |
| `parent_id` | TEXT NULL | Self-FK to `agent_spawns(id)` `ON DELETE SET NULL`. NULL for top-level spawns (Admin / Chakotay-routing-root). |
| `skill` | TEXT NOT NULL | Skill slug (`ballard`, `tuvok`, `icheb`, …). |
| `started_at` | INTEGER NOT NULL | Unix-ms timestamp. |
| `ended_at` | INTEGER NULL | Unix-ms. NULL while the spawn is in-flight; populated on `Stop` event. |
| `project` | TEXT NOT NULL | Project slug (`tracelab`, `nexus`, …) — derived from the cwd-detection convention or the worker's project frontmatter. |
| `session_ref` | TEXT NULL | FK into `sessions.id` `ON DELETE SET NULL` — nullable for spawns without app-session context. |

**Indexes:** `(parent_id)`, `(project)`, `(session_ref)`, `(started_at DESC)`
(the last one drives the "recent spawns" query for the dashboard list view).
Primary key on `id` is sufficient for idempotency — ULID is globally unique
across all sources, so no extra UNIQUE-tuple needed on this table.

**`agent_tokens`** — append-only token-usage events per spawn.

| Column | Type | Notes |
|---|---|---|
| `id` | INTEGER PRIMARY KEY AUTOINCREMENT | Surrogate. |
| `spawn_id` | TEXT NOT NULL | FK into `agent_spawns(id)` `ON DELETE CASCADE`. |
| `input_tokens` | INTEGER NOT NULL DEFAULT 0 | |
| `output_tokens` | INTEGER NOT NULL DEFAULT 0 | |
| `cache_read` | INTEGER NOT NULL DEFAULT 0 | |
| `cache_write` | INTEGER NOT NULL DEFAULT 0 | |
| `ts` | INTEGER NOT NULL | Unix-ms timestamp of the token-usage event. |
| `source` | TEXT NOT NULL | `CHECK(source IN ('sdk-hook','transcript','mcp-push'))` — discriminator preserved per row for forensic comparison when two sources disagree. |

**UNIQUE constraint:** `(spawn_id, ts, source)` — the same token event
from the same source kollidiert (idempotency under hook retry); the
same token event from a *different* source is allowed as a separate row,
keeping per-source forensics intact (see ADR-013 §Consequences).

**Indexes:** `(spawn_id, ts, source)` UNIQUE (above), and a covering
`(spawn_id)` for token-aggregation queries.

**`agent_verdicts`** — QS verdicts emitted by tuvok / icheb / carey.

| Column | Type | Notes |
|---|---|---|
| `id` | INTEGER PRIMARY KEY AUTOINCREMENT | Surrogate. |
| `spawn_id` | TEXT NOT NULL | FK into `agent_spawns(id)` `ON DELETE CASCADE` — refers to the *spawn that was reviewed*, not the QS-skill's own spawn. |
| `verdict` | TEXT NOT NULL | `CHECK(verdict IN ('freigabe','auflagen','rueckgabe','eskalation','none'))`. |
| `lerneffekt_md` | TEXT NULL | Markdown text of the Lerneffekt note, nullable for verdicts without an explicit lesson. |
| `ts` | INTEGER NOT NULL | Unix-ms. |

**UNIQUE constraint:** `(spawn_id, verdict, ts)` — same verdict from two
sources kollidiert on the same ts. Different verdicts at different ts
for the same spawn (e.g. initial `auflagen` followed by `freigabe` after
fix) are kept as separate rows — the QS history is intact.

**Index:** `(spawn_id)` for verdict-lookup-by-spawn.

**`agent_mailbox_edges`** — directed cross-agent edges (who routed to whom).

| Column | Type | Notes |
|---|---|---|
| `id` | INTEGER PRIMARY KEY AUTOINCREMENT | Surrogate. |
| `from_spawn_id` | TEXT NOT NULL | FK into `agent_spawns(id)` `ON DELETE CASCADE`. |
| `to_spawn_id` | TEXT NOT NULL | FK into `agent_spawns(id)` `ON DELETE CASCADE`. |
| `edge_type` | TEXT NOT NULL | `CHECK(edge_type IN ('spawn','return','escalate','delegate'))`. |
| `ts` | INTEGER NOT NULL | Unix-ms. |

**UNIQUE constraint:** `(from_spawn_id, to_spawn_id, edge_type, ts)` —
same edge from two sources kollidiert.

**Indexes:** the UNIQUE-tuple plus `(from_spawn_id)` and `(to_spawn_id)`
for tree-walk queries in both directions (recursive CTE for `/agents/tree/{id}`).

### Endpoint surface

All endpoints sit under the standard hub authentication regime (Bearer
on every authenticated route — see API Conventions). The ingest endpoint
is shared by all three sources; reads are per-domain.

| Method | Path | Auth | Purpose |
|---|---|---|---|
| POST | `/agents/ingest` | yes | Unified ingest from all three sources, discriminated via `source` field in the JSON payload (`sdk-hook` / `transcript` / `mcp-push`). Idempotent: re-POSTing the same `(spawn_id, ts, source)` tuple returns 200 OK no-op (consistent with the Phase-1 `/ingest` 202-on-already-there pattern and the Phase-2a `/adb/*` 200-OK-with-discriminator pattern — see API Conventions §Idempotent state-mutating endpoints). 201 only on first-write. |
| GET | `/agents/sessions` | yes | List of agent spawns with cursor-based pagination — `?limit=N&cursor=<opaque>` (consistent with `/sessions` and `/events` pagination shape). Returns `{spawns: [...], next_cursor: "..."}`. |
| GET | `/agents/tree/{spawn_id}` | yes | Subgraph for the given spawn: parent-chain up to the root plus all descendants, via SQLite recursive CTE on `agent_spawns(parent_id)`. Returns `{root: {...}, nodes: [...], edges_from_mailbox: [...]}`. |
| GET | `/agents/tokens` | yes | Token-usage aggregation. Filter via `?session=<ref>` and/or `?spawn_id=<id>`. Returns sum across all sources plus per-source breakdown (`{total: {...}, by_source: {sdk-hook: {...}, transcript: {...}, mcp-push: {...}}}`) so the dashboard can show "they agree" or "they disagree by N tokens" at a glance. |
| GET | `/agents/verdicts` | yes | Verdict list with filter on `?status=<verdict>` and/or `?project=<slug>` and/or `?session=<ref>`. Returns `{verdicts: [{spawn_id, verdict, lerneffekt_md, ts, skill}], next_cursor: "..."}`. |

**`POST /agents/ingest` payload shape (wire-compat across all 3 sources):**

```json
{
  "spawn_id":   "01HXXX...",          // ULID, required, idempotency key
  "parent_id":  "01HYYY...",          // optional, for spawn lifecycle events
  "skill":      "ballard",            // required
  "project":    "tracelab",           // required
  "session_ref": "01HSESS...",        // optional (FK to sessions, nullable)
  "started_at": 1715900000123,        // unix-ms; required on first spawn-begin
  "ended_at":   1715900099456,        // unix-ms; required on spawn-end
  "tokens": {                          // optional; usage event
    "input_tokens":  12345,
    "output_tokens": 678,
    "cache_read":    0,
    "cache_write":   0,
    "ts":            1715900050000
  },
  "verdict": {                         // optional; QS-output event
    "verdict":       "freigabe",
    "lerneffekt_md": "...",
    "ts":            1715900070000
  },
  "mailbox_edges": [                   // optional; cross-agent routing events
    { "from_spawn_id": "01HZZZ...", "to_spawn_id": "01HAAA...",
      "edge_type": "spawn", "ts": 1715900001000 }
  ],
  "source":     "sdk-hook"             // required: discriminator
}
```

**Source-specific payload shape (which fields each source typically fills):**

| Source | Spawn-lifecycle | Tokens | Verdict | Mailbox-edges |
|---|---|---|---|---|
| **SDK-Hooks** | partial (`Stop` carries spawn-end, `PostToolUse` doesn't carry spawn-begin natively) | yes (`Stop` hook has full usage in scope) | rarely (hooks don't introspect QS-output) | inferred (`Stop`-after-spawn-call) |
| **Transcript-Tail** | partial (catches lifecycle from `.jsonl` post-hoc, with 1–5 s tail-cycle latency) | yes (parses `usage` blocks in `.jsonl`) | last-resort regex (verdict-marker pattern) | inferred (cross-references in `.jsonl`) |
| **MCP-Push** | full (worker emits both begin and end atomically) | yes (worker reports its own usage explicitly) | yes (QS-skill's `agent_event` call is naturally typed) | full (routing-skill emits edges directly) |

Fields not relevant to a given source are simply omitted — `decodeJSON`
in the hub rejects unknown fields but accepts missing optional fields,
so payload shape stays small per source while the schema underneath
captures the union.

### Transcript-Tail bridge (S2 implementation)

The transcript-tail bridge is the second of three ingest sources, live
since Phase 2d S2 (Auftrag #033). It lives in `internal/agents/transcript.go`
alongside the SDK-hook handler from S1 — Option A (extend `internal/agents/`)
over Option B (new `internal/transcript/`) so the bridge shares the
`store.InsertAgent*` dispatch with the S1 handler. The chosen layout is
documented inline at the top of `transcript.go`.

#### Tail-pattern choice — polling over fsnotify

`time.Ticker` polling at a 1 s default cycle. Rationale:

- Stdlib only, no new dependency; keeps the CGO-free cross-compile
  invariant intact (`modernc.org/sqlite` is the only "external" runtime,
  and it is pure Go).
- Claude-Code transcripts are append-only; we only ever care about new
  bytes past the last consumed offset. fsnotify's strengths
  (rename/delete/truncate edges) do not buy us anything here.
- 1 s tail-cycle latency is fine for forensic-grade dashboards. The
  live stream remains the Phase-2c S2 SSE path; the transcript-tail is
  the slower-but-comprehensive backfill source.
- fsnotify behaviour under heavily-appended files is per-platform
  (inotify Linux, kqueue BSD, ReadDirectoryChangesW Windows) — adding
  test surface for a sub-5 % latency win no human notices.

Tunable via `cfg.Agents.Transcript.PollIntervalMs` (default 1000).

#### Persistence choice — direct store call

The bridge calls `store.InsertAgent*` directly rather than POSTing to
its own `/agents/ingest`. We are inside the hub's address space —
HTTP would force a JSON+auth round-trip per event with no behaviour
win, and a bridge-side crash during an in-flight HTTP self-call would
leak a half-persisted state. The S1 handler's `persist()` is itself a
thin dispatcher over the same `Insert*` methods; calling them directly
just skips the wire re-shape.

#### Verified JSONL field mapping

The Claude-Code transcript JSONL format is not publicly documented,
so the S2 worker brief required pre-hardcoding verification by reading
real transcripts under `~/.claude/projects/-home-kaik-Projekte-tracelab/*.jsonl`.
The verified mapping below was empirically derived from those files
(same anti-doppel-counting discipline that S1's WebFetch-against-hooks-doku
step established):

| JSONL field (where) | Maps to | Notes |
|---|---|---|
| top-level `sessionId` | (not persisted as FK) | Claude-Code conversation id, NOT a tracelab `sessions.id`; populating `agent_spawns.session_ref` from this would always FK-fail. Reserved for future cross-domain joins (S4-S5). |
| top-level `cwd` | `agent_spawns.project` (via basename) | `/home/kaik/Projekte/tracelab` → `"tracelab"`. Empty cwd → `"unknown"` so the row still satisfies the project-required guard. |
| top-level `timestamp` (RFC3339-Z) | `*.ts` (unix-nano) | Unparseable → line skipped silently. |
| top-level `uuid` | (not persisted) | Per-message correlation id; only consumed by the bridge for log lines. |
| top-level `agentId` (subagent streams only) | `agent_spawns.id` (via `padTo26`) | Subagent JSONL lives at `<session>/subagents/agent-<id>.jsonl`; the `agentId` top-level field is the 16-17-char hex id. Padded to 26 chars (ULID-shape) per the S1 convention. |
| top-level `isSidechain: true` | bridge marker for subagent stream | Determines `ownerSkill = "subagent"` vs `"session"`. |
| top-level `type` | bridge dispatch | `"assistant"` → token+spawn-begin emit; `"user"` with `toolUseResult` → verdict + spawn-end emit; everything else → no-op. |
| `message.usage.input_tokens` | `agent_tokens.input_tokens` | Per-LLM-iteration delta, NOT cumulative. |
| `message.usage.output_tokens` | `agent_tokens.output_tokens` | |
| `message.usage.cache_creation_input_tokens` | `agent_tokens.cache_write` | Renamed in schema for brevity. |
| `message.usage.cache_read_input_tokens` | `agent_tokens.cache_read` | |
| `message.content[].type:"tool_use"` with `name:"Agent"\|"Task"\|"Skill"` | `agent_spawns` row (child) | Parent-side first sighting of a child spawn. `spawn_id` derived from `tool_use.id` (`stripNonHex` + `padTo26`); coalesced later when the subagent stream re-emits the same id via the subagent file. |
| `tool_use.input.description` | `agent_spawns.skill` | Worker brief's "description" field is the closest human-readable label. |
| top-level `toolUseResult.agentId` | `agent_spawns.id` (via `padTo26`) | Parent-side spawn-end marker for the just-finished subagent. |
| top-level `toolUseResult.agentType` | `agent_spawns.skill` | E.g. `"general-purpose"`. |
| top-level `toolUseResult.status` | `agent_verdicts.verdict` (via `mapToolResultStatusToVerdict`) | Mapping: `"completed"\|"success"` → `freigabe`, `"error"\|"failed"` → `rueckgabe`, `"escalated"\|"escalation"` → `eskalation`, anything else → `none`. Empty status → no verdict row. |
| `toolUseResult.usage.*` | `agent_tokens` (summary row at spawn-end) | Aggregate totals (vs. per-message deltas above). Same `source="transcript"`, distinct `ts`, so coexists with per-message rows. |

#### Idempotency with the SDK-hook source

The `agent_tokens.UNIQUE(spawn_id, ts, source)` schema constraint
(ADR-013 §Consequences §Per-source-forensic-breakdown) makes the
transcript-tail bridge fully additive against S1's SDK-hook source.
A spawn that both sources see is persisted as:

- exactly ONE row in `agent_spawns` (collapsed on PK by INSERT OR
  IGNORE — neither source needs to know the other ran).
- TWO rows in `agent_tokens` (one per source) so an operator querying
  `/agents/tokens?spawn_id=<X>` sees both counts side by side and
  spots disagreement at a glance.

`TestTranscriptMultiSourceCoexistence` pins this contract empirically.

#### Limitations

- **Polling-latency floor.** Events surface 1 s after they hit disk
  by default; tunable down to ~100 ms before syscall overhead starts
  to bite (operators with a forensic-only deployment should leave it
  at 1000 ms).
- **Multi-file watcher.** `discoverFiles` walks `projectsRoot` on
  every tick. With thousands of project subdirs this would scale
  poorly; current heuristic is "Claude-Code users have O(tens) of
  projects". A `time.Now().Sub(last walk) > 60 s`-style cache could
  be added if walk cost ever shows up in a profile.
- **Partial-line bookkeeping.** A trailing line without `\n` is NOT
  consumed — we wait for the next tick. This is what makes mid-write
  tail safe; downside is a stalled writer leaves the last line
  invisible until either the writer completes or the file is
  truncated.
- **Corrupted-line tolerance.** JSON-decode failure on one line is
  logged + skipped; subsequent lines proceed normally. Pinned by
  `TestTranscriptTailCorruptedLineSkipped`.
- **SessionRef intentionally empty.** The Claude-Code `sessionId` is
  not the same domain as `sessions.id`; populating the FK would always
  break. S4-S5 will add a separate cross-domain join layer.

#### ADR-013: Multi-Ingest pattern for the agent-observability domain

> **Status:** Accepted (confirmed 2026-05-17 via Admin-y-auf-alle-3 nach
> chakotay-routing-bestätigung). Promoted from Proposed at the open of
> S1, before any handler code is written. Form follows the new
> `XBrain/30_Wissen/ADR-Konventionen.md` transition-pattern (Proposed →
> Accepted: status header bumped, Decision body filled with chosen
> variant + date + confirm source).

##### Context

The agent-observability domain (Phase 2d) needs four event classes
captured: spawn lifecycle (begin/end), token-usage deltas, QS verdicts
with Lerneffekt notes, and cross-agent mailbox edges. Three plausible
ingest sources exist, each with its own coverage gap (see Phase-2d
Architecture §Architecture above for the source matrix):

- **SDK-Hooks** (`PostToolUse`, `Stop`) — low-latency push. Gap: a
  worker without hook configuration (or with a hook that errored out
  silently) emits nothing — the lifecycle is lost.
- **Transcript-Tail** — robust pull from `~/.claude/projects/*/*.jsonl`,
  works on every worker regardless of hook config. Gap: 1–5 s tail
  cycle (no sub-second latency); no atomic spawn-begin event (the
  `.jsonl` line for spawn-begin arrives only once the worker has emitted
  its first message).
- **MCP-Push** (`agent_event` tool in `tracelab-mcp`) — explicit,
  workflow-instrumented, atomic with the action being reported. Gap:
  workflow-pflicht — the worker has to actively call the tool; in
  practice this is forgotten or skipped, especially during exploratory
  spawns.

Pre-Phase-2d analysis (Block-Briefing 2026-05-17) found that none of
the three sources alone covers all four event classes reliably across
all worker configurations. The schema and dashboard semantics require
high-completeness lifecycle and usage data; partial coverage degrades
both the spawn-tree visualisation and the token-burn-down chart in S4.

##### Options considered

**(a) Single-Source SDK-Hooks.** Push, low-latency. Reject: workers
without hook config emit nothing; in a heterogeneous skill ecosystem
(some skills are hook-aware, others are not — see XBrain/30_Wissen
skill conventions), this leaves observability holes that show up as
missing tree nodes.

**(b) Single-Source Transcript-Tail.** Pull, robust against config
drift. Reject: 1–5 s latency on spawn-begin means the dashboard live-tail
of the agent stream visibly lags the actual spawn; the operator sees a
spawn finish on the live-tail before they see it begin, which breaks
the mental model.

**(c) Single-Source MCP-Push.** Active, atomic. Reject: workflow-pflicht
is a soft contract — in practice the `agent_event` tool will be
forgotten in exactly the cases that matter most (an escalating spawn
that doesn't follow its own conventions). Missed-emit failure mode is
silent: there is no "did the worker call the tool?" probe.

**(d) Multi-Ingest with all three sources writing into the same schema.**
Coverage via redundancy: a missed-emit by one source is caught by
another. Cost: idempotency must be enforced at the schema layer (the
hub cannot trust the sources to agree on event identity), and the
operator needs a forensic view when two sources disagree on the same
event's data (token counts, verdict text).

##### Decision

**(d) Multi-Ingest 3 Quellen (SDK-Hooks + Transcript-Tail + MCP-Push)**
— gewählt am 2026-05-17, confirmed durch Admin via
chakotay-routing-bestätigung.

Begründung: Robustheit gegen Hook-Lücken ((a) unzureichend),
Vollständigkeit via Transcript-Tail ((b) unzureichend ohne
Push-Atomarität), Explicitness bei MCP-Push ((c) unzureichend ohne
Hook-Coverage). Idempotenz via spawn-ID + UNIQUE-Tupel macht die 3
Quellen kollisionsfrei. Die vier Event-Class-zu-Primary-Source-Mappings
in §Architecture §Tripwire pattern sind Teil dieser Decision (welche
Quelle ist authoritative für welche Event-Klasse; welche Quelle agiert
als Fallback unter welchem Trigger).

##### Consequences

- **Idempotency is enforced at the schema layer.** Three UNIQUE-tuple
  constraints — `agent_tokens(spawn_id, ts, source)`,
  `agent_verdicts(spawn_id, verdict, ts)`,
  `agent_mailbox_edges(from_spawn_id, to_spawn_id, edge_type, ts)` —
  collapse same-event-from-same-source duplicates to one row. The
  `agent_spawns.id` is supplied by the writer as a ULID, globally
  unique across all sources, so the spawn lifecycle row needs no
  composite UNIQUE.
- **Per-source forensic breakdown is preserved.** `agent_tokens.source`
  is a column, not just a write-time-only discriminator — when two
  sources report different `input_tokens` for the same `(spawn_id, ts)`,
  both rows survive and `/agents/tokens` exposes the disagreement
  through the `by_source` breakdown. This makes "the hook and the
  transcript disagree by 200 tokens" investigatable instead of
  silently averaged-away.
- **Tripwire-tests are mandatory in S1+S2.** When the primary source
  for an event class is silent past the 60 s heartbeat threshold while
  spawn-end is still pending (`ended_at IS NULL`), the hub logs a
  structured `WARN` and the fallback row becomes the surviving record
  under the UNIQUE-tuple. The tests pin: (i) fallback fills the gap
  with no data loss, (ii) primary recovery does not re-collide on the
  fallback's surviving row, (iii) `WARN` is emitted (not a silent
  fallback).
- **Wire-compat of the `/agents/ingest` payload.** Fields are
  fakultativ per source (a Transcript-Tail call often carries only
  `tokens` and `verdict` for an existing `spawn_id`; an SDK-Hook call
  on `Stop` carries the full lifecycle; an MCP-Push call carries
  whatever the worker reports). The `source` discriminator is
  required. Adding a fourth source later (e.g. an OpenTelemetry
  exporter) is a new value in the `CHECK(source IN ...)` enum on
  `agent_tokens.source` plus a schema migration — backwards compatible
  with the three existing sources (their payload shape doesn't change).
- **Three concurrent writers in production.** The hub's existing
  SQLite `MaxOpenConns=1` discipline (`internal/store/sqlite.go`)
  already serialises concurrent writes; multi-ingest does not change
  that contract. Throughput envelope is unchanged (the bottleneck is
  not three POSTs vs one; it is the SQLite single-writer lane).
- **Audit-trail: `agent_spawns` rows never carry a `source`.** The
  spawn identity is one ULID, regardless of which source first wrote
  it. The `source` discrimination only matters per event below the
  spawn — tokens, verdicts, edges — where two sources can carry the
  same event semantics but disagree on the payload values.
- **No new dependency.** All three sources POST plain JSON to
  `/agents/ingest`. SDK-Hooks shells out to `curl` (or a tiny
  cross-compiled CLI helper, decided in S1); Transcript-Tail runs
  in-process inside the hub (no second daemon); MCP-Push reuses
  `internal/client/` from inside `tracelab-mcp`.

##### Rejected (in detail)

- **(a) SDK-Hooks-only** — see §Options. The deal-breaker is the
  silent-config-drift mode: a hook config that errors out emits
  nothing, and the operator only notices when the spawn-tree shows
  gaps. By the time the gap is investigated, the spawn is long over.
- **(b) Transcript-Tail-only** — the spawn-begin latency makes the
  live-tail-of-agents tab visibly wrong. The dashboard's mental model
  is "what is happening right now" — a 5 s lag on lifecycle events
  breaks that, and there is no way to repair it within a pull-only
  architecture.
- **(c) MCP-Push-only** — the missed-emit failure mode is silent and
  correlates exactly with the most-interesting spawns (escalations,
  unusual workflows). Relying on a soft contract for the data domain
  that exists to *observe* skill conventions is circular.

##### Wire-compat statement

The `/agents/ingest` payload accepts fakultative fields per source plus
a required `source` discriminator. When a source is removed (e.g.
hook-config is rolled back), the remaining sources continue to write
under the same payload shape with no protocol change. Adding a fourth
source is one schema migration (the `CHECK(source IN ...)` enum on
`agent_tokens.source`) plus an additive value — no breaking change to
the three existing sources. The agent-spawns table needs no migration
when a new source joins (the spawn identity is source-independent).

The four read endpoints (`/agents/sessions`, `/agents/tree/{id}`,
`/agents/tokens`, `/agents/verdicts`) are pure read views — adding or
removing a source affects the data they return but not their shape.
The `/agents/tokens` `by_source` breakdown grows a key when a source
joins and loses a key when a source leaves; consumers tolerate this
since each `by_source.<name>` block is independent.
