// Package web hosts the embedded UI artefact tree for the tracelab
// dashboard (Phase 2c, ADR-011 Decision 3 + Decision 4).
//
// The package has no Go business logic: it declares two //go:embed
// filesystems, Templates and Static, and exports them as the public
// surface consumed by internal/dashboard at handler-init time.
//
// Layout:
//
//	web/
//	├── embed.go          ← this file
//	├── templates/        ← html/template files (.gohtml)
//	└── static/           ← htmx.min.js, dashboard.css (vendored assets)
//
// Templates and static assets ship inside the tracelab-hub binary; no
// external --web-root flag, no loose-file fallback. See ADR-011 for the
// rationale (single-binary distribution, offline reproducibility,
// CGO-free preserved).
package web

import "embed"

// Templates is the html/template tree. Files have the .gohtml extension
// to be unambiguous as Go templates (vs. raw HTML editor previews) and
// are parsed by internal/dashboard via template.ParseFS.
//
//go:embed templates/*.gohtml
var Templates embed.FS

// Static carries the vendored client-side assets (htmx + dashboard CSS).
// Served verbatim by the dashboard handler at /dashboard/static/*. The
// htmx distribution is pinned at vendor time — version + source URL
// documented in the file header of static/htmx.min.js.
//
//go:embed static/*
var Static embed.FS
