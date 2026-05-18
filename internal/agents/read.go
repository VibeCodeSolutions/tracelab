// Phase 2d S4 — read-surface for the dashboard "Agents" tab.
//
// Four HTTP read endpoints sit alongside the existing POST /agents/ingest:
//
//   - GET /agents/sessions             — paginated newest-first list
//   - GET /agents/tree/{spawn_id}      — BFS-ordered subtree (root + descendants)
//   - GET /agents/tokens?spawn_id=…    — token rows + aggregated counts
//   - GET /agents/verdicts?spawn_id=…  — verdict rows
//
// Wireup posture: read endpoints are NOT mounted through internal/http
// (the Phase-2d-S4 cross-check-scope brief explicitly forbids touching
// internal/http/). Instead, cmd/hub/main.go wraps the chi handler with
// a tiny prefix-dispatcher (AgentsReadMux below) that intercepts the
// four read-paths before they reach the chi router. The dispatcher
// applies its own bearer-auth guard so the auth posture matches the
// rest of the JSON read surface (/sessions, /events, /crashes).
//
// 0 Bytes Diff in internal/http/ — confirmed by the cross-check-scope
// audit (#035 worker self-check, post-implementation).
//
// Wire shape — every endpoint returns application/json; the response
// envelope mirrors the existing list-reads in internal/http/handlers.go:
// the top-level key names the resource ("spawns", "tree", "tokens",
// "verdicts") so a future field-addition (e.g. paging cursor) stays
// non-breaking.

package agents

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/VibeCodeSolutions/tracelab/internal/store"
)

// AgentsReadLimitDefault is the per-request cap used by GET /agents/sessions
// when no ?limit= is supplied. 50 mirrors the sessions / crashes endpoints.
const AgentsReadLimitDefault = 50

// AgentsReadLimitMax caps the user-controllable ?limit= override —
// belt-and-braces against pathological URLs.
const AgentsReadLimitMax = 500

// SpawnWire is the JSON shape returned by /agents/sessions and
// embedded in /agents/tree. Time fields are unix-nano (consistent with
// the ingest wire shape and the sessions/events/crashes endpoints).
type SpawnWire struct {
	ID         string `json:"id"`
	ParentID   string `json:"parent_id,omitempty"`
	Skill      string `json:"skill"`
	StartedAt  int64  `json:"started_at"`
	EndedAt    *int64 `json:"ended_at,omitempty"`
	Project    string `json:"project"`
	SessionRef string `json:"session_ref,omitempty"`
}

// spawnsListResp is the response body for GET /agents/sessions.
type spawnsListResp struct {
	Spawns []SpawnWire `json:"spawns"`
	Total  int64       `json:"total"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
}

// TreeNodeWire embeds SpawnWire with a Depth field so the consumer
// (dashboard renderer, MCP tool) can indent without re-walking the
// parent chain. The slice returned in TreeResp is BFS-ordered (root
// first, then children, then grand-children).
type TreeNodeWire struct {
	SpawnWire
	Depth int `json:"depth"`
}

// treeResp is the response body for GET /agents/tree/{spawn_id}.
type treeResp struct {
	Root  string         `json:"root"`
	Nodes []TreeNodeWire `json:"nodes"`
}

// TokenRowWire is one agent_tokens row in the JSON wire shape. ts is
// unix-nano. Source is preserved so the consumer can break down by
// source ("sdk-hook", "transcript", "mcp-push").
type TokenRowWire struct {
	ID           int64  `json:"id"`
	SpawnID      string `json:"spawn_id"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CacheRead    int64  `json:"cache_read"`
	CacheWrite   int64  `json:"cache_write"`
	TS           int64  `json:"ts"`
	Source       string `json:"source"`
}

// TokenTotals is the aggregated-counts side-shape returned alongside
// the raw rows. "total" is the sum across all sources; "by_source"
// maps each canonical source string to its own aggregate. Sources
// with zero rows simply don't appear in the map (no need to render a
// trailing "[mcp-push: 0/0]" pill when nothing was ever pushed from
// that path).
type TokenTotals struct {
	InputTokens  int64                     `json:"input_tokens"`
	OutputTokens int64                     `json:"output_tokens"`
	CacheRead    int64                     `json:"cache_read"`
	CacheWrite   int64                     `json:"cache_write"`
	BySource     map[string]TokenSourceSum `json:"by_source"`
}

// TokenSourceSum is the per-source aggregate inside TokenTotals.
type TokenSourceSum struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	CacheRead    int64 `json:"cache_read"`
	CacheWrite   int64 `json:"cache_write"`
	RowCount     int64 `json:"row_count"`
}

// tokensResp is the response body for GET /agents/tokens?spawn_id=…
type tokensResp struct {
	SpawnID string         `json:"spawn_id"`
	Tokens  []TokenRowWire `json:"tokens"`
	Totals  TokenTotals    `json:"totals"`
}

// EdgeRowWire is one agent_mailbox_edges row in the JSON wire shape.
// ts is unix-nano; the four FK + enum fields are eagerly populated
// (the schema rejects nullable columns on this table).
//
// Phase 2d S5 — pairs with /agents/edges.
type EdgeRowWire struct {
	ID          int64  `json:"id"`
	FromSpawnID string `json:"from_spawn_id"`
	ToSpawnID   string `json:"to_spawn_id"`
	EdgeType    string `json:"edge_type"`
	TS          int64  `json:"ts"`
}

// edgesResp is the response body for GET /agents/edges?spawn_id=…
// Two slices — `in` (rows pointing AT the spawn) and `out` (rows pointing
// AWAY from the spawn). The wire shape is symmetric so the dashboard can
// render both columns without per-row direction inference.
type edgesResp struct {
	SpawnID string        `json:"spawn_id"`
	In      []EdgeRowWire `json:"in"`
	Out     []EdgeRowWire `json:"out"`
}

// VerdictRowWire is one agent_verdicts row in the JSON wire shape.
// ts is unix-nano; lerneffekt_md is omitted when the source row
// carried NULL or an empty string.
type VerdictRowWire struct {
	ID           int64  `json:"id"`
	SpawnID      string `json:"spawn_id"`
	Verdict      string `json:"verdict"`
	LerneffektMD string `json:"lerneffekt_md,omitempty"`
	TS           int64  `json:"ts"`
}

// verdictsResp is the response body for GET /agents/verdicts?spawn_id=…
type verdictsResp struct {
	SpawnID  string           `json:"spawn_id"`
	Verdicts []VerdictRowWire `json:"verdicts"`
}

// AgentsReadMux returns an http.Handler that serves the four /agents/*
// read endpoints with the same bearer-auth posture as the rest of the
// JSON read surface (/sessions, /events, /crashes). The handler is
// designed to be slotted in front of the main chi router by
// cmd/hub/main.go — anything outside the prefix-set is passed through
// to `next`.
//
// Prefix-set:
//   - GET /agents/sessions
//   - GET /agents/tree/{spawn_id}
//   - GET /agents/tokens
//   - GET /agents/verdicts
//   - GET /agents/edges       (Phase 2d S5)
//
// POST /agents/ingest continues to be served by the chi router (it sits
// inside the ingest handler's existing bearer-protected sub-group);
// the dispatcher recognises the method+path combination and passes
// through to `next` for that case.
//
// authToken must be the same bearer secret the rest of the JSON
// endpoints use. An empty token is rejected at construction (mirrors
// internal/http.New).
func (h *Handler) AgentsReadMux(authToken string, next http.Handler) http.Handler {
	if authToken == "" {
		// Defensive: returning a no-op would silently bypass auth on
		// the read endpoints. Panic loudly so production wireup catches
		// the config bug at start-up rather than first-request.
		panic("agents.AgentsReadMux: authToken must be non-empty")
	}
	expected := []byte(authToken)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Pass-through fast-path: anything not under /agents/ or any
		// non-GET on the read prefixes routes to the next handler.
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/agents/") {
			next.ServeHTTP(w, r)
			return
		}

		path := r.URL.Path
		var dispatch func(http.ResponseWriter, *http.Request)
		switch {
		case path == "/agents/sessions":
			dispatch = h.readSessions
		case strings.HasPrefix(path, "/agents/tree/"):
			dispatch = h.readTree
		case path == "/agents/tokens":
			dispatch = h.readTokens
		case path == "/agents/verdicts":
			dispatch = h.readVerdicts
		case path == "/agents/edges":
			dispatch = h.readEdges
		default:
			// Unknown /agents/<x> path — let the chi router handle it
			// (currently only POST /agents/ingest is registered there;
			// any GET we don't recognise will land on chi's 404).
			next.ServeHTTP(w, r)
			return
		}

		// Bearer-auth identical to internal/http.bearerAuth (constant-time
		// compare, 401 for both missing and mismatched). Replicated here
		// rather than exporting bearerAuth so internal/http stays 0-bytes
		// touched (cross-check-scope discipline).
		hdr := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(hdr, prefix) {
			writeJSON(w, http.StatusUnauthorized, errorBody("unauthorized"))
			return
		}
		got := []byte(strings.TrimPrefix(hdr, prefix))
		if subtle.ConstantTimeCompare(got, expected) != 1 {
			writeJSON(w, http.StatusUnauthorized, errorBody("unauthorized"))
			return
		}

		dispatch(w, r)
	})
}

// readSessions handles GET /agents/sessions[?limit=&offset=&project=&session_ref=].
func (h *Handler) readSessions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := parseLimitParam(q.Get("limit"), AgentsReadLimitDefault, AgentsReadLimitMax)
	offset := parseOffsetParam(q.Get("offset"))
	project := strings.TrimSpace(q.Get("project"))
	sessionRef := strings.TrimSpace(q.Get("session_ref"))
	if len(project) > 128 {
		project = project[:128]
	}
	if len(sessionRef) > 128 {
		sessionRef = sessionRef[:128]
	}

	ctx := r.Context()
	total, err := h.store.CountAgentSpawns(ctx, project, sessionRef)
	if err != nil {
		h.logReadError(ctx, "agents readSessions count", err)
		writeJSON(w, http.StatusInternalServerError, errorBody("count failed"))
		return
	}
	rows, err := h.store.ListAgentSpawns(ctx, store.ListAgentSpawnsOpts{
		Limit:            limit,
		Offset:           offset,
		FilterProject:    project,
		FilterSessionRef: sessionRef,
	})
	if err != nil {
		h.logReadError(ctx, "agents readSessions list", err)
		writeJSON(w, http.StatusInternalServerError, errorBody("list failed"))
		return
	}

	wires := make([]SpawnWire, 0, len(rows))
	for _, r := range rows {
		wires = append(wires, spawnRowToWire(r))
	}
	writeJSON(w, http.StatusOK, spawnsListResp{
		Spawns: wires,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

// readTree handles GET /agents/tree/{spawn_id}. Unknown id → 404.
func (h *Handler) readTree(w http.ResponseWriter, r *http.Request) {
	rootID := strings.TrimPrefix(r.URL.Path, "/agents/tree/")
	if rootID == "" || strings.ContainsAny(rootID, "/?#") {
		writeJSON(w, http.StatusBadRequest, errorBody("spawn_id required"))
		return
	}
	if !isValidSpawnIDFormat(rootID) {
		writeJSON(w, http.StatusBadRequest, errorBody("invalid spawn_id"))
		return
	}

	ctx := r.Context()
	rows, err := h.store.ListAgentSpawnTree(ctx, rootID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorBody("spawn not found"))
			return
		}
		h.logReadError(ctx, "agents readTree", err)
		writeJSON(w, http.StatusInternalServerError, errorBody("tree query failed"))
		return
	}

	// Compute per-node depth from the parent chain. The slice is
	// already BFS-ordered (root first), so a single forward pass with
	// a depth-by-id map populates each child's depth before we hit it.
	depthByID := map[string]int{rootID: 0}
	nodes := make([]TreeNodeWire, 0, len(rows))
	for _, r := range rows {
		wire := spawnRowToWire(r)
		d := 0
		if r.ID == rootID {
			d = 0
		} else if r.ParentID != nil {
			if parentDepth, ok := depthByID[*r.ParentID]; ok {
				d = parentDepth + 1
			}
		}
		depthByID[r.ID] = d
		nodes = append(nodes, TreeNodeWire{SpawnWire: wire, Depth: d})
	}
	writeJSON(w, http.StatusOK, treeResp{Root: rootID, Nodes: nodes})
}

// readTokens handles GET /agents/tokens?spawn_id=…
func (h *Handler) readTokens(w http.ResponseWriter, r *http.Request) {
	spawnID := strings.TrimSpace(r.URL.Query().Get("spawn_id"))
	if spawnID == "" {
		writeJSON(w, http.StatusBadRequest, errorBody("spawn_id required"))
		return
	}
	if !isValidSpawnIDFormat(spawnID) {
		writeJSON(w, http.StatusBadRequest, errorBody("invalid spawn_id"))
		return
	}

	ctx := r.Context()
	rows, err := h.store.AgentTokensBySpawn(ctx, spawnID)
	if err != nil {
		h.logReadError(ctx, "agents readTokens", err)
		writeJSON(w, http.StatusInternalServerError, errorBody("tokens query failed"))
		return
	}

	wires := make([]TokenRowWire, 0, len(rows))
	totals := TokenTotals{BySource: map[string]TokenSourceSum{}}
	for _, t := range rows {
		wires = append(wires, TokenRowWire{
			ID:           t.ID,
			SpawnID:      t.SpawnID,
			InputTokens:  t.InputTokens,
			OutputTokens: t.OutputTokens,
			CacheRead:    t.CacheRead,
			CacheWrite:   t.CacheWrite,
			TS:           t.TS.UnixNano(),
			Source:       t.Source,
		})
		totals.InputTokens += t.InputTokens
		totals.OutputTokens += t.OutputTokens
		totals.CacheRead += t.CacheRead
		totals.CacheWrite += t.CacheWrite
		bs := totals.BySource[t.Source]
		bs.InputTokens += t.InputTokens
		bs.OutputTokens += t.OutputTokens
		bs.CacheRead += t.CacheRead
		bs.CacheWrite += t.CacheWrite
		bs.RowCount++
		totals.BySource[t.Source] = bs
	}
	writeJSON(w, http.StatusOK, tokensResp{
		SpawnID: spawnID,
		Tokens:  wires,
		Totals:  totals,
	})
}

// readVerdicts handles GET /agents/verdicts?spawn_id=…
func (h *Handler) readVerdicts(w http.ResponseWriter, r *http.Request) {
	spawnID := strings.TrimSpace(r.URL.Query().Get("spawn_id"))
	if spawnID == "" {
		writeJSON(w, http.StatusBadRequest, errorBody("spawn_id required"))
		return
	}
	if !isValidSpawnIDFormat(spawnID) {
		writeJSON(w, http.StatusBadRequest, errorBody("invalid spawn_id"))
		return
	}

	ctx := r.Context()
	rows, err := h.store.AgentVerdictsBySpawn(ctx, spawnID)
	if err != nil {
		h.logReadError(ctx, "agents readVerdicts", err)
		writeJSON(w, http.StatusInternalServerError, errorBody("verdicts query failed"))
		return
	}

	wires := make([]VerdictRowWire, 0, len(rows))
	for _, v := range rows {
		wires = append(wires, VerdictRowWire{
			ID:           v.ID,
			SpawnID:      v.SpawnID,
			Verdict:      v.Verdict,
			LerneffektMD: v.LerneffektMD,
			TS:           v.TS.UnixNano(),
		})
	}
	writeJSON(w, http.StatusOK, verdictsResp{
		SpawnID:  spawnID,
		Verdicts: wires,
	})
}

// readEdges handles GET /agents/edges?spawn_id=…
//
// Returns the in-edges (rows whose to_spawn_id == spawn_id) and the
// out-edges (rows whose from_spawn_id == spawn_id) as two parallel
// slices. Empty slices (not null) are emitted when the spawn has no
// edges in a direction so consumers can iterate without nil-guards.
//
// Phase 2d S5 — mailbox-edge read surface. Cross-references to live-tail
// events (events.id ↔ spawn) are an open architecture question (see
// ADR-014 — Proposed, status awaits Admin/Chakotay decision); this
// endpoint deliberately ships in the simpler shape (in/out only) so the
// data is consumable today, and a future field-addition (e.g.
// `event_refs`) stays non-breaking on top.
func (h *Handler) readEdges(w http.ResponseWriter, r *http.Request) {
	spawnID := strings.TrimSpace(r.URL.Query().Get("spawn_id"))
	if spawnID == "" {
		writeJSON(w, http.StatusBadRequest, errorBody("spawn_id required"))
		return
	}
	if !isValidSpawnIDFormat(spawnID) {
		writeJSON(w, http.StatusBadRequest, errorBody("invalid spawn_id"))
		return
	}

	ctx := r.Context()
	inRows, outRows, err := h.store.AgentEdgesForSpawn(ctx, spawnID)
	if err != nil {
		h.logReadError(ctx, "agents readEdges", err)
		writeJSON(w, http.StatusInternalServerError, errorBody("edges query failed"))
		return
	}

	in := make([]EdgeRowWire, 0, len(inRows))
	for _, e := range inRows {
		in = append(in, edgeRowToWire(e))
	}
	out := make([]EdgeRowWire, 0, len(outRows))
	for _, e := range outRows {
		out = append(out, edgeRowToWire(e))
	}
	writeJSON(w, http.StatusOK, edgesResp{
		SpawnID: spawnID,
		In:      in,
		Out:     out,
	})
}

// edgeRowToWire converts the store-layer AgentMailboxEdgeRow to the
// public EdgeRowWire JSON shape.
func edgeRowToWire(e store.AgentMailboxEdgeRow) EdgeRowWire {
	return EdgeRowWire{
		ID:          e.ID,
		FromSpawnID: e.FromSpawnID,
		ToSpawnID:   e.ToSpawnID,
		EdgeType:    e.EdgeType,
		TS:          e.TS.UnixNano(),
	}
}

// spawnRowToWire converts the store-layer AgentSpawnRow to the public
// SpawnWire JSON shape. Nil pointers collapse to "" so the JSON output
// stays consistent with the omitempty tags.
func spawnRowToWire(r store.AgentSpawnRow) SpawnWire {
	w := SpawnWire{
		ID:        r.ID,
		Skill:     r.Skill,
		StartedAt: r.StartedAt.UnixNano(),
		Project:   r.Project,
	}
	if r.ParentID != nil {
		w.ParentID = *r.ParentID
	}
	if r.SessionRef != nil {
		w.SessionRef = *r.SessionRef
	}
	if r.EndedAt != nil {
		n := r.EndedAt.UnixNano()
		w.EndedAt = &n
	}
	return w
}

// parseLimitParam validates a ?limit= query param against a default
// and a max ceiling. Empty / unparseable / out-of-range values fall
// back to the default (we never 400 on a bad limit — the surface is
// browser-loaded by the dashboard, and a 400 would render mid-swap).
func parseLimitParam(raw string, def, max int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

// parseOffsetParam validates a ?offset= query param. Empty / negative
// / unparseable values collapse to 0.
func parseOffsetParam(raw string) int {
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// isValidSpawnIDFormat is the second defence-in-depth layer for the
// path/query parameter readers. Accepts only single-segment ids
// matching [A-Za-z0-9_-]. The store uses a 26-char ULID-shaped id by
// convention but the charset is intentionally wider so a future id-
// scheme migration doesn't break this handler — the negative space
// (".." / "/" / whitespace / control chars / shell-meta) is what we
// actually reject. Length is bounded so a 4 MB curl-injection can't
// blow up the scan; 128 mirrors the filter caps in the dashboard
// session/crash handlers.
func isValidSpawnIDFormat(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c == '_' || c == '-':
		default:
			return false
		}
	}
	return true
}

// logReadError is a tiny wrapper around the package logger that keeps
// the error-log shape identical across the four read endpoints.
// Centralising it (rather than inlining h.log.LogAttrs in each
// handler) shrinks the surface a code review needs to glance over.
func (h *Handler) logReadError(ctx context.Context, msg string, err error) {
	h.log.LogAttrs(ctx, slog.LevelError, msg, slog.Any("error", err))
}
