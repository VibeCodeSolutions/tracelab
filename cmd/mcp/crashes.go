// crashes_list — fourth real MCP tool (Phase 2b S6).
//
// Per ADR-007 (Admin-confirmed 2026-05-15) and ADR-009 (Hub-/crashes-
// endpoint shape, Admin-confirmed via #023 briefing):
//
//   - Tool name:   crashes_list (snake_case <verb>_<noun>, no
//     `tracelab_` prefix — MCP ecosystem convention, see ADR-007).
//   - Input:       { "session_id": string (required),
//                    "limit"?: number }
//     limit follows the hub default (500) and cap (5000) per ADR-009.
//   - Output:      { "crashes": CrashEvent[] }
//     JSON-encoded TextContent (same pattern as sessions_list /
//     tail_since / adb_* — mcp-go v0.45.0 has no structured-result
//     type).
//   - Hub call:    GET /crashes?session=…&limit=… via
//                  internal/client.CrashesList. Bearer is attached by
//                  the *client.Client constructed in newServer.
//
// List semantics in one sentence: returns the session's crash digest
// newest-first, capped at limit; unknown session id renders as
// crashes:[] (the endpoint is a list-read, not a session-existence
// probe — ADR-009 Decision 2).
package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/VibeCodeSolutions/tracelab/internal/client"
)

// crashesListToolName is the canonical tool name; pinned in one place
// so tests, registration, and any future error message all reference
// the same literal.
const crashesListToolName = "crashes_list"

// crashesListDescription is the short description surfaced via
// tools/list. One sentence, mentions the two knobs (session_id/limit)
// so the Claude Code tool picker has enough context without consulting
// docs.
const crashesListDescription = "List crashes for a tracelab session, newest first. Requires session_id (string); optional limit (number, default 500, max 5000)."

// crashesListResult is the public output envelope. JSON-encoded into a
// single TextContent per ADR-007.
type crashesListResult struct {
	Crashes []client.CrashEvent `json:"crashes"`
}

// newCrashesListTool builds the ServerTool registered into the MCP
// server. The closure captures c so each invocation reuses the same
// hub client (bearer-bound at server-start).
func newCrashesListTool(c *client.Client) server.ServerTool {
	tool := mcp.NewTool(
		crashesListToolName,
		mcp.WithDescription(crashesListDescription),
		mcp.WithString("session_id",
			mcp.Description("Session ID (required). Discover available sessions via the sessions_list tool."),
			mcp.Required(),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of crashes to return. Hub default (500) applies when omitted; hub caps at 5000."),
			mcp.Min(1),
		),
	)
	return server.ServerTool{
		Tool:    tool,
		Handler: crashesListHandler(c),
	}
}

// crashesListHandler is the typed handler closure. Split from
// newCrashesListTool so unit tests can invoke it without driving the
// MCP dispatch path.
func crashesListHandler(c *client.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// session_id is required. mcp-go v0.45.0 does not enforce
		// required-fields at the dispatch layer (verified for the
		// sessions_list / tail_since tripwire tests); we fail-fast
		// here with a tool-result error so the consumer sees an
		// actionable message and no hub round-trip is wasted.
		sessionID := req.GetString("session_id", "")
		if sessionID == "" {
			return mcp.NewToolResultError("session_id required"), nil
		}

		// limit is optional. mcp-go's GetInt tolerates missing /
		// float64 / int — returns the default (0) when absent or
		// wrong-typed (see TestCrashesListInputSchemaWrongTypes
		// Tolerated tripwire). limit=0 → hub applies its default.
		limit := req.GetInt("limit", 0)

		crashes, err := c.CrashesList(ctx, sessionID, limit)
		if err != nil {
			return mcp.NewToolResultError(translateHubError(err)), nil
		}

		// Defensive: never marshal a nil slice as "null" — emit "[]".
		if crashes == nil {
			crashes = []client.CrashEvent{}
		}
		body, err := json.Marshal(crashesListResult{Crashes: crashes})
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("encode response: %v", err),
			), nil
		}
		return mcp.NewToolResultText(string(body)), nil
	}
}
