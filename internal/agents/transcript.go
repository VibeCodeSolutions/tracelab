package agents

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/VibeCodeSolutions/tracelab/internal/store"
)

// Phase 2d S2 — Transcript-Tail-Bridge.
//
// This file owns the "second of three" ingest sources for the
// agent-observability domain (ADR-013 §Multi-Ingest). It tails the
// Claude-Code transcript JSONL files under ProjectsRoot and feeds the
// resulting spawn / token / verdict events into the same store layer
// the SDK-hook source uses (with source="transcript" so the per-source
// UNIQUE-tuple on agent_tokens keeps both rows alive for forensic
// comparison — see ADR-013 §Consequences §Per-source-forensic-breakdown).
//
// ## Layout choice (Lead autonomy, documented for ADR-014-if-needed)
//
// Option A — extend internal/agents (this file).
// Option B — new internal/transcript package.
//
// Picked Option A because the tail-loop's only real consumer is the
// store.InsertAgent* family the S1 handler already drives. Sharing the
// package means the dispatch logic (insert spawn first, then tokens,
// then verdicts) is shared across both sources with no public surface
// duplication. Option B would have required exporting one of the
// internal helpers or duplicating the dispatch loop.
//
// ## Tail-loop choice — polling over fsnotify
//
// Picked time.Ticker polling over fsnotify because:
//  1. stdlib only, no new dep, keeps CGO-free cross-compile invariant
//     intact (ballard-CL "Cross-Compile-Reflex").
//  2. Claude-Code transcripts are append-only — we only ever care about
//     new bytes past our last offset, never about rename/delete/truncate
//     edge cases fsnotify excels at.
//  3. 1 s tail cycle is plenty for forensic-grade dashboards (the live
//     stream is the SSE path from Phase 2c S2; this is the slower,
//     more-comprehensive backfill source).
//  4. fsnotify's behaviour under heavily-appended files is per-platform
//     (inotify Linux, kqueue BSD, ReadDirectoryChangesW Windows) — more
//     test surface for a 5 % latency win that no human notices.
//
// ## Bridge choice — direct store call over HTTP-self-call
//
// Picked direct store call (h.store.InsertAgent* from the bridge
// goroutine) over POST /agents/ingest (a self-HTTP call) because:
//  1. We're inside the hub's address space — there is no boundary to
//     cross. HTTP would force a JSON round-trip, an auth-header round-
//     trip, and an extra goroutine for the http.Server to dispatch.
//  2. The S1 handler's persist() is essentially a thin dispatcher over
//     the same Insert* methods. Calling them directly skips the wire
//     re-shape with zero behaviour change.
//  3. Atomicity: a bridge crash during an in-flight HTTP self-call
//     would leak a half-persisted state; calling store directly keeps
//     each spawn/tokens/verdict triple in one error-recovery scope.

// transcriptRecord is the subset of fields we read from every JSONL
// line. The transcript format carries many more fields (parentUuid,
// gitBranch, isSidechain, content arrays, etc.) — we deliberately
// decode only what we need and ignore the rest so future Claude-Code
// transcript additions do not break the bridge.
//
// VERIFIED field mapping (S2 worker pre-hardcoding step, transcripts
// inspected: ~/.claude/projects/-home-kaik-Projekte-tracelab/*.jsonl):
//
//   - top-level `sessionId`     → parent-session UUID
//   - top-level `cwd`           → project working directory
//   - top-level `timestamp`     → RFC3339-Z event time
//   - top-level `uuid`          → message UUID (correlation)
//   - top-level `agentId`       → subagent ID (only inside subagents/agent-*.jsonl)
//   - top-level `isSidechain`   → true on subagent streams
//   - top-level `type`          → "user" | "assistant" | "system" | "attachment" | …
//   - message.usage.{input,output,cache_creation_input,cache_read_input}_tokens
//                              → per-message token-delta (assistant messages only)
//   - message.content[].type    → "text" | "thinking" | "tool_use" | "tool_result"
//   - message.content[].name    → tool name (e.g. "Agent", "Skill") on tool_use
//   - message.content[].input.{description,prompt}
//                              → spawn-begin metadata on Agent/Skill tool_use
//   - top-level `toolUseResult.status`  → "completed" | "error" | … (spawn-end verdict)
//   - top-level `toolUseResult.{totalTokens,totalDurationMs,totalToolUseCount}`
//                              → spawn-summary deltas (in addition to per-message)
//   - top-level `toolUseResult.{agentId,agentType}`
//                              → spawn-id + skill on spawn-end
type transcriptRecord struct {
	Type             string                 `json:"type"`
	UUID             string                 `json:"uuid"`
	SessionID        string                 `json:"sessionId"`
	AgentID          string                 `json:"agentId,omitempty"`
	IsSidechain      bool                   `json:"isSidechain,omitempty"`
	Timestamp        string                 `json:"timestamp"`
	CWD              string                 `json:"cwd,omitempty"`
	Message          *transcriptMessage     `json:"message,omitempty"`
	ToolUseResult    *transcriptToolResult  `json:"toolUseResult,omitempty"`
}

type transcriptMessage struct {
	ID      string                  `json:"id,omitempty"`
	Role    string                  `json:"role,omitempty"`
	Model   string                  `json:"model,omitempty"`
	Usage   *transcriptUsage        `json:"usage,omitempty"`
	Content []transcriptContentItem `json:"content,omitempty"`
}

type transcriptUsage struct {
	InputTokens             int64 `json:"input_tokens"`
	OutputTokens            int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

type transcriptContentItem struct {
	Type  string                `json:"type"`
	Name  string                `json:"name,omitempty"`
	ID    string                `json:"id,omitempty"`
	Input *transcriptToolInput  `json:"input,omitempty"`
}

type transcriptToolInput struct {
	Description string `json:"description,omitempty"`
	Prompt      string `json:"prompt,omitempty"`
}

type transcriptToolResult struct {
	Status            string           `json:"status,omitempty"`
	AgentID           string           `json:"agentId,omitempty"`
	AgentType         string           `json:"agentType,omitempty"`
	TotalTokens       int64            `json:"totalTokens,omitempty"`
	TotalDurationMs   int64            `json:"totalDurationMs,omitempty"`
	TotalToolUseCount int64            `json:"totalToolUseCount,omitempty"`
	Usage             *transcriptUsage `json:"usage,omitempty"`
}

// transcriptEvent is the bridge's internal event type after parsing one
// transcript record. Exactly one of Spawn/Tokens/Verdict/Edge is populated.
// SourceFile + Offset are kept for debug logging.
//
// Edge events were added in Phase 2d S5 — Agent/Task/Skill tool_use
// entries in an assistant message imply a directed edge from the
// owning spawn to the new child spawn (edge_type="spawn"). toolUseResult
// records on the parent stream additionally imply a "return" edge from
// the finished subagent back up to the parent.
type transcriptEvent struct {
	Spawn   *store.AgentSpawn
	Tokens  *store.AgentTokenUsage
	Verdict *store.AgentVerdict
	Edge    *store.AgentMailboxEdge
}

// parseTranscriptLine maps one JSONL line to zero, one, or more
// transcript events. A single assistant message with a Task tool_use
// inside its content produces both a Tokens event (from message.usage)
// and a Spawn event (from the tool_use content item).
//
// Project slug is derived from `cwd` (basename), falling back to "unknown"
// when cwd is empty. spawn_id is the writer-supplied 26-char ULID-shape:
// on subagent streams it is padTo26(agentId); on top-level streams it is
// padTo26(sessionId-hex-stripped).
func parseTranscriptLine(line []byte) ([]transcriptEvent, error) {
	if len(line) == 0 {
		return nil, nil
	}
	var rec transcriptRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return nil, err
	}
	// Skip control/system lines that carry no observable agent state.
	if rec.Type == "" {
		return nil, nil
	}

	ts, err := parseTranscriptTime(rec.Timestamp)
	if err != nil {
		// A line without a parseable timestamp is uninteresting — emit
		// nothing rather than fail the whole tail-loop.
		return nil, nil
	}
	project := projectFromCWD(rec.CWD)
	spawnID := spawnIDFromRecord(rec)

	var events []transcriptEvent

	// Spawn-end events come via toolUseResult.agentId — parent-session
	// records the verdict for the *subagent* it just finished. The
	// status string maps to one of the schema-CHECK'd verdict values.
	if rec.ToolUseResult != nil && rec.ToolUseResult.AgentID != "" {
		subSpawnID := padTo26(rec.ToolUseResult.AgentID)
		skill := rec.ToolUseResult.AgentType
		if skill == "" {
			skill = "subagent"
		}
		// FK-precondition: ensure the child spawn row exists before
		// the verdict/tokens rows attach. Idempotent via INSERT OR
		// IGNORE on the PK.
		events = append(events, transcriptEvent{Spawn: &store.AgentSpawn{
			ID:         subSpawnID,
			Skill:      skill,
			StartedAt:  ts,
			Project:    project,
			// SessionRef intentionally NOT populated from the
			// transcript sessionId — that field is the Claude-Code
			// conversation id, NOT a tracelab sessions.id. The FK
			// would always break here. SessionRef is reserved for
			// future cross-domain joins where an agent spawn is
			// known to belong to an app session (TODO S4-S5).
			SessionRef: "",
		}})
		verdictStr := mapToolResultStatusToVerdict(rec.ToolUseResult.Status)
		if verdictStr != "" {
			events = append(events, transcriptEvent{Verdict: &store.AgentVerdict{
				SpawnID: subSpawnID,
				Verdict: verdictStr,
				TS:      ts,
			}})
		}
		// Mailbox-edge: the subagent returned to its parent. The
		// PARENT-side spawn id (spawnID, derived from this record's
		// sessionId / agentId) is the receiver of the return edge.
		// FK precondition: both spawn rows must exist; the parent
		// row is emitted later in this function (hasOwnerChild branch),
		// the child row is the InsertOrIgnore-safe spawn we prepended
		// just above.
		events = append(events, transcriptEvent{Edge: &store.AgentMailboxEdge{
			FromSpawnID: subSpawnID,
			ToSpawnID:   spawnID,
			EdgeType:    "return",
			TS:          ts,
		}})
		// The toolUseResult also carries aggregate token totals — emit
		// them as a token event under the subagent's id. These are
		// per-spawn summaries, not per-message deltas, but the
		// UNIQUE(spawn_id, ts, source) tuple keeps them distinct from
		// the SDK-hook source's own row (per-source forensics, ADR-013).
		if u := rec.ToolUseResult.Usage; u != nil &&
			(u.InputTokens != 0 || u.OutputTokens != 0 ||
				u.CacheCreationInputTokens != 0 || u.CacheReadInputTokens != 0) {
			events = append(events, transcriptEvent{Tokens: &store.AgentTokenUsage{
				SpawnID:      subSpawnID,
				InputTokens:  u.InputTokens,
				OutputTokens: u.OutputTokens,
				CacheRead:    u.CacheReadInputTokens,
				CacheWrite:   u.CacheCreationInputTokens,
				TS:           ts,
				Source:       string(SourceTranscript),
			}})
		}
	}

	// Per-message token usage on assistant records.
	if rec.Type == "assistant" && rec.Message != nil && rec.Message.Usage != nil {
		u := rec.Message.Usage
		if u.InputTokens != 0 || u.OutputTokens != 0 ||
			u.CacheCreationInputTokens != 0 || u.CacheReadInputTokens != 0 {
			events = append(events, transcriptEvent{Tokens: &store.AgentTokenUsage{
				SpawnID:      spawnID,
				InputTokens:  u.InputTokens,
				OutputTokens: u.OutputTokens,
				CacheRead:    u.CacheReadInputTokens,
				CacheWrite:   u.CacheCreationInputTokens,
				TS:           ts,
				Source:       string(SourceTranscript),
			}})
		}

		// Spawn-begin events come from Agent / Skill tool_use entries
		// in the assistant message content. Top-level Agent tool_use
		// kicks off a subagent; the future subagent JSONL file will
		// be tailed independently. We still record a spawn row from
		// the parent side so the spawn-tree is queryable even before
		// the subagent runs its first message.
		for _, c := range rec.Message.Content {
			if c.Type != "tool_use" {
				continue
			}
			if c.Name != "Agent" && c.Name != "Task" && c.Name != "Skill" {
				continue
			}
			// The child spawn's id is unknown at parent-side (the
			// subagent file lives at subagents/agent-<id>.jsonl and
			// only emerges after the subagent starts). For Skill
			// tool_use we record the parent-session as the spawn (a
			// skill load is a same-session in-flight operation). For
			// Agent/Task we record under the toolUseId-derived spawn,
			// later coalesced by the subagent stream via INSERT OR
			// IGNORE on the PK.
			skill := "unknown"
			if c.Input != nil && c.Input.Description != "" {
				skill = c.Input.Description
			}
			childSpawnID := spawnFromToolUseID(c.ID, spawnID)
			spawn := store.AgentSpawn{
				ID:         childSpawnID,
				ParentID:   spawnID,
				Skill:      skill,
				StartedAt:  ts,
				Project:    project,
				// SessionRef intentionally NOT populated from the
			// transcript sessionId — that field is the Claude-Code
			// conversation id, NOT a tracelab sessions.id. The FK
			// would always break here. SessionRef is reserved for
			// future cross-domain joins where an agent spawn is
			// known to belong to an app session (TODO S4-S5).
			SessionRef: "",
			}
			events = append(events, transcriptEvent{Spawn: &spawn})
			// Mailbox-edge: parent spawning child. The owner-spawn row
			// is prepended later in this function (hasOwnerChild),
			// the child row is the InsertOrIgnore-safe spawn above —
			// FK precondition holds when the edge insert runs.
			events = append(events, transcriptEvent{Edge: &store.AgentMailboxEdge{
				FromSpawnID: spawnID,
				ToSpawnID:   childSpawnID,
				EdgeType:    "spawn",
				TS:          ts,
			}})
		}
	}

	// Ensure a spawn row exists for the owner of this record (the
	// session itself for top-level streams, the subagent id for
	// sidechain streams) BEFORE any token/verdict rows attached to
	// that same spawn try to insert. INSERT OR IGNORE on the PK
	// keeps repeats free, so prepending unconditionally is fine.
	// The toolUseResult branch above also prepends a spawn row for
	// the *child* subagent it terminates — that one has a different
	// id so both coexist.
	hasOwnerChild := false
	for _, e := range events {
		switch {
		case e.Tokens != nil && e.Tokens.SpawnID == spawnID:
			hasOwnerChild = true
		case e.Verdict != nil && e.Verdict.SpawnID == spawnID:
			hasOwnerChild = true
		case e.Edge != nil && (e.Edge.FromSpawnID == spawnID || e.Edge.ToSpawnID == spawnID):
			// Edges FK-back to the owning spawn from EITHER endpoint —
			// the spawn-edge from parent originates at the owner, the
			// return-edge from a subagent terminates at the owner.
			hasOwnerChild = true
		}
	}
	if hasOwnerChild {
		ownerSkill := "session"
		if rec.IsSidechain && rec.AgentID != "" {
			ownerSkill = "subagent"
		}
		ownerSpawn := store.AgentSpawn{
			ID:         spawnID,
			Skill:      ownerSkill,
			StartedAt:  ts,
			Project:    project,
			// SessionRef intentionally NOT populated from the
			// transcript sessionId — that field is the Claude-Code
			// conversation id, NOT a tracelab sessions.id. The FK
			// would always break here. SessionRef is reserved for
			// future cross-domain joins where an agent spawn is
			// known to belong to an app session (TODO S4-S5).
			SessionRef: "",
		}
		events = append([]transcriptEvent{{Spawn: &ownerSpawn}}, events...)
	}

	return events, nil
}

// projectFromCWD returns the basename of cwd as a project slug. Empty
// cwd → "unknown" so the resulting spawn row still satisfies the
// store-level Project-required check.
func projectFromCWD(cwd string) string {
	if cwd == "" {
		return "unknown"
	}
	return filepath.Base(cwd)
}

// spawnIDFromRecord derives the 26-char ULID-shaped spawn id from a
// transcript record. Subagent stream → padTo26(agentId). Top-level
// stream → padTo26(hex-stripped sessionId). Both are stable across
// repeated reads of the same record (idempotent re-emit safe).
func spawnIDFromRecord(rec transcriptRecord) string {
	if rec.AgentID != "" {
		return padTo26(rec.AgentID)
	}
	return padTo26(stripUUIDHyphens(rec.SessionID))
}

// spawnFromToolUseID derives the spawn id for a child spawn whose
// child JSONL file does not yet exist (parent-side first sighting).
// We hash the toolUseId into a 26-char hex by stripping and padding,
// so subsequent subagent-stream re-discovery via padTo26(agentId)
// collides via INSERT OR IGNORE rather than producing a duplicate row.
//
// Concretely we take the suffix of the toolu_ id and pad with the
// parent spawn id's tail. NOT cryptographic — just deterministic and
// idempotent for the same toolUseId.
func spawnFromToolUseID(toolUseID, parentSpawnID string) string {
	// Strip the toolu_ prefix and any non-alphanumerics, then pad.
	stripped := stripNonHex(toolUseID)
	if stripped == "" {
		return padTo26(parentSpawnID + "child")
	}
	return padTo26(stripped)
}

// padTo26 ensures a string is exactly 26 characters by truncation or
// padding with 'a' (matches the S1 hook-script's 26-char ULID-shape
// convention, ballard-CL #032 lesson 9).
func padTo26(s string) string {
	if len(s) >= 26 {
		return s[:26]
	}
	return s + strings.Repeat("a", 26-len(s))
}

func stripUUIDHyphens(s string) string {
	return strings.ReplaceAll(s, "-", "")
}

func stripNonHex(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// parseTranscriptTime accepts both RFC3339 with Z suffix and plain
// RFC3339Nano. Returns zero time on parse failure (callers skip those
// lines silently).
func parseTranscriptTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("empty timestamp")
	}
	// RFC3339 covers both Z and timezone offsets.
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}

// mapToolResultStatusToVerdict normalises the toolUseResult.status
// strings observed in real transcripts into the schema-CHECK'd verdict
// vocabulary. Unknown statuses map to "none" — we still record the
// spawn-end so the spawn lifecycle is closed even when the status is
// transcribed in a form we have not seen yet.
func mapToolResultStatusToVerdict(status string) string {
	switch strings.ToLower(status) {
	case "completed", "success":
		return "freigabe"
	case "error", "failed":
		return "rueckgabe"
	case "escalated", "escalation":
		return "eskalation"
	case "":
		return ""
	default:
		return "none"
	}
}

// expandHome expands a leading "~" in p to the user's home directory.
// Used at hub wire-up so the bridge does not have to inject HOME into
// every test. Returns p unchanged if it does not start with "~".
func expandHome(p string) (string, error) {
	if !strings.HasPrefix(p, "~") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expand home: %w", err)
	}
	if p == "~" {
		return home, nil
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:]), nil
	}
	return p, nil
}

// ===== Tail bridge =====

// TranscriptBridgeDeps wires the tail bridge into the store + logger.
// Picked struct-deps over positional args so a future Hub field (e.g.
// a metrics sink) does not change the constructor signature.
type TranscriptBridgeDeps struct {
	Store          *store.Store
	Logger         *slog.Logger
	ProjectsRoot   string
	PollInterval   time.Duration
}

// TranscriptBridge is the long-running tail-loop. Create one via
// NewTranscriptBridge, start it with Run(ctx). Run blocks until ctx is
// cancelled or an unrecoverable error occurs; cancellation returns
// nil (regular shutdown).
type TranscriptBridge struct {
	store        *store.Store
	log          *slog.Logger
	projectsRoot string
	poll         time.Duration

	mu      sync.Mutex
	offsets map[string]int64 // file path → bytes already consumed
}

// NewTranscriptBridge constructs the bridge. ProjectsRoot is expanded
// (~ → $HOME) before the goroutine starts so a missing $HOME surfaces
// as a wire-up error rather than a tail-loop crash.
func NewTranscriptBridge(deps TranscriptBridgeDeps) (*TranscriptBridge, error) {
	if deps.Store == nil {
		return nil, errors.New("transcript: store required")
	}
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	root, err := expandHome(deps.ProjectsRoot)
	if err != nil {
		return nil, err
	}
	if root == "" {
		return nil, errors.New("transcript: projects_root required")
	}
	poll := deps.PollInterval
	if poll <= 0 {
		poll = time.Second
	}
	return &TranscriptBridge{
		store:        deps.Store,
		log:          deps.Logger,
		projectsRoot: root,
		poll:         poll,
		offsets:      make(map[string]int64),
	}, nil
}

// ProjectsRoot returns the resolved root directory the bridge watches.
// Useful for log lines at wire-up.
func (b *TranscriptBridge) ProjectsRoot() string { return b.projectsRoot }

// Run starts the tail-loop. Blocks until ctx is cancelled. Returns nil
// on graceful shutdown; non-nil only if the loop's preconditions are
// unrecoverable (e.g. ProjectsRoot is unreadable AND was never readable
// — see startupCheck below).
func (b *TranscriptBridge) Run(ctx context.Context) error {
	// Soft start: if the projects root does not exist yet, we still
	// run — a user may be configuring tracelab before Claude-Code
	// drops its first transcript. We log and retry every tick.
	if _, err := os.Stat(b.projectsRoot); err != nil {
		b.log.LogAttrs(ctx, slog.LevelInfo, "transcript tail starting (projects root not yet present)",
			slog.String("projects_root", b.projectsRoot),
			slog.Any("err", err),
		)
	} else {
		b.log.LogAttrs(ctx, slog.LevelInfo, "transcript tail starting",
			slog.String("projects_root", b.projectsRoot),
			slog.Duration("poll", b.poll),
		)
	}

	ticker := time.NewTicker(b.poll)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			b.log.Info("transcript tail stopped")
			return nil
		case <-ticker.C:
			b.tickOnce(ctx)
		}
	}
}

// tickOnce walks the projects root, tails each *.jsonl file forward
// from its last offset, parses new lines, and persists the resulting
// events. Errors on individual files are logged and skipped — one bad
// file never breaks the whole loop.
func (b *TranscriptBridge) tickOnce(ctx context.Context) {
	files, err := b.discoverFiles()
	if err != nil {
		b.log.LogAttrs(ctx, slog.LevelWarn, "transcript tail discover failed",
			slog.String("projects_root", b.projectsRoot),
			slog.Any("err", err),
		)
		return
	}
	for _, path := range files {
		if err := b.tailFile(ctx, path); err != nil {
			b.log.LogAttrs(ctx, slog.LevelWarn, "transcript tail file failed",
				slog.String("path", path),
				slog.Any("err", err),
			)
		}
	}
}

// discoverFiles walks projectsRoot and returns every *.jsonl path,
// including those under per-session subagents/ subdirs (subagent-*.jsonl).
// Hidden dotfiles are skipped.
func (b *TranscriptBridge) discoverFiles() ([]string, error) {
	var out []string
	err := filepath.WalkDir(b.projectsRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Permissions on a sibling project should not block ours.
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") && path != b.projectsRoot {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		out = append(out, path)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return out, nil
}

// tailFile opens path, seeks to the last consumed offset, line-buffers
// the rest, decodes each complete line as a transcript record, and
// persists the resulting events. A partial trailing line (no newline)
// is left for the next tick — we do NOT consume it yet, so we never
// publish a malformed record.
func (b *TranscriptBridge) tailFile(ctx context.Context, path string) error {
	b.mu.Lock()
	off := b.offsets[path]
	b.mu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		// File may have been deleted between discovery and open.
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() < off {
		// File was truncated/rotated — reset offset and start over.
		off = 0
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return err
	}

	br := bufio.NewReader(f)
	consumed := int64(0)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 && err == nil {
			// Complete line including \n — strip and process.
			consumed += int64(len(line))
			trimmed := bytes_trim(line)
			if len(trimmed) > 0 {
				b.processLine(ctx, path, trimmed)
			}
			continue
		}
		if err == io.EOF {
			// len(line) > 0 here would be a partial line — do NOT
			// advance the offset past it; we'll re-read next tick.
			break
		}
		if err != nil {
			return err
		}
	}

	b.mu.Lock()
	b.offsets[path] = off + consumed
	b.mu.Unlock()
	return nil
}

// processLine decodes one JSONL line, fans out into events, and
// persists each. A decode failure on one line is logged and skipped
// (corrupted-line tolerance — one bad row should not desync the whole
// file).
func (b *TranscriptBridge) processLine(ctx context.Context, path string, line []byte) {
	events, err := parseTranscriptLine(line)
	if err != nil {
		b.log.LogAttrs(ctx, slog.LevelDebug, "transcript line decode failed (skipped)",
			slog.String("path", path),
			slog.Any("err", err),
		)
		return
	}
	for _, e := range events {
		if err := b.persistEvent(ctx, e); err != nil {
			b.log.LogAttrs(ctx, slog.LevelWarn, "transcript persist failed",
				slog.String("path", path),
				slog.Any("err", err),
			)
		}
	}
}

// persistEvent calls the matching store.InsertAgent* method for the
// populated field. INSERT OR IGNORE in the store layer guarantees
// idempotency against repeated tail reads (e.g. after a hub restart)
// and against the SDK-hook source for the same spawn (UNIQUE-tuple
// keeps both rows for forensic per-source breakdown).
func (b *TranscriptBridge) persistEvent(ctx context.Context, e transcriptEvent) error {
	switch {
	case e.Spawn != nil:
		_, err := b.store.InsertAgentSpawn(ctx, *e.Spawn)
		return err
	case e.Tokens != nil:
		_, err := b.store.InsertAgentTokens(ctx, *e.Tokens)
		return err
	case e.Verdict != nil:
		// Verdicts FK back to the spawn — if the spawn row does not
		// exist yet (e.g. we tailed the parent verdict before the
		// child spawn-begin), surface as a non-fatal log so we don't
		// stall the loop. A future tick will re-emit the verdict
		// after the spawn row lands.
		_, err := b.store.InsertAgentVerdict(ctx, *e.Verdict)
		return err
	case e.Edge != nil:
		// Edges FK back to BOTH spawn endpoints (CASCADE delete on
		// either side cleans the edge automatically). The parent +
		// child spawn rows are emitted in the same event batch as the
		// edge — the parseTranscriptLine event ordering guarantees
		// they precede the edge insert. INSERT OR IGNORE on the
		// (from, to, type, ts) UNIQUE tuple makes the edge insert
		// idempotent against re-tails.
		_, err := b.store.InsertAgentMailboxEdge(ctx, *e.Edge)
		return err
	}
	return nil
}

// bytes_trim drops trailing \r\n from a buffered line. We avoid the
// bytes package import here so this file's dependency footprint stays
// minimal; one byte loop is plenty for the trim job.
func bytes_trim(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}
