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

## Phase 2c — Dashboard (placeholder)

ADR pending — **scope and stack to be discussed with Admin before start**
(per plan-briefing 2026-05-13). No defaults set in advance.
