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
| **S5 — `adb` sub-cmd** | **open design — see ADR-004 below** | may trigger hub API change → Auto-Stop |
| **S6 — `run` sub-cmd** | **open design — see ADR-005 below** | daemon-management strategy → Auto-Stop |

S1–S4 are well-defined and can proceed once ADR-001/-002/-003 are approved.
S5 and S6 each require an explicit decision before they enter implementation.

### Open ADRs — Auto-Stop before S5 / S6

#### ADR-004 (OPEN): `tracelab adb` scope

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

**Decision pending:** Admin confirm before S5 starts.

#### ADR-005 (OPEN): `tracelab run` semantics

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

**Decision pending:** Admin confirm before S6 starts (or removal from DoD).

---

## Phase 2b — MCP server (placeholder)

ADR pending — written at start of Phase 2b. Will reuse `internal/client/`.

## Phase 2c — Dashboard (placeholder)

ADR pending — **scope and stack to be discussed with Admin before start**
(per plan-briefing 2026-05-13). No defaults set in advance.
