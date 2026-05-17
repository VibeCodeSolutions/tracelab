#!/usr/bin/env bash
#
# agent-hook.sh — Tracelab Phase-2d SDK-Hook ingest adapter.
#
# Reads a Claude Code hook payload from stdin and POSTs a derived
# IngestPayload to the tracelab-hub /agents/ingest endpoint.
#
# Hook payload reference (verified 2026-05-17 against
# https://code.claude.com/docs/en/hooks):
#
#   Common envelope:
#     session_id, transcript_path, cwd, permission_mode,
#     hook_event_name, effort.level
#     optional under --agent or subagent context:
#       agent_id, agent_type
#
#   PostToolUse adds:
#     tool_name, tool_input{}, tool_use_id, tool_result{type,content,isError}
#
#   Stop adds:
#     (only the common envelope today — token-usage is NOT present;
#      that is sourced from the transcript-tail in S2, see ADR-013
#      §Options Considered §(a) for the rationale).
#
# Mapping decisions for this S1 skeleton:
#
#   * Spawn identity: agent_id when present, else session_id.
#     A top-level Claude Code session without subagent invocation
#     therefore reports its OWN session as the spawn-id; nested
#     subagents get their own agent_id and thread that through.
#
#   * Spawn-begin trigger: PostToolUse with tool_name == "Task"
#     (the subagent-spawning tool). The hook fires AFTER the call
#     succeeds, so the spawn is real by the time we report it.
#
#   * Spawn-end trigger: Stop. We report spawn-end without
#     ended_at on the spawn row itself (the row was likely written
#     by the begin-event already), and emit a verdict='none' as a
#     liveness marker until S2 brings the transcript-tail with
#     actual lifecycle markers.
#
#   * skill / project: skill defaults to the agent_type if present,
#     else "claude-code-session". project defaults to the basename
#     of cwd. Both are fakultativ on the wire — the hub validates
#     spawn.skill + spawn.project as required, but those defaults
#     give the hub a usable row even if the operator forgot to set
#     TRACELAB_AGENT_SKILL / TRACELAB_AGENT_PROJECT env vars.
#
# Environment:
#
#   TRACELAB_HUB           hub base URL, default http://127.0.0.1:8765
#   TRACELAB_TOKEN         bearer token (required — empty exits 0 silently
#                          so a missing config never breaks Claude Code)
#   TRACELAB_AGENT_SKILL   override default skill (optional)
#   TRACELAB_AGENT_PROJECT override default project name (optional)
#
# Exit codes:
#
#   Always 0 — Claude Code hooks MUST NOT block the agent workflow.
#   A failure to reach the hub is logged to stderr (visible in
#   claude-code's hook log) but does not propagate.
#
# Dependencies: bash, jq, curl (all standard on Fedora/Debian/macOS).

set -u
set -o pipefail

HUB_URL="${TRACELAB_HUB:-http://127.0.0.1:8765}"
TOKEN="${TRACELAB_TOKEN:-}"
SKILL_OVERRIDE="${TRACELAB_AGENT_SKILL:-}"
PROJECT_OVERRIDE="${TRACELAB_AGENT_PROJECT:-}"

# Silent no-op if the token is missing. The hook must not crash the
# host claude-code session because tracelab isn't configured.
if [[ -z "$TOKEN" ]]; then
  exit 0
fi

# Buffer stdin so we can read it twice (jq + payload dispatch).
RAW="$(cat || true)"
if [[ -z "$RAW" ]]; then
  exit 0
fi

# Tolerate missing jq — if jq isn't installed we can't shape the
# payload, but we don't want to fail loudly either. Log to stderr
# and exit clean.
if ! command -v jq >/dev/null 2>&1; then
  echo "tracelab agent-hook: jq missing, skipping ingest" >&2
  exit 0
fi

EVENT_NAME="$(jq -r '.hook_event_name // empty' <<<"$RAW")"
if [[ -z "$EVENT_NAME" ]]; then
  exit 0
fi

# Resolve spawn identifier: prefer the subagent-specific agent_id,
# fall back to the top-level session_id.
SPAWN_ID="$(jq -r '.agent_id // .session_id // empty' <<<"$RAW")"
if [[ -z "$SPAWN_ID" ]]; then
  exit 0
fi

# Pad / truncate to 26 chars to match the ULID-shaped TEXT PK
# contract from the agent_spawns schema. We use a deterministic
# transform so the same Claude Code session always maps to the same
# spawn id, even though session_id is technically longer/shorter.
# Implementation: take the first 26 chars; if shorter, pad with 'a'.
if (( ${#SPAWN_ID} > 26 )); then
  SPAWN_ID="${SPAWN_ID:0:26}"
elif (( ${#SPAWN_ID} < 26 )); then
  PAD=$(printf 'a%.0s' $(seq 1 $((26 - ${#SPAWN_ID}))))
  SPAWN_ID="${SPAWN_ID}${PAD}"
fi

CWD="$(jq -r '.cwd // empty' <<<"$RAW")"
PROJECT="${PROJECT_OVERRIDE:-${CWD##*/}}"
if [[ -z "$PROJECT" ]]; then
  PROJECT="unknown"
fi

AGENT_TYPE="$(jq -r '.agent_type // empty' <<<"$RAW")"
SKILL="${SKILL_OVERRIDE:-${AGENT_TYPE:-claude-code-session}}"

TS_NS=$(date +%s%N)
TOOL_NAME="$(jq -r '.tool_name // empty' <<<"$RAW")"

# Dispatch on event kind.
PAYLOAD=""
case "$EVENT_NAME" in
  PostToolUse)
    # Only Task-tool calls map to spawn-begin events; other tools
    # (Bash, Write, Read, etc.) don't need a spawn row.
    if [[ "$TOOL_NAME" != "Task" ]]; then
      exit 0
    fi
    PAYLOAD="$(jq -nc \
      --arg id "$SPAWN_ID" \
      --arg skill "$SKILL" \
      --arg project "$PROJECT" \
      --argjson started_at "$TS_NS" \
      '{
         source: "sdk-hook",
         spawn: {
           id: $id,
           skill: $skill,
           started_at: $started_at,
           project: $project
         }
       }')"
    ;;
  Stop)
    # Spawn-end: write a liveness verdict ('none') and let S2
    # transcript-tail provide the real verdict when it lands.
    PAYLOAD="$(jq -nc \
      --arg id "$SPAWN_ID" \
      --argjson ts "$TS_NS" \
      '{
         source: "sdk-hook",
         verdicts: [{
           spawn_id: $id,
           verdict: "none",
           ts: $ts
         }]
       }')"
    ;;
  *)
    # Unknown event — nothing to do.
    exit 0
    ;;
esac

# POST to the hub. Capture both stdout (body) and the HTTP code via
# -w. A 4xx/5xx is logged to stderr but never propagated as a
# non-zero exit.
RESP="$(
  curl --silent \
       --show-error \
       --max-time 5 \
       --output /dev/null \
       --write-out "%{http_code}" \
       -X POST \
       -H "Authorization: Bearer ${TOKEN}" \
       -H "Content-Type: application/json" \
       --data "$PAYLOAD" \
       "${HUB_URL}/agents/ingest" 2>&1 || true
)"

if [[ "$RESP" != "200" ]]; then
  echo "tracelab agent-hook: ${EVENT_NAME} POST -> HTTP ${RESP}" >&2
fi

exit 0
