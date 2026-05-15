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

#### ADR-007: Tool surface (skeleton — finalised in S2)

The MCP server exposes four tools, mirroring the CLI sub-commands so that
Claude Code and the human operator drive Tracelab through the same mental
model. Naming, exact signatures, and the tool-vs-resource decision for
`tail` are an explicit Auto-Stop in Sub-Sprint S2.

- **`sessions`** — list + get-by-id. Reuses `internal/client.ListSessions`.
  No hub-side change.
- **`tail`** — live event stream for a session (or all sessions). Reuses
  `internal/client.Tail` (WebSocket). MCP-side shape (single streaming
  tool call vs. resource subscription) is the S2 decision.
- **`adb`** — devices / start / stop. Reuses the `/adb/*` HTTP surface
  established in Phase 2a S5 (ADR-004 Option B). No hub-side change.
- **`crashes`** — list crash events for a session. **⚠ S6 risk:** the hub
  has no `/crashes` HTTP endpoint today (verified 2026-05-15 by `grep -rn
  "/crashes" internal/http/`; the `crashes` table exists in `internal/
  store/`, including migrations 0001 + 0002, and `internal/store/sqlite.go`
  line 397 explicitly documents the gap with `// Used by tests and the
  future /crashes API.`). S6 therefore needs an additive Hub-Schema-Change
  (new HTTP endpoint reading from the existing store) — analogous to the
  Phase-2a S5 / ADR-004 pattern. This is a registered Auto-Stop trigger in
  the Phase-2b plan briefing; Admin-confirm is required before S6 starts.

Tool naming convention, auth strategy (Bearer from `tracelab.toml` via
shared `internal/cliconfig/`), and per-tool input/output schemas are
written into this ADR during S2.

## Phase 2c — Dashboard (placeholder)

ADR pending — **scope and stack to be discussed with Admin before start**
(per plan-briefing 2026-05-13). No defaults set in advance.
