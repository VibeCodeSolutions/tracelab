// crashes.go — Crash-inspector tab handlers (Phase 2c S4).
//
// Two surfaces, mirroring the S3 sessions pattern:
//
//   - CrashesHandler / renderCrashesBody — the list view at
//     /dashboard/tab/crashes (htmx swap) and the body slot embedded in
//     the full-layout render at /dashboard?tab=crashes. Parses
//     sort/session-filter/page query params, queries
//     store.ListCrashes + store.CountCrashes +
//     store.ListSessionIDsWithCrashes, renders tab_crashes.gohtml.
//   - CrashDetailHandler / renderCrashDetailBody — the detail view at
//     /dashboard/tab/crashes/{id} (htmx swap). Looks the crash up via
//     store.CrashByID (404 on unknown id), renders the full stacktrace
//     in tab_crash_detail.gohtml.
//
// Both write body-only HTML (no <html> envelope) so htmx can drop the
// response into #dashboard-content with hx-swap=innerHTML.
//
// The `crashes` table already enforces UNIQUE(session_id, fingerprint)
// via the schema's idx_crashes_session_fp index and UpsertCrash bumps
// `count`+`ts` for duplicates, so every row returned is the
// already-deduplicated representative of one fingerprint-per-session.
// No GROUP BY needed at this layer.
//
// Auth posture: registered outside the bearer-group; the dashboard
// sub-router is permanently Loopback-only (see ADR-011 *Consequences*
// and the Dashboard field doc in internal/http/server.go).
package dashboard

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/VibeCodeSolutions/tracelab/internal/store"
)

// CrashesPageSize is the default page size for the crash-inspector tab.
// 50 mirrors the sessions tab's default. Exposed as a package variable
// so tests can shrink it without changing the production default.
var CrashesPageSize = 50

// CrashesPageSizeMax caps the user-controllable ?limit= override —
// belt-and-braces against pathological URLs.
const CrashesPageSizeMax = 500

// topFramesPreview is the number of leading stacktrace lines surfaced
// in the list-view "Top frames" column. Three matches the
// fingerprint-window used by internal/crash.Fingerprint, so a user who
// recognises a fingerprint also recognises the preview at a glance.
const topFramesPreview = 3

// crashSortKey is the wire-level enum for the ?sort= query parameter.
// Translated to store.CrashSort at the query boundary. The set is
// closed; unknown values silently fall back to the default.
type crashSortKey string

const (
	crashSortTSDesc          crashSortKey = "ts_desc"
	crashSortTSAsc           crashSortKey = "ts_asc"
	crashSortCountDesc       crashSortKey = "count_desc"
	crashSortFingerprintAsc  crashSortKey = "fingerprint_asc"
	defaultCrashSortKey                   = crashSortTSDesc
)

// allowedCrashSortKeys is the whitelist for ?sort=. Listed in the order
// the template renders them in the sort dropdown.
var allowedCrashSortKeys = []crashSortKey{
	crashSortTSDesc,
	crashSortTSAsc,
	crashSortCountDesc,
	crashSortFingerprintAsc,
}

// crashSortLabels are the human-readable strings shown next to each
// option in the sort dropdown.
var crashSortLabels = map[crashSortKey]string{
	crashSortTSDesc:         "Newest first",
	crashSortTSAsc:          "Oldest first",
	crashSortCountDesc:      "Most frequent first",
	crashSortFingerprintAsc: "Fingerprint",
}

// crashesViewParams is the validated query-param bundle for the list view.
type crashesViewParams struct {
	Sort      crashSortKey
	SessionID string
	Page      int // 1-based; clamped to >= 1
	Limit     int
}

// crashRow is the per-row dot value rendered in the crashes table.
type crashRow struct {
	ID            int64
	SessionID     string
	Fingerprint   string
	Count         int
	LastSeenHuman string
	TopFrames     []string // up to topFramesPreview lines
}

// crashSortOption is one entry in the rendered sort dropdown.
type crashSortOption struct {
	Key      string
	Label    string
	Selected bool
}

// sessionFilterOption is one entry in the session-filter dropdown.
type sessionFilterOption struct {
	ID       string
	Label    string // either the full session id or the "All sessions" sentinel
	Selected bool
}

// crashesViewData is the dot value passed to tab_crashes.gohtml.
type crashesViewData struct {
	Crashes        []crashRow
	SortOptions    []crashSortOption
	SessionOptions []sessionFilterOption
	SessionID      string // currently selected filter (empty = all)
	Page           int
	Limit          int
	Total          int64
	PageCount      int
	HasPrev        bool
	HasNext        bool
	PrevURL        string
	NextURL        string
	// Empty is true when no crashes match the current filter; the
	// template uses it to render an empty-state message rather than
	// an empty table.
	Empty bool
}

// crashDetailViewData is the dot value passed to tab_crash_detail.gohtml.
type crashDetailViewData struct {
	ID            int64
	SessionID     string
	Fingerprint   string
	Count         int
	LastSeenHuman string
	// StackLines is the full normalized stacktrace split on newlines so
	// the template can render it as a <pre> with per-line escaping.
	StackLines []string
	BackURL    string
}

// CrashesHandler is the htmx-swap endpoint at GET /dashboard/tab/crashes.
// Renders the list-view body (no envelope) so the response is droppable
// into #dashboard-content with hx-swap=innerHTML.
func (h *Handler) CrashesHandler(w http.ResponseWriter, r *http.Request) {
	body, err := h.renderCrashesBody(r)
	if err != nil {
		h.log.LogAttrs(r.Context(), slog.LevelError,
			"dashboard crashes: render failed",
			slog.Any("error", err))
		http.Error(w, "internal dashboard error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body)
}

// CrashDetailHandler is the htmx-swap endpoint at
// GET /dashboard/tab/crashes/{id}. Unknown id → 404 (loud failure on
// stale links, mirroring SessionDetailHandler).
func (h *Handler) CrashDetailHandler(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimPrefix(r.URL.Path, "/dashboard/tab/crashes/")
	// Defence-in-depth: chi's wildcard route already strips the leading
	// prefix; re-check for empty / embedded slash to fail loud on
	// surprises (e.g. "/dashboard/tab/crashes/foo/bar").
	if raw == "" || strings.Contains(raw, "/") {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	body, err := h.renderCrashDetailBody(r, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		h.log.LogAttrs(r.Context(), slog.LevelError,
			"dashboard crash-detail: render failed",
			slog.Int64("crash_id", id),
			slog.Any("error", err))
		http.Error(w, "internal dashboard error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body)
}

// renderCrashesBody is the data-fetch + template-execute core of the
// list view. Shared between CrashesHandler (htmx swap) and
// LayoutHandler (full-page render with the body slotted into the
// layout shell).
func (h *Handler) renderCrashesBody(r *http.Request) ([]byte, error) {
	if h.store == nil {
		return nil, fmt.Errorf("dashboard crashes: no store")
	}
	params := parseCrashesParams(r.URL.Query())
	ctx := r.Context()

	total, err := h.store.CountCrashes(ctx, params.SessionID)
	if err != nil {
		return nil, fmt.Errorf("count crashes: %w", err)
	}

	offset := (params.Page - 1) * params.Limit
	// Snap back to the last valid page if the filter shrank the result
	// set since the previous Prev/Next link was generated.
	if total > 0 && int64(offset) >= total {
		maxPage := int((total + int64(params.Limit) - 1) / int64(params.Limit))
		if maxPage < 1 {
			maxPage = 1
		}
		params.Page = maxPage
		offset = (params.Page - 1) * params.Limit
	}

	rows, err := h.store.ListCrashes(ctx, store.ListCrashesOpts{
		Limit:           params.Limit,
		Offset:          offset,
		FilterSessionID: params.SessionID,
		Sort:            params.Sort.toStoreSort(),
	})
	if err != nil {
		return nil, fmt.Errorf("list crashes: %w", err)
	}

	sessionIDs, err := h.store.ListSessionIDsWithCrashes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list crash session ids: %w", err)
	}

	view := buildCrashesViewData(rows, total, sessionIDs, params)
	var buf bytes.Buffer
	if err := h.crashesTpl.Execute(&buf, view); err != nil {
		return nil, fmt.Errorf("execute crashes tab: %w", err)
	}
	return buf.Bytes(), nil
}

// renderCrashDetailBody is the data-fetch + template-execute core of
// the detail view. Returns sql.ErrNoRows when the id is unknown so the
// caller maps to 404.
func (h *Handler) renderCrashDetailBody(r *http.Request, id int64) ([]byte, error) {
	if h.store == nil {
		return nil, fmt.Errorf("dashboard crash-detail: no store")
	}
	ctx := r.Context()

	c, err := h.store.CrashByID(ctx, id)
	if err != nil {
		// Forwarded verbatim — caller maps sql.ErrNoRows → 404.
		return nil, err
	}

	// Back-URL preserves the list-view's sort/session/page state so the
	// user returns to the same view they came from.
	backURL := "/dashboard/tab/crashes"
	if r.URL.RawQuery != "" {
		backURL += "?" + r.URL.RawQuery
	}

	view := crashDetailViewData{
		ID:            c.ID,
		SessionID:     c.SessionID,
		Fingerprint:   c.Fingerprint,
		Count:         c.Count,
		LastSeenHuman: formatHumanTime(c.TS),
		StackLines:    splitStacktrace(c.Stacktrace),
		BackURL:       backURL,
	}
	var buf bytes.Buffer
	if err := h.crashDetailTpl.Execute(&buf, view); err != nil {
		return nil, fmt.Errorf("execute crash-detail tab: %w", err)
	}
	return buf.Bytes(), nil
}

// parseCrashesParams validates url.Values into the typed param bundle.
// Unknown / out-of-range values are clamped to defaults; the handler
// never 400s the user since this surface is loaded by browser
// navigation (a 400 would render as a raw error page mid-tab-swap).
func parseCrashesParams(q url.Values) crashesViewParams {
	p := crashesViewParams{
		Sort:      defaultCrashSortKey,
		SessionID: q.Get("session"),
		Page:      1,
		Limit:     CrashesPageSize,
	}
	if raw := q.Get("sort"); raw != "" {
		candidate := crashSortKey(raw)
		for _, k := range allowedCrashSortKeys {
			if k == candidate {
				p.Sort = candidate
				break
			}
		}
	}
	if raw := q.Get("page"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 1 {
			p.Page = n
		}
	}
	if raw := q.Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			if n > CrashesPageSizeMax {
				n = CrashesPageSizeMax
			}
			p.Limit = n
		}
	}
	// Trim + cap the session-id filter so a malicious URL can't blow
	// up the WHERE clause's bind allocation. Session ids in this store
	// are 26 hex chars; a small ceiling is plenty.
	p.SessionID = strings.TrimSpace(p.SessionID)
	if len(p.SessionID) > 128 {
		p.SessionID = p.SessionID[:128]
	}
	return p
}

// toStoreSort maps the wire-level crashSortKey to the store's typed
// enum. Unknown values (already filtered by allowedCrashSortKeys at
// parse-time) map to the default for belt-and-braces.
func (k crashSortKey) toStoreSort() store.CrashSort {
	switch k {
	case crashSortTSAsc:
		return store.SortCrashTSAsc
	case crashSortCountDesc:
		return store.SortCrashCountDesc
	case crashSortFingerprintAsc:
		return store.SortCrashFingerprintAsc
	case crashSortTSDesc:
		fallthrough
	default:
		return store.SortCrashTSDesc
	}
}

// buildCrashesViewData assembles the typed dot value for tab_crashes.gohtml
// from the raw store rows, the session-id list, and the validated params.
func buildCrashesViewData(rows []store.CrashRow, total int64, sessionIDs []string, p crashesViewParams) crashesViewData {
	out := crashesViewData{
		Crashes:        make([]crashRow, 0, len(rows)),
		SortOptions:    buildCrashSortOptions(p.Sort),
		SessionOptions: buildSessionFilterOptions(sessionIDs, p.SessionID),
		SessionID:      p.SessionID,
		Page:           p.Page,
		Limit:          p.Limit,
		Total:          total,
		Empty:          len(rows) == 0,
	}
	for _, r := range rows {
		out.Crashes = append(out.Crashes, crashRow{
			ID:            r.ID,
			SessionID:     r.SessionID,
			Fingerprint:   r.Fingerprint,
			Count:         r.Count,
			LastSeenHuman: formatHumanTime(r.TS),
			TopFrames:     extractTopFrames(r.Stacktrace, topFramesPreview),
		})
	}
	if total > 0 {
		out.PageCount = int((total + int64(p.Limit) - 1) / int64(p.Limit))
	} else {
		out.PageCount = 1
	}
	out.HasPrev = p.Page > 1
	out.HasNext = p.Page < out.PageCount
	out.PrevURL = buildCrashesURL(p, p.Page-1)
	out.NextURL = buildCrashesURL(p, p.Page+1)
	return out
}

// buildCrashSortOptions returns the [{key,label,selected}] list for the
// sort dropdown.
func buildCrashSortOptions(selected crashSortKey) []crashSortOption {
	out := make([]crashSortOption, 0, len(allowedCrashSortKeys))
	for _, k := range allowedCrashSortKeys {
		out = append(out, crashSortOption{
			Key:      string(k),
			Label:    crashSortLabels[k],
			Selected: k == selected,
		})
	}
	return out
}

// buildSessionFilterOptions returns the [{id,label,selected}] list for
// the session-filter dropdown. Always prepends an "all sessions"
// sentinel entry (id="") so the user can clear the filter.
func buildSessionFilterOptions(sessionIDs []string, selectedID string) []sessionFilterOption {
	out := make([]sessionFilterOption, 0, len(sessionIDs)+1)
	out = append(out, sessionFilterOption{
		ID:       "",
		Label:    "All sessions",
		Selected: selectedID == "",
	})
	for _, id := range sessionIDs {
		out = append(out, sessionFilterOption{
			ID:       id,
			Label:    id,
			Selected: id == selectedID,
		})
	}
	return out
}

// buildCrashesURL constructs the /dashboard/tab/crashes URL with the
// current sort+session filter and the given (1-based) page. Used for
// htmx-driven prev/next navigation.
func buildCrashesURL(p crashesViewParams, page int) string {
	if page < 1 {
		page = 1
	}
	q := []string{
		"sort=" + string(p.Sort),
		"page=" + strconv.Itoa(page),
		"limit=" + strconv.Itoa(p.Limit),
	}
	if p.SessionID != "" {
		q = append(q, "session="+url.QueryEscape(p.SessionID))
	}
	return "/dashboard/tab/crashes?" + strings.Join(q, "&")
}

// extractTopFrames pulls the first n "interesting" non-header lines
// from a stacktrace. Header shapes (Traceback, Exception in thread,
// panic:, etc.) are skipped so the preview surfaces the actual frame
// locations the fingerprint hashes over — the user gets a meaningful
// "where did this crash" summary at the list level. Empty input
// returns nil so the template can render a graceful "—" instead of a
// blank cell.
func extractTopFrames(stack string, n int) []string {
	if stack == "" {
		return nil
	}
	out := make([]string, 0, n)
	for _, line := range strings.Split(stack, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		// Stop on the obvious header shapes. Kept inline (rather than
		// reusing internal/crash.isHeaderLine) so the dashboard never
		// imports the crash package — that would risk cycle on a
		// future crash-package refactor.
		if isPreviewHeader(t) {
			continue
		}
		out = append(out, t)
		if len(out) >= n {
			break
		}
	}
	return out
}

// isPreviewHeader matches the same header shapes internal/crash.topFrames
// skips, kept duplicated here so the dashboard layer doesn't take an
// import dependency on the crash package.
func isPreviewHeader(t string) bool {
	switch {
	case strings.HasPrefix(t, "Traceback (most recent call last):"),
		strings.HasPrefix(t, "Exception in thread"),
		strings.HasPrefix(t, "Caused by:"),
		strings.HasPrefix(t, "panic:"),
		strings.HasPrefix(t, "goroutine N ["),
		strings.HasPrefix(t, "thread '"),
		strings.HasPrefix(t, "stack backtrace:"):
		return true
	}
	return false
}

// splitStacktrace breaks a stacktrace into trimmed-but-preserved lines
// for the detail view. Unlike extractTopFrames, headers are kept so the
// user sees the full context.
func splitStacktrace(stack string) []string {
	if stack == "" {
		return nil
	}
	return strings.Split(stack, "\n")
}
