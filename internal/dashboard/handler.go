// Package dashboard owns the HTTP handlers that render the tracelab
// dashboard UI (Phase 2c, ADR-011 Decision 4).
//
// Handlers are constructed by NewHandler and registered as a sub-router
// under /dashboard* in internal/http.New. Templates and static assets
// live in the top-level web package and are accessed exclusively via
// the //go:embed filesystems web.Templates and web.Static.
//
// Lifecycle: NewHandler parses every template in web.Templates at
// construction time (template.ParseFS). Failure to parse is a fatal
// configuration error and surfaces as a non-nil error to the caller —
// the hub then refuses to start, which is the correct posture for a
// missing-asset bug (vs. silently 500-ing on first dashboard hit).
//
// Phase 2c S1 scope: skeleton + tab navigation. Layout, four tab
// placeholders (live-tail / sessions / crashes / agents), static-asset
// serving via http.FileServer over web.Static. S2-S5 fill the tab
// bodies and add the live-tail data path (ADR-012, pending).
package dashboard

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/VibeCodeSolutions/tracelab/internal/store"
	"github.com/VibeCodeSolutions/tracelab/web"
)

// DefaultTab is the slug rendered when GET /dashboard is hit without a
// ?tab=<slug> query string. Picked to match the natural left-to-right
// reading order of the tab bar (live-tail is the dashboard's primary
// surface in Phase 2c).
const DefaultTab = "live-tail"

// Tab describes one entry in the dashboard's tab navigation. The slug
// is both the URL parameter (?tab=<slug>) and the template suffix
// (tab_<slug>.gohtml after substituting "-" for "_").
type Tab struct {
	Slug   string
	Label  string
	Active bool
}

// tabs is the ordered list of tabs rendered by the layout. Defined as
// a package-level slice so the order is single-source and tests can
// reach for it. Phase 2c S2-S5 do not reorder this list.
var tabs = []Tab{
	{Slug: "live-tail", Label: "Live-Tail"},
	{Slug: "sessions", Label: "Sessions"},
	{Slug: "crashes", Label: "Crashes"},
	{Slug: "agents", Label: "Agents"},
}

// layoutData is the dot value passed to base.gohtml. It carries the
// active tab's slug, the full tab list (so the template can mark the
// active one), the rendered active-tab body as a pre-escaped
// template.HTML chunk, and the hub version string for the header.
type layoutData struct {
	Version   string
	ActiveTab string
	Tabs      []Tab
	Body      template.HTML
}

// Handler is the HTTP-handler bundle for the dashboard sub-router. It
// is constructed once at hub start-up and shared across requests; all
// state (template trees, static FS) is read-only.
//
// store is the persistence layer used by the data-driven tabs
// (Phase 2c S3 sessions, S4 crashes). It may be nil in unit-test
// contexts that only exercise the static-shell rendering (skeleton +
// live-tail SSE), in which case the data-driven handlers return 503
// rather than panicking.
type Handler struct {
	version string
	log     *slog.Logger
	store   *store.Store

	layout *template.Template            // base.gohtml as the entrypoint
	tabTpl map[string]*template.Template // slug → tab_<slug>.gohtml

	// sessionsTpl renders the Phase 2c S3 session-browser tab body
	// (templates/tab_sessions.gohtml) with a live data payload. Kept
	// separate from tabTpl because the rest of tabTpl is rendered
	// with nil data; here we need a typed dot value.
	sessionsTpl *template.Template
	// sessionDetailTpl renders the Phase 2c S3 session-detail view
	// (templates/tab_session_detail.gohtml). Same rationale as
	// sessionsTpl.
	sessionDetailTpl *template.Template

	staticHandler http.Handler // wraps web.Static for /dashboard/static/*
}

// NewHandler parses every template in web.Templates and prepares the
// static-asset sub-handler. version is the tracelab-hub semver
// (rendered in the dashboard header). log is the slog handler used for
// dashboard-side error logging; nil falls back to slog.Default().
// st is the persistence layer used by data-driven tabs (S3 sessions,
// S4 crashes); pass nil for tests that only need the static shell.
func NewHandler(version string, log *slog.Logger, st *store.Store) (*Handler, error) {
	if log == nil {
		log = slog.Default()
	}

	// base.gohtml is the layout entrypoint. ParseFS attaches every
	// other template found in templates/*.gohtml as named templates,
	// but we render each tab body in a separate ParseFS pass so the
	// active body can be passed in as a pre-rendered HTML chunk to
	// base.gohtml. This keeps the template surface "one entrypoint
	// per render" rather than relying on {{template "tab_…"}} dispatch
	// inside the layout — the layout doesn't need to know the tab
	// list at template-compile time.
	layout, err := template.ParseFS(web.Templates, "templates/base.gohtml")
	if err != nil {
		return nil, fmt.Errorf("dashboard: parse base layout: %w", err)
	}

	tabTpl := make(map[string]*template.Template, len(tabs))
	for _, t := range tabs {
		name := tabTemplateName(t.Slug)
		tpl, err := template.ParseFS(web.Templates, "templates/"+name)
		if err != nil {
			return nil, fmt.Errorf("dashboard: parse %s: %w", name, err)
		}
		tabTpl[t.Slug] = tpl
	}

	// Phase 2c S3 data-driven templates. Parsed via fresh ParseFS
	// passes so each gets its own root template namespace; the
	// session-detail template is fully independent of the layout shell
	// and the session-list shell (no template-block reuse needed).
	sessionsTpl, err := template.ParseFS(web.Templates,
		"templates/tab_sessions.gohtml")
	if err != nil {
		return nil, fmt.Errorf("dashboard: parse sessions tab: %w", err)
	}
	sessionDetailTpl, err := template.ParseFS(web.Templates,
		"templates/tab_session_detail.gohtml")
	if err != nil {
		return nil, fmt.Errorf("dashboard: parse session-detail tab: %w", err)
	}

	// /dashboard/static/* serves the embedded JS/CSS verbatim. Sub-FS
	// strips the leading "static/" so the URL path maps to file names
	// directly (e.g. /dashboard/static/htmx.min.js → static/htmx.min.js
	// in the embed FS).
	sub, err := fs.Sub(web.Static, "static")
	if err != nil {
		return nil, fmt.Errorf("dashboard: sub-FS static: %w", err)
	}
	staticHandler := http.StripPrefix("/dashboard/static/", http.FileServer(http.FS(sub)))

	return &Handler{
		version:          version,
		log:              log,
		store:            st,
		layout:           layout,
		tabTpl:           tabTpl,
		sessionsTpl:      sessionsTpl,
		sessionDetailTpl: sessionDetailTpl,
		staticHandler:    staticHandler,
	}, nil
}

// LayoutHandler renders GET /dashboard. The optional ?tab=<slug>
// selects the active tab; unknown or absent slugs fall back to
// DefaultTab. The response is a full HTML document.
func (h *Handler) LayoutHandler(w http.ResponseWriter, r *http.Request) {
	slug := r.URL.Query().Get("tab")
	if _, ok := h.tabTpl[slug]; !ok {
		slug = DefaultTab
	}

	body, err := h.renderTabBody(r, slug)
	if err != nil {
		h.log.Error("dashboard layout: tab render failed",
			slog.String("tab", slug), slog.Any("error", err))
		http.Error(w, "internal dashboard error", http.StatusInternalServerError)
		return
	}

	data := layoutData{
		Version:   h.version,
		ActiveTab: slug,
		Tabs:      h.tabsWithActive(slug),
		Body:      template.HTML(body), //nolint:gosec // body comes from our own embedded templates, never user input
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.layout.Execute(w, data); err != nil {
		// Headers already sent; downgrade to a log-only error.
		h.log.Error("dashboard layout: execute failed", slog.Any("error", err))
	}
}

// renderTabBody is the dispatch point for "render this tab's body
// bytes". Most tabs are static and go through renderTab (no request
// data needed); the data-driven tabs (Phase 2c S3 sessions, future
// S4 crashes) take the *http.Request so they can read query params.
//
// When the store is nil (skeleton-only tests) the data-driven tabs
// render an empty view rather than 500-ing — the template still
// executes against a zero-value sessionsViewData with empty Sessions
// slice, so the tab body is well-formed and discoverable in the
// rendered HTML.
func (h *Handler) renderTabBody(r *http.Request, slug string) ([]byte, error) {
	switch slug {
	case "sessions":
		if h.store != nil {
			return h.renderSessionsBody(r)
		}
		return h.renderEmptySessionsBody()
	default:
		return h.renderTab(slug)
	}
}

// renderEmptySessionsBody executes the sessions tab template against a
// well-formed but empty view-data. Used in skeleton-only test
// contexts where no store is wired in.
func (h *Handler) renderEmptySessionsBody() ([]byte, error) {
	view := sessionsViewData{
		Sessions:    nil,
		SortOptions: buildSortOptions(defaultSortKey),
		Page:        1,
		Limit:       SessionsPageSize,
		Total:       0,
		PageCount:   1,
		Empty:       true,
	}
	var buf bytes.Buffer
	if err := h.sessionsTpl.Execute(&buf, view); err != nil {
		return nil, fmt.Errorf("execute empty sessions tab: %w", err)
	}
	return buf.Bytes(), nil
}

// TabHandler renders GET /dashboard/tab/{slug} for htmx hx-get swaps.
// The response is the tab body alone (no <html> envelope) so htmx can
// drop it into #dashboard-content with hx-swap="innerHTML". Unknown
// slugs return 404 (different posture from LayoutHandler — for the
// xhr swap we want the error to surface so a stale page link fails
// loud instead of silently rendering the default tab).
//
// The sessions tab and the session-detail sub-route own their own
// http.HandlerFunc (SessionsHandler / SessionDetailHandler) because
// they handle nested paths ("/dashboard/tab/sessions/{id}") that
// don't fit the simple slug-to-template map this handler dispatches.
// The router wires those before the wildcard, so TabHandler only
// sees the static-template slugs at runtime.
func (h *Handler) TabHandler(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/dashboard/tab/")
	if slug == "" || strings.Contains(slug, "/") {
		http.NotFound(w, r)
		return
	}
	if _, ok := h.tabTpl[slug]; !ok {
		http.NotFound(w, r)
		return
	}
	body, err := h.renderTabBody(r, slug)
	if err != nil {
		h.log.Error("dashboard tab: render failed",
			slog.String("tab", slug), slog.Any("error", err))
		http.Error(w, "internal dashboard error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body)
}

// StaticHandler serves /dashboard/static/* from the embedded asset
// tree. Wraps http.FileServer(http.FS(web.Static)) with a StripPrefix.
func (h *Handler) StaticHandler(w http.ResponseWriter, r *http.Request) {
	h.staticHandler.ServeHTTP(w, r)
}

// renderTab executes the per-tab template into a []byte. The bytes are
// then either wrapped by the layout (LayoutHandler) or written
// directly to the response (TabHandler).
func (h *Handler) renderTab(slug string) ([]byte, error) {
	tpl, ok := h.tabTpl[slug]
	if !ok {
		return nil, fmt.Errorf("unknown tab: %q", slug)
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, nil); err != nil {
		return nil, fmt.Errorf("execute %s: %w", slug, err)
	}
	return buf.Bytes(), nil
}

// tabsWithActive returns a copy of the package-level tabs slice with
// the Active flag set on the matching slug. Returns a fresh slice so
// concurrent layout renders never share the underlying array.
func (h *Handler) tabsWithActive(activeSlug string) []Tab {
	out := make([]Tab, len(tabs))
	for i, t := range tabs {
		t.Active = (t.Slug == activeSlug)
		out[i] = t
	}
	return out
}

// tabTemplateName maps a tab slug to its template file name. Slugs use
// hyphens for URL legibility ("live-tail") and templates use underscores
// for Go-file-style readability ("tab_live_tail.gohtml").
func tabTemplateName(slug string) string {
	return "tab_" + strings.ReplaceAll(slug, "-", "_") + ".gohtml"
}
