# agent-hook.sh — Claude Code SDK-Hook adapter for tracelab

Bridges Claude Code's `PostToolUse` + `Stop` lifecycle hooks into the
tracelab-hub `POST /agents/ingest` endpoint (Phase 2d, ADR-013 Source
`sdk-hook`).

## Why this script

ADR-013 defines three concurrent ingest sources for the
agent-observability domain. `sdk-hook` is the low-latency push path —
the hook fires inline with the agent's tool calls, so spawn-begin
events are reported within milliseconds of the actual subagent
invocation. The trade-off is coverage: a workflow without hook
configuration emits nothing.

In Phase 2d S1 this script only reports the spawn lifecycle (begin
on `PostToolUse` of the `Task` tool, end-marker on `Stop`). Token
usage and rich verdict text arrive via the transcript-tail source
in S2 — the hook payload does not contain token counts (verified
against the official hook schema 2026-05-17), so trying to derive
them here would be best-effort guesswork at best.

## Verified hook payload shape

```json
{
  "session_id":      "abc123",
  "transcript_path": "/.../.claude/projects/.../transcript.jsonl",
  "cwd":             "/path/to/project",
  "permission_mode": "default",
  "hook_event_name": "PostToolUse",
  "effort":          { "level": "medium" },
  "agent_id":        "subagent-id (optional, only in subagent context)",
  "agent_type":      "subagent type name (optional)",

  "tool_name":   "Task",
  "tool_input":  { "...": "varies by tool" },
  "tool_use_id": "tool_xyz123",
  "tool_result": { "type": "text", "content": "...", "isError": false }
}
```

Source: https://code.claude.com/docs/en/hooks (fetched 2026-05-17).

## Configuration

The script reads three environment variables:

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `TRACELAB_TOKEN` | yes | (none — empty exits 0 silently) | bearer token for the hub |
| `TRACELAB_HUB` | no | `http://127.0.0.1:8765` | hub base URL |
| `TRACELAB_AGENT_SKILL` | no | `agent_type` from payload, else `claude-code-session` | spawn.skill column |
| `TRACELAB_AGENT_PROJECT` | no | basename of cwd from payload | spawn.project column |

A missing `TRACELAB_TOKEN` is the explicit no-op signal — the script
exits 0 so a host without tracelab configured isn't disrupted.

## Wiring into Claude Code

Add to `~/.claude/settings.json`:

```jsonc
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": { "tools": ["Task"] },
        "command": "/path/to/tracelab/scripts/agent-hook.sh"
      }
    ],
    "Stop": [
      {
        "command": "/path/to/tracelab/scripts/agent-hook.sh"
      }
    ]
  }
}
```

The `matcher` on `PostToolUse` is an optimisation: without it the hook
would run on every tool call (Bash, Write, Read, etc.) and exit
early. With it, claude-code only invokes the hook for `Task`-tool
calls (i.e. subagent invocations), which is the only case that maps
to a spawn-begin event.

## Behaviour by event

| Event | Tool filter | Sends to /agents/ingest |
|---|---|---|
| `PostToolUse` | only `tool_name == "Task"` | `source: sdk-hook`, `spawn{id,skill,started_at,project}` |
| `Stop` | all | `source: sdk-hook`, `verdicts[0]{spawn_id, verdict:"none", ts}` |
| anything else | n/a | exits 0 silently |

The `verdict: "none"` on Stop is a liveness marker, NOT the real
verdict. The S2 transcript-tail will write the real
`freigabe`/`auflagen`/etc verdict from the worker's actual report.
Multi-ingest collapses to dedup at the storage layer (ADR-013) so
having both rows is harmless.

## Spawn-id contract

The schema requires `agent_spawns.id` to be a 26-char ULID-shaped
TEXT primary key. The hook adapter derives it deterministically from
`agent_id` (if present, i.e. inside a subagent) or `session_id`
(top-level session), then pads/truncates to 26 chars. The same
Claude Code session always maps to the same spawn id — so a Stop
hook can join the spawn row that a PostToolUse hook wrote earlier.

## Exit codes

Always 0. Hooks must not block claude-code. Hub errors (4xx/5xx,
timeout, connection refused) are logged to stderr — visible in
`~/.claude/projects/*/hooks.log` if claude-code's hook logging is
enabled, otherwise discarded.

## Dependencies

`bash`, `jq`, `curl`. All standard on Fedora / Debian / macOS.

## Out of scope (S1)

* Token usage — not present in the hook payload, sourced via
  transcript-tail in S2.
* Mailbox-edges — derivable from the subagent spawn tree but the
  hook fires once per spawn, not once per edge; the transcript-tail
  is the right source here too (S2).
* PreToolUse / SessionStart / UserPromptSubmit — not yet mapped; if
  a use case appears, the dispatch switch in the script is the only
  thing that needs changing.

## Testing

A future S2 step will check in a recorded sample payload as a
fixture under `scripts/agent-hook-sample-payload.json` plus a
golden-file test that pipes it into the script and asserts the
POST shape. For S1, the script is exercised live against a running
hub via `make hub` + a manual claude-code session.
