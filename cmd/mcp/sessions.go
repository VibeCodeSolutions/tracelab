// sessions_list — first real MCP tool (Phase 2b S3).
//
// Per ADR-007 (Admin-confirmed 2026-05-15):
//
//   - Tool name:   sessions_list (snake_case <verb>_<noun>, no `tracelab_`
//     prefix — MCP ecosystem convention).
//   - Input:       { "limit"?: number, "since"?: string } — both optional.
//     `since` is an RFC3339 timestamp; rows with
//     StartedAt < since are filtered out client-side, since the
//     hub's GET /sessions has no `since` parameter today.
//   - Output:      { "sessions": Session[] } — JSON body delivered as a
//     TextContent (mcp-go v0.45.0 has no structured-result type;
//     the canonical pattern is a JSON-encoded TextContent block).
//   - Hub call:    internal/client.ListSessions(ctx, limit). Bearer is
//     attached by the *client.Client (constructed in newServer
//     with the resolved token).
//
// Session DTO shape (mirror of internal/client.Session, see types.go:37):
//
//	{ "id":"...", "label":"...", "started_at":1700..., "ended_at":1700... }
//
// `ended_at` is omitted (omitempty) for running sessions — consumers
// distinguish running vs. ended by key absence, not by a null value.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/VibeCodeSolutions/tracelab/internal/client"
)

// sessionsListToolName is the canonical tool name; pinned in one place so
// tests, the registration call, and any future error message reference
// the same literal.
const sessionsListToolName = "sessions_list"

// sessionsListDescription is the short description surfaced via
// tools/list. Kept compact (one sentence) so Claude Code's tool picker
// renders cleanly; full semantics live in this file's package-doc above.
const sessionsListDescription = "List recent tracelab sessions from the hub. Optional limit (number) and since (RFC3339 timestamp) filter."

// sessionsListResult is the public output envelope. JSON-encoded into a
// single TextContent per ADR-007 — see package doc.
type sessionsListResult struct {
	Sessions []client.Session `json:"sessions"`
}

// newSessionsListTool builds the ServerTool registered into the MCP
// server. The closure captures c so each invocation reuses the same hub
// client (bearer-bound at server-start).
func newSessionsListTool(c *client.Client) server.ServerTool {
	tool := mcp.NewTool(
		sessionsListToolName,
		mcp.WithDescription(sessionsListDescription),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of sessions to return. Hub default applies when omitted; hub caps at 1000."),
			mcp.Min(1),
		),
		mcp.WithString("since",
			mcp.Description("RFC3339 timestamp; sessions with started_at before this value are filtered out. Example: 2026-05-15T00:00:00Z"),
		),
	)
	return server.ServerTool{
		Tool:    tool,
		Handler: sessionsListHandler(c),
	}
}

// sessionsListHandler is the typed handler closure. Split from
// newSessionsListTool so the smoke test can invoke it without going
// through the MCP dispatch path.
func sessionsListHandler(c *client.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// mcp-go's GetInt tolerates missing / float64 / int — returns
		// the default (0) when absent or wrong-typed. That matches our
		// "both optional" semantic: 0 means "use hub default".
		limit := req.GetInt("limit", 0)

		// since is parsed up-front so an invalid timestamp is reported
		// as a tool-result error (IsError=true) rather than a Go error
		// — the latter would surface to the MCP transport, not to the
		// caller's tool result.
		sinceStr := req.GetString("since", "")
		var sinceNs int64
		if sinceStr != "" {
			t, err := time.Parse(time.RFC3339, sinceStr)
			if err != nil {
				return mcp.NewToolResultError(
					fmt.Sprintf("invalid since: expected RFC3339, got %q", sinceStr),
				), nil
			}
			sinceNs = t.UnixNano()
		}

		sessions, err := c.ListSessions(ctx, limit)
		if err != nil {
			return mcp.NewToolResultError(translateHubError(err)), nil
		}

		// Client-side since-filter. Cheap (O(n)) and keeps the hub
		// contract unchanged; if the hub ever grows a real `?since`
		// parameter, this drops to a no-op.
		if sinceNs > 0 {
			filtered := sessions[:0:0]
			for _, s := range sessions {
				if s.StartedAt >= sinceNs {
					filtered = append(filtered, s)
				}
			}
			sessions = filtered
		}

		// Defensive: never marshal a nil slice as "null" — emit "[]".
		if sessions == nil {
			sessions = []client.Session{}
		}
		body, err := json.Marshal(sessionsListResult{Sessions: sessions})
		if err != nil {
			// json.Marshal on this concrete shape cannot fail in
			// practice; surface as a tool-result error so the caller
			// sees a useful message instead of an opaque transport
			// failure.
			return mcp.NewToolResultError(
				fmt.Sprintf("encode response: %v", err),
			), nil
		}
		return mcp.NewToolResultText(string(body)), nil
	}
}

// translateHubError maps client-package errors to short tool-result
// strings. We do NOT leak Go-error wrap noise (`Get "http://..."`,
// dial-tcp addresses) — the consumer is an MCP client, often an LLM,
// which benefits from a concise actionable message.
func translateHubError(err error) string {
	switch {
	case errors.Is(err, client.ErrUnauthorized):
		return "unauthorized — check [auth].token in tracelab.toml"
	case errors.Is(err, client.ErrServerError):
		return fmt.Sprintf("hub error: %v", err)
	default:
		return fmt.Sprintf("cannot reach hub: %v", err)
	}
}
