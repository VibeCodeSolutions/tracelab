// agents.go — Agents-tab handler (Phase 2d S4).
//
// Renders the dashboard's fourth tab body with three side-by-side
// surfaces driven by the Phase-2d agent-observability data:
//
//   - Skill-Spawn-Tree:  hierarchical list of agent spawns ordered by
//                        parent → children (BFS), depth visualised
//                        through indentation and a tree-glyph prefix.
//   - Token-Usage:       per-spawn aggregated input/output/cache counts
//                        with per-source breakdown (sdk-hook / transcript
//                        / mcp-push) when multiple sources reported.
//   - Verdict + Lerneffekt: latest verdict per spawn + the Lerneffekt
//                        markdown note, rendered as plain text (the
//                        full markdown render is out-of-scope for S4 —
//                        the dashboard surfaces the text verbatim).
//
// The tab body is htmx-swappable (no <html> envelope) so the rest of
// the dashboard plumbing — LayoutHandler wrapping it, TabHandler
// serving it directly — works identically to the sessions / crashes
// tabs.
//
// Mailbox-edges and cross-references to the live-tail event stream are
// explicitly S5 scope (the Phase-2d sammel-gate) — not surfaced here.

package dashboard

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/VibeCodeSolutions/tracelab/internal/store"
)

// AgentsPageSize is the default page size for the agents list view.
// 50 mirrors the sessions / crashes default. Exposed as a package
// variable so tests can shrink it.
var AgentsPageSize = 50

// AgentsPageSizeMax caps the user-controllable ?limit= override.
const AgentsPageSizeMax = 500

// agentsViewParams is the validated query-param bundle for the tab.
// project / session_ref are passed through as exact-match filters; the
// dashboard does NOT expose substring search here (the typical filter
// granularity for skill-spawn-trees is "give me the runs for project
// foo" rather than "find anything containing foo").
type agentsViewParams struct {
	Project    string
	SessionRef string
	Page       int // 1-based
	Limit      int
}

// agentSpawnRow is the per-row dot value rendered in the spawns table.
// All time strings are pre-formatted so the template stays HTML-only.
type agentSpawnRow struct {
	ID               string
	ParentID         string
	Skill            string
	Project          string
	SessionRef       string
	StartedAtHuman   string
	EndedAtHuman     string // empty when ended_at is NULL (still running)
	Depth            int    // BFS depth from the root of the spawn's tree
	IndentMarker     string // tree-glyph prefix ("├─" / "└─" etc.)
	TokensInput      int64
	TokensOutput     int64
	TokensCacheRead  int64
	TokensCacheWrite int64
	TokensRowCount   int64
	BySource         []agentTokenSourceRow // per-source breakdown if multi-source
	Verdict          string                // latest verdict's text, e.g. "freigabe"
	VerdictHuman     string                // capitalised display form
	LerneffektMD     string                // raw markdown (rendered as plain text in S4)
}

// agentTokenSourceRow is one per-source aggregate inside the spawn row.
type agentTokenSourceRow struct {
	Source       string // "sdk-hook" | "transcript" | "mcp-push"
	InputTokens  int64
	OutputTokens int64
	CacheRead    int64
	CacheWrite   int64
	RowCount     int64
}

// agentsViewData is the dot value passed to tab_agents.gohtml.
type agentsViewData struct {
	Spawns    []agentSpawnRow
	Project   string
	SessionID string
	Page      int
	Limit     int
	Total     int64
	PageCount int
	HasPrev   bool
	HasNext   bool
	PrevURL   string
	NextURL   string
	Empty     bool
	// ErrorMsg is non-empty when the tab body needs to surface a
	// store-side failure inline (e.g. tokens lookup failed for one
	// spawn). The template renders a tl-error-card alongside the
	// data so the operator sees both the partial results and the
	// failure.
	ErrorMsg string
}

// AgentsHandler is the htmx-swap endpoint at GET /dashboard/tab/agents.
// Renders the tab body (no envelope) so the response is droppable into
// #dashboard-content with hx-swap=innerHTML.
func (h *Handler) AgentsHandler(w http.ResponseWriter, r *http.Request) {
	body, err := h.renderAgentsBody(r)
	if err != nil {
		h.log.LogAttrs(r.Context(), slog.LevelError,
			"dashboard agents: render failed",
			slog.Any("error", err))
		http.Error(w, "internal dashboard error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body)
}

// renderAgentsBody is the data-fetch + template-execute core. Shared
// between AgentsHandler (htmx swap) and LayoutHandler (full-page
// render with the body slotted into the layout shell).
func (h *Handler) renderAgentsBody(r *http.Request) ([]byte, error) {
	if h.store == nil {
		return h.renderEmptyAgentsBody()
	}
	params := parseAgentsParams(r.URL.Query())
	ctx := r.Context()

	total, err := h.store.CountAgentSpawns(ctx, params.Project, params.SessionRef)
	if err != nil {
		return nil, fmt.Errorf("count agent spawns: %w", err)
	}

	offset := (params.Page - 1) * params.Limit
	if total > 0 && int64(offset) >= total {
		maxPage := int((total + int64(params.Limit) - 1) / int64(params.Limit))
		if maxPage < 1 {
			maxPage = 1
		}
		params.Page = maxPage
		offset = (params.Page - 1) * params.Limit
	}

	rows, err := h.store.ListAgentSpawns(ctx, store.ListAgentSpawnsOpts{
		Limit:            params.Limit,
		Offset:           offset,
		FilterProject:    params.Project,
		FilterSessionRef: params.SessionRef,
	})
	if err != nil {
		return nil, fmt.Errorf("list agent spawns: %w", err)
	}

	view, fetchErr := h.buildAgentsViewData(ctx, rows, total, params)
	if fetchErr != "" {
		// Don't abort — render what we have, with a visible error
		// card so the operator sees both the partial table and the
		// failure that produced it.
		view.ErrorMsg = fetchErr
	}

	var buf bytes.Buffer
	if err := h.agentsTpl.Execute(&buf, view); err != nil {
		return nil, fmt.Errorf("execute agents tab: %w", err)
	}
	return buf.Bytes(), nil
}

// renderEmptyAgentsBody is the skeleton-only fallback (used when no
// store is wired — test contexts). Returns a well-formed but empty
// view so the rendered HTML still carries the tl-tab-panel envelope.
func (h *Handler) renderEmptyAgentsBody() ([]byte, error) {
	view := agentsViewData{
		Spawns:    nil,
		Page:      1,
		Limit:     AgentsPageSize,
		Total:     0,
		PageCount: 1,
		Empty:     true,
	}
	var buf bytes.Buffer
	if err := h.agentsTpl.Execute(&buf, view); err != nil {
		return nil, fmt.Errorf("execute empty agents tab: %w", err)
	}
	return buf.Bytes(), nil
}

// buildAgentsViewData assembles the typed dot value from raw store
// rows. Tokens + verdicts are fetched per-spawn — the dataset is
// O(50) per page, so a per-row roundtrip is fine here; if a future
// page size grows substantially this would justify a batched query.
//
// Returns (view, "" ) on full success, or (partial view, errMsg) when
// a sub-query failed — the template renders the partial table plus an
// error card.
func (h *Handler) buildAgentsViewData(ctx context.Context, rows []store.AgentSpawnRow, total int64, p agentsViewParams) (agentsViewData, string) {
	out := agentsViewData{
		Spawns:    make([]agentSpawnRow, 0, len(rows)),
		Project:   p.Project,
		SessionID: p.SessionRef,
		Page:      p.Page,
		Limit:     p.Limit,
		Total:     total,
		Empty:     len(rows) == 0,
	}

	// Build a depth map from parent_id pointers within the visible
	// page only. Spawns whose parent is NOT on the current page get
	// depth 0 (treated as a virtual root for layout purposes).
	depth := computeVisiblePageDepth(rows)

	var errMsg string
	for _, r := range rows {
		row := agentSpawnRow{
			ID:             r.ID,
			Skill:          r.Skill,
			Project:        r.Project,
			StartedAtHuman: formatHumanTime(r.StartedAt),
			Depth:          depth[r.ID],
		}
		if r.ParentID != nil {
			row.ParentID = *r.ParentID
		}
		if r.SessionRef != nil {
			row.SessionRef = *r.SessionRef
		}
		if r.EndedAt != nil {
			row.EndedAtHuman = formatHumanTime(*r.EndedAt)
		}
		row.IndentMarker = treeGlyph(row.Depth, row.ParentID != "")

		tokens, err := h.store.AgentTokensBySpawn(ctx, r.ID)
		if err != nil {
			if errMsg == "" {
				errMsg = fmt.Sprintf("tokens lookup failed for %s: %v", r.ID, err)
			}
		} else {
			aggregateTokensIntoRow(&row, tokens)
		}

		verdicts, err := h.store.AgentVerdictsBySpawn(ctx, r.ID)
		if err != nil {
			if errMsg == "" {
				errMsg = fmt.Sprintf("verdicts lookup failed for %s: %v", r.ID, err)
			}
		} else if len(verdicts) > 0 {
			// Use the latest verdict (last by ts ASC — see store-layer
			// ORDER BY). If multiple verdicts coexist (e.g. an "auflagen"
			// followed by a later "freigabe"), the most recent wins.
			latest := verdicts[len(verdicts)-1]
			row.Verdict = latest.Verdict
			row.VerdictHuman = formatVerdictLabel(latest.Verdict)
			row.LerneffektMD = latest.LerneffektMD
		}

		out.Spawns = append(out.Spawns, row)
	}

	if total > 0 {
		out.PageCount = int((total + int64(p.Limit) - 1) / int64(p.Limit))
	} else {
		out.PageCount = 1
	}
	out.HasPrev = p.Page > 1
	out.HasNext = p.Page < out.PageCount
	out.PrevURL = buildAgentsURL(p, p.Page-1)
	out.NextURL = buildAgentsURL(p, p.Page+1)

	return out, errMsg
}

// aggregateTokensIntoRow sums the per-row token counts into the
// agentSpawnRow's flat fields and the per-source breakdown slice.
// Sources with zero rows don't appear in the breakdown.
func aggregateTokensIntoRow(row *agentSpawnRow, tokens []store.AgentTokenRow) {
	bySource := map[string]agentTokenSourceRow{}
	for _, t := range tokens {
		row.TokensInput += t.InputTokens
		row.TokensOutput += t.OutputTokens
		row.TokensCacheRead += t.CacheRead
		row.TokensCacheWrite += t.CacheWrite
		row.TokensRowCount++
		s := bySource[t.Source]
		s.Source = t.Source
		s.InputTokens += t.InputTokens
		s.OutputTokens += t.OutputTokens
		s.CacheRead += t.CacheRead
		s.CacheWrite += t.CacheWrite
		s.RowCount++
		bySource[t.Source] = s
	}
	// Render the breakdown in a stable order (canonical source list)
	// so the rendered HTML diff-compares cleanly in tests and the
	// operator's eye learns the column positions.
	for _, src := range []string{"sdk-hook", "transcript", "mcp-push"} {
		if s, ok := bySource[src]; ok {
			row.BySource = append(row.BySource, s)
		}
	}
}

// computeVisiblePageDepth assigns each row on the current page a
// depth value based on parent_id pointers visible on this page. If a
// row's parent is not on the page (i.e. it was rendered on a previous
// page or filtered out), the row's depth is 0 — it shows up as a
// virtual root for the page's visual hierarchy. This is intentional:
// the agents tab is a paged list view, not a "fetch the full tree"
// view (that's GET /agents/tree/{id}, surfaced via the MCP tool).
func computeVisiblePageDepth(rows []store.AgentSpawnRow) map[string]int {
	depth := make(map[string]int, len(rows))
	// First pass: mark every row as depth-0 (default).
	for _, r := range rows {
		depth[r.ID] = 0
	}
	// Iterate until no more depths change; bounded by len(rows) since
	// each pass increases at least one depth by 1, and depth is capped
	// at len(rows). In practice 1-3 passes suffice.
	for pass := 0; pass < len(rows); pass++ {
		changed := false
		for _, r := range rows {
			if r.ParentID == nil {
				continue
			}
			if pd, ok := depth[*r.ParentID]; ok && depth[r.ID] != pd+1 {
				if depth[r.ID] < pd+1 {
					depth[r.ID] = pd + 1
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}
	return depth
}

// treeGlyph returns the per-depth indent prefix. A root row (depth=0)
// has no glyph; deeper rows are prefixed with ASCII tree marks plus
// non-breaking spaces. We deliberately use plain ASCII rather than
// box-drawing Unicode here so terminal-side `curl` previews stay
// readable.
func treeGlyph(depth int, hasParent bool) string {
	if depth == 0 {
		return ""
	}
	// Two NBSPs per level of indent + "└─ " for the leaf marker.
	// The template renders &nbsp; explicitly so the leading whitespace
	// survives HTML normalisation.
	if !hasParent {
		return ""
	}
	return strings.Repeat("  ", depth-1) + "└─ "
}

// formatVerdictLabel turns the canonical verdict-string into a
// display-cased label. Mirrors the multilingual posture of the rest
// of the dashboard (German verdict names, English-y column heads).
func formatVerdictLabel(v string) string {
	switch v {
	case "freigabe":
		return "Freigabe"
	case "auflagen":
		return "Auflagen"
	case "rueckgabe":
		return "Rückgabe"
	case "eskalation":
		return "Eskalation"
	case "none":
		return "—"
	default:
		return v
	}
}

// parseAgentsParams validates url.Values into the typed param bundle.
// Unknown values are clamped to defaults; the handler never 400s the
// user (browser navigation surface, a 400 renders as a raw error page
// mid-swap).
func parseAgentsParams(q url.Values) agentsViewParams {
	p := agentsViewParams{
		Project:    strings.TrimSpace(q.Get("project")),
		SessionRef: strings.TrimSpace(q.Get("session")),
		Page:       1,
		Limit:      AgentsPageSize,
	}
	if raw := q.Get("page"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 1 {
			p.Page = n
		}
	}
	if raw := q.Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			if n > AgentsPageSizeMax {
				n = AgentsPageSizeMax
			}
			p.Limit = n
		}
	}
	if len(p.Project) > 128 {
		p.Project = p.Project[:128]
	}
	if len(p.SessionRef) > 128 {
		p.SessionRef = p.SessionRef[:128]
	}
	return p
}

// buildAgentsURL constructs the /dashboard/tab/agents URL with the
// current filters + page. Used for htmx prev/next navigation.
func buildAgentsURL(p agentsViewParams, page int) string {
	if page < 1 {
		page = 1
	}
	q := []string{
		"page=" + strconv.Itoa(page),
		"limit=" + strconv.Itoa(p.Limit),
	}
	if p.Project != "" {
		q = append(q, "project="+url.QueryEscape(p.Project))
	}
	if p.SessionRef != "" {
		q = append(q, "session="+url.QueryEscape(p.SessionRef))
	}
	return "/dashboard/tab/agents?" + strings.Join(q, "&")
}
