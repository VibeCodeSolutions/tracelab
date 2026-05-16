// sessions.go — Session-browser tab handlers (Phase 2c S3).
//
// Two surfaces:
//
//   - SessionsHandler / renderSessionsBody — the list view at
//     /dashboard/tab/sessions (htmx swap) and the body slot embedded in
//     the full-layout render at /dashboard?tab=sessions. Parses
//     sort/filter/page query params, queries store.ListSessionsWithCounts
//     + store.CountSessions, renders templates/tab_sessions.gohtml.
//   - SessionDetailHandler / renderSessionDetailBody — the detail view
//     at /dashboard/tab/sessions/{id} (htmx swap). Verifies session
//     existence via store.SessionByID (404 on unknown id), fetches
//     recent events via store.RecentEvents and crashes via
//     store.CrashesBySession, renders templates/tab_session_detail.gohtml.
//
// Both handlers write body-only HTML (no <html> envelope) so htmx can
// drop the response into #dashboard-content with hx-swap=innerHTML.
// The "wrap in layout" path goes through LayoutHandler in handler.go,
// which calls renderSessionsBody / renderSessionDetailBody and wraps
// the bytes in the layout's template.HTML slot.
//
// Auth posture: registered outside the bearer-group; see the Dashboard
// field doc in internal/http/server.go for the full rationale
// (permanently Loopback-only, ADR-011 *Consequences*).
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
	"time"

	"github.com/VibeCodeSolutions/tracelab/internal/store"
)

// SessionsPageSize is the default page size for the session-browser tab.
// 50 mirrors the existing GET /sessions default. Exposed as a package
// variable so tests can shrink it without changing the production
// default.
var SessionsPageSize = 50

// SessionsPageSizeMax caps the user-controllable ?limit= override.
// Keeping it well below the HTTP-layer's /sessions ceiling (1000) so a
// curious user can't trash the dashboard render budget by handing us a
// 10k page size.
const SessionsPageSizeMax = 500

// detailEventsLimit caps the per-detail-view event list. 200 is the
// memo'd default from the auftrag — large enough to read a typical
// crash run end-to-end, small enough that the rendered HTML stays
// under ~150 KB even with verbose meta payloads.
const detailEventsLimit = 200

// sortKey is the typed enum for the ?sort= query parameter. Kept as
// strings on the wire (URL-friendly), translated to the store's
// SessionSort at the query boundary. The set is closed; any other
// value silently falls back to the default.
type sortKey string

const (
	sortStartedAtDesc   sortKey = "started_at_desc"
	sortStartedAtAsc    sortKey = "started_at_asc"
	sortSessionIDAsc    sortKey = "session_id"
	sortEventCountDesc  sortKey = "event_count_desc"
	defaultSortKey              = sortStartedAtDesc
)

// allowedSortKeys is the whitelist the HTTP layer validates ?sort=
// against. Listed in the order the template renders them in the sort
// dropdown.
var allowedSortKeys = []sortKey{
	sortStartedAtDesc,
	sortStartedAtAsc,
	sortSessionIDAsc,
	sortEventCountDesc,
}

// sortLabels are the human-readable strings rendered next to each
// option in the sort dropdown. Order tracks allowedSortKeys.
var sortLabels = map[sortKey]string{
	sortStartedAtDesc:  "Newest first",
	sortStartedAtAsc:   "Oldest first",
	sortSessionIDAsc:   "Session-ID",
	sortEventCountDesc: "Most events first",
}

// sessionsViewParams is the validated query-param bundle that drives
// the session-browser list view. Built by parseSessionsParams from the
// raw *http.Request.URL.Query().
type sessionsViewParams struct {
	Sort   sortKey
	Filter string
	Page   int // 1-based; clamped to >= 1
	Limit  int
}

// sessionRow is the per-row dot value rendered in the sessions table.
// Mirrors store.SessionWithCounts but with display-ready strings so
// the template stays HTML-only (no time-formatting in the template).
type sessionRow struct {
	ID             string
	Label          string
	StartedAtHuman string
	EndedAtHuman   string // empty when EndedAt is nil
	EventCount     int64
	CrashCount     int64
}

// sortOption is one entry in the rendered sort-dropdown.
type sortOption struct {
	Key      string
	Label    string
	Selected bool
}

// sessionsViewData is the dot value passed to tab_sessions.gohtml.
type sessionsViewData struct {
	Sessions    []sessionRow
	SortOptions []sortOption
	Filter      string
	Page        int
	Limit       int
	Total       int64
	PageCount   int
	HasPrev     bool
	HasNext     bool
	PrevURL     string
	NextURL     string
	// Empty is true when no sessions match the current filter; the
	// template uses it to decide between rendering the table and an
	// "empty state" message.
	Empty bool
}

// sessionDetailViewData is the dot value passed to
// tab_session_detail.gohtml.
type sessionDetailViewData struct {
	Session        sessionRow
	Events         []detailEventRow
	Crashes        []detailCrashRow
	EventLimit     int
	EventLimitHit  bool
	BackURL        string
}

// detailEventRow is one event in the detail view.
type detailEventRow struct {
	SeqID    int64
	TSHuman  string
	Source   string
	Level    string
	Msg      string
}

// detailCrashRow is one crash in the detail view.
type detailCrashRow struct {
	ID          int64
	TSHuman     string
	Fingerprint string
	Count       int
}

// SessionsHandler is the htmx-swap endpoint at
// GET /dashboard/tab/sessions. Renders the list view body (no
// envelope) so the response is droppable into #dashboard-content with
// hx-swap=innerHTML.
func (h *Handler) SessionsHandler(w http.ResponseWriter, r *http.Request) {
	body, err := h.renderSessionsBody(r)
	if err != nil {
		h.log.LogAttrs(r.Context(), slog.LevelError,
			"dashboard sessions: render failed",
			slog.Any("error", err))
		http.Error(w, "internal dashboard error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body)
}

// SessionDetailHandler is the htmx-swap endpoint at
// GET /dashboard/tab/sessions/{id}. Renders the detail view body.
//
// Unknown session id → 404 (not a silent fallback): the user clicked
// a stale link or hand-typed a bad id; we want that to fail loud.
func (h *Handler) SessionDetailHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/dashboard/tab/sessions/")
	// Defence-in-depth: chi's wildcard route already strips the
	// leading prefix, but we re-check the trailing-slash and any
	// embedded slash to avoid surprises (e.g. "/dashboard/tab/sessions/foo/bar").
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	body, err := h.renderSessionDetailBody(r, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		h.log.LogAttrs(r.Context(), slog.LevelError,
			"dashboard session-detail: render failed",
			slog.String("session_id", id),
			slog.Any("error", err))
		http.Error(w, "internal dashboard error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body)
}

// renderSessionsBody is the data-fetch + template-execute core of the
// list view. Shared between SessionsHandler (htmx swap) and
// LayoutHandler (full-page render with the body slotted into the
// layout shell).
func (h *Handler) renderSessionsBody(r *http.Request) ([]byte, error) {
	if h.store == nil {
		// Defensive: NewHandler accepts nil store for skeleton-only
		// tests. The production hub always wires st through.
		return nil, fmt.Errorf("dashboard sessions: no store")
	}
	params := parseSessionsParams(r.URL.Query())
	ctx := r.Context()

	total, err := h.store.CountSessions(ctx, params.Filter)
	if err != nil {
		return nil, fmt.Errorf("count sessions: %w", err)
	}

	offset := (params.Page - 1) * params.Limit
	// If the page is past the end (e.g. filter shrank the result set
	// since the previous page-link was generated), snap back to the
	// last valid page. Keeps Prev/Next from getting stuck on a
	// permanently-empty view.
	if total > 0 && int64(offset) >= total {
		maxPage := int((total + int64(params.Limit) - 1) / int64(params.Limit))
		if maxPage < 1 {
			maxPage = 1
		}
		params.Page = maxPage
		offset = (params.Page - 1) * params.Limit
	}

	rows, err := h.store.ListSessionsWithCounts(ctx, store.ListSessionsOpts{
		Limit:             params.Limit,
		Offset:            offset,
		FilterIDSubstring: params.Filter,
		Sort:              params.Sort.toStoreSort(),
	})
	if err != nil {
		return nil, fmt.Errorf("list sessions with counts: %w", err)
	}

	view := buildSessionsViewData(rows, total, params)
	var buf bytes.Buffer
	if err := h.sessionsTpl.Execute(&buf, view); err != nil {
		return nil, fmt.Errorf("execute sessions tab: %w", err)
	}
	return buf.Bytes(), nil
}

// renderSessionDetailBody is the data-fetch + template-execute core of
// the detail view. Returns sql.ErrNoRows when the session id is
// unknown so the caller can map to 404.
func (h *Handler) renderSessionDetailBody(r *http.Request, id string) ([]byte, error) {
	if h.store == nil {
		return nil, fmt.Errorf("dashboard session-detail: no store")
	}
	ctx := r.Context()

	sess, err := h.store.SessionByID(ctx, id)
	if err != nil {
		// Forwarded verbatim — caller handles sql.ErrNoRows → 404.
		return nil, err
	}

	events, err := h.store.RecentEvents(ctx, id, detailEventsLimit+1)
	if err != nil {
		return nil, fmt.Errorf("recent events: %w", err)
	}
	limitHit := false
	if len(events) > detailEventsLimit {
		events = events[:detailEventsLimit]
		limitHit = true
	}

	crashes, err := h.store.CrashesBySession(ctx, id, 100)
	if err != nil {
		return nil, fmt.Errorf("crashes by session: %w", err)
	}

	// Re-derive the counts for the detail header — these may be
	// higher than detailEventsLimit since the limit only bounds
	// what we render.
	row := sessionRow{
		ID:             sess.ID,
		Label:          sess.Label,
		StartedAtHuman: formatHumanTime(sess.StartedAt),
		EventCount:     int64(len(events)),
		CrashCount:     int64(len(crashes)),
	}
	if sess.EndedAt != nil {
		row.EndedAtHuman = formatHumanTime(*sess.EndedAt)
	}

	eventRows := make([]detailEventRow, len(events))
	for i, e := range events {
		eventRows[i] = detailEventRow{
			SeqID:   e.ID,
			TSHuman: formatHumanTime(e.TS),
			Source:  e.Source,
			Level:   e.Level,
			Msg:     e.Msg,
		}
	}
	crashRows := make([]detailCrashRow, len(crashes))
	for i, c := range crashes {
		crashRows[i] = detailCrashRow{
			ID:          c.ID,
			TSHuman:     formatHumanTime(c.TS),
			Fingerprint: c.Fingerprint,
			Count:       c.Count,
		}
	}

	// Back-URL preserves sort/filter/page from the query string so
	// the user returns to the same list state they came from.
	backURL := "/dashboard/tab/sessions"
	if r.URL.RawQuery != "" {
		backURL += "?" + r.URL.RawQuery
	}

	view := sessionDetailViewData{
		Session:       row,
		Events:        eventRows,
		Crashes:       crashRows,
		EventLimit:    detailEventsLimit,
		EventLimitHit: limitHit,
		BackURL:       backURL,
	}
	var buf bytes.Buffer
	if err := h.sessionDetailTpl.Execute(&buf, view); err != nil {
		return nil, fmt.Errorf("execute session-detail tab: %w", err)
	}
	return buf.Bytes(), nil
}

// parseSessionsParams validates url.Values into the typed param bundle.
// Unknown / out-of-range values are clamped to the defaults; the
// handler never 400s the user, since this surface is loaded by browser
// navigation (a 400 would render as a raw error page mid-tab-swap).
func parseSessionsParams(q url.Values) sessionsViewParams {
	p := sessionsViewParams{
		Sort:   defaultSortKey,
		Filter: q.Get("filter"),
		Page:   1,
		Limit:  SessionsPageSize,
	}
	if raw := q.Get("sort"); raw != "" {
		candidate := sortKey(raw)
		for _, k := range allowedSortKeys {
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
			if n > SessionsPageSizeMax {
				n = SessionsPageSizeMax
			}
			p.Limit = n
		}
	}
	// Sanitise filter: trim and cap length so a 4 MB curl-injection
	// can't blow up the LIKE pattern allocation.
	p.Filter = strings.TrimSpace(p.Filter)
	if len(p.Filter) > 128 {
		p.Filter = p.Filter[:128]
	}
	return p
}

// toStoreSort maps the wire-level sortKey to the store's typed enum.
// Unknown sortKey values (already filtered by allowedSortKeys at
// parse-time) map to the default for belt-and-braces.
func (k sortKey) toStoreSort() store.SessionSort {
	switch k {
	case sortStartedAtAsc:
		return store.SortSessionStartedAtAsc
	case sortSessionIDAsc:
		return store.SortSessionIDAsc
	case sortEventCountDesc:
		return store.SortSessionEventCountDesc
	case sortStartedAtDesc:
		fallthrough
	default:
		return store.SortSessionStartedAtDesc
	}
}

// buildSessionsViewData assembles the typed dot-value for
// tab_sessions.gohtml from the raw store rows and the validated
// params. Centralised so the template stays a pure renderer.
func buildSessionsViewData(rows []store.SessionWithCounts, total int64, p sessionsViewParams) sessionsViewData {
	out := sessionsViewData{
		Sessions:    make([]sessionRow, 0, len(rows)),
		SortOptions: buildSortOptions(p.Sort),
		Filter:      p.Filter,
		Page:        p.Page,
		Limit:       p.Limit,
		Total:       total,
		Empty:       len(rows) == 0,
	}
	for _, r := range rows {
		row := sessionRow{
			ID:             r.ID,
			Label:          r.Label,
			StartedAtHuman: formatHumanTime(r.StartedAt),
			EventCount:     r.EventCount,
			CrashCount:     r.CrashCount,
		}
		if r.EndedAt != nil {
			row.EndedAtHuman = formatHumanTime(*r.EndedAt)
		}
		out.Sessions = append(out.Sessions, row)
	}
	if total > 0 {
		out.PageCount = int((total + int64(p.Limit) - 1) / int64(p.Limit))
	} else {
		out.PageCount = 1
	}
	out.HasPrev = p.Page > 1
	out.HasNext = p.Page < out.PageCount
	out.PrevURL = buildSessionsURL(p, p.Page-1)
	out.NextURL = buildSessionsURL(p, p.Page+1)
	return out
}

// buildSortOptions returns the [{key,label,selected}] list for the
// sort dropdown.
func buildSortOptions(selected sortKey) []sortOption {
	out := make([]sortOption, 0, len(allowedSortKeys))
	for _, k := range allowedSortKeys {
		out = append(out, sortOption{
			Key:      string(k),
			Label:    sortLabels[k],
			Selected: k == selected,
		})
	}
	return out
}

// buildSessionsURL constructs the /dashboard/tab/sessions URL with the
// current filter+sort and the given (1-based) page. Used for
// htmx-driven prev/next navigation.
func buildSessionsURL(p sessionsViewParams, page int) string {
	if page < 1 {
		page = 1
	}
	q := []string{
		"sort=" + string(p.Sort),
		"page=" + strconv.Itoa(page),
		"limit=" + strconv.Itoa(p.Limit),
	}
	if p.Filter != "" {
		q = append(q, "filter="+url.QueryEscape(p.Filter))
	}
	return "/dashboard/tab/sessions?" + strings.Join(q, "&")
}

// formatHumanTime renders a UTC RFC-3339 timestamp without sub-second
// precision. Picked over a localised format because the dashboard is
// a single-user dev tool — UTC + ISO is unambiguous, easy to grep, and
// avoids pulling timezone awareness into the template layer.
func formatHumanTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04:05Z")
}
