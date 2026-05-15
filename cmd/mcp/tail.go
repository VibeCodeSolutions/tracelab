// tail_since — second real MCP tool (Phase 2b S4).
//
// Per ADR-007 (Admin-confirmed 2026-05-15) and ADR-008 (Hub-/events-
// endpoint shape, Admin-confirmed via #021 briefing):
//
//   - Tool name:   tail_since (snake_case <verb>_<noun>, no `tracelab_`
//     prefix — MCP ecosystem convention, see ADR-007).
//   - Input:       { "session": string (required),
//                    "since_seq"?: number,
//                    "limit"?: number }
//     since_seq is the opaque int64 cursor returned by the previous
//     call's next_since_seq; 0 / absent means "start from the earliest
//     event". limit follows the hub default (500) and cap (5000) per
//     ADR-008.
//   - Output:      { "events": Event[], "next_since_seq": number }
//     JSON-encoded TextContent (same pattern as sessions_list — mcp-go
//     v0.45.0 has no structured-result type).
//   - Hub call:    GET /events?session=…&since_seq=…&limit=… via
//                  internal/client.EventsSince. Bearer is attached by
//                  the *client.Client constructed in newServer.
//
// Cursor semantics in one sentence: walk forward by feeding the
// previous response's next_since_seq back into the next call's
// since_seq; empty results echo the caller's cursor (stable-on-empty),
// so a polling loop never spins backwards.
package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/VibeCodeSolutions/tracelab/internal/client"
)

// tailSinceToolName is the canonical tool name; pinned in one place so
// tests, registration, and any future error message all reference the
// same literal.
const tailSinceToolName = "tail_since"

// tailSinceDescription is the short description surfaced via tools/list.
// One sentence, mentions the three knobs (session/since_seq/limit) so
// the Claude Code tool picker has enough context without consulting
// docs.
const tailSinceDescription = "Fetch the next page of tracelab events for a session. Requires session (string); optional since_seq (number cursor from previous call) and limit (number, default 500, max 5000)."

// tailSinceResult is the public output envelope. JSON-encoded into a
// single TextContent per ADR-007.
type tailSinceResult struct {
	Events       []client.Event `json:"events"`
	NextSinceSeq int64          `json:"next_since_seq"`
}

// newTailSinceTool builds the ServerTool registered into the MCP
// server. The closure captures c so each invocation reuses the same
// hub client (bearer-bound at server-start).
func newTailSinceTool(c *client.Client) server.ServerTool {
	tool := mcp.NewTool(
		tailSinceToolName,
		mcp.WithDescription(tailSinceDescription),
		mcp.WithString("session",
			mcp.Description("Session ID (required). Discover available sessions via the sessions_list tool."),
			mcp.Required(),
		),
		mcp.WithNumber("since_seq",
			mcp.Description("Opaque int64 cursor from the previous call's next_since_seq. Omit (or pass 0) to start from the earliest event in the session."),
			mcp.Min(0),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum events to return. Hub default (500) applies when omitted; hub caps at 5000."),
			mcp.Min(1),
		),
	)
	return server.ServerTool{
		Tool:    tool,
		Handler: tailSinceHandler(c),
	}
}

// tailSinceHandler is the typed handler closure. Split from
// newTailSinceTool so unit tests can invoke it without driving the MCP
// dispatch path.
func tailSinceHandler(c *client.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// session is required. mcp-go v0.45.0 does not enforce
		// required-fields at the dispatch layer (verified for the
		// sessions_list tripwire test); we fail-fast here with a
		// tool-result error so the consumer sees an actionable
		// message and no hub round-trip is wasted.
		session := req.GetString("session", "")
		if session == "" {
			return mcp.NewToolResultError("session required"), nil
		}

		// since_seq / limit are optional. mcp-go's GetInt tolerates
		// missing / float64 / int — returns the default (0) when
		// absent or wrong-typed (see TestTailSince_InputSchema_
		// WrongTypesTolerated for the tripwire).
		sinceSeq := int64(req.GetInt("since_seq", 0))
		limit := req.GetInt("limit", 0)

		events, next, err := c.EventsSince(ctx, session, sinceSeq, limit)
		if err != nil {
			return mcp.NewToolResultError(translateHubError(err)), nil
		}

		// Defensive: never marshal a nil slice as "null" — emit "[]".
		if events == nil {
			events = []client.Event{}
		}
		body, err := json.Marshal(tailSinceResult{
			Events:       events,
			NextSinceSeq: next,
		})
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("encode response: %v", err),
			), nil
		}
		return mcp.NewToolResultText(string(body)), nil
	}
}
