// Command tracelab-mcp is the Tracelab MCP server.
//
// Phase 2b S1 (skeleton): the binary starts, registers four placeholder
// tools (sessions / tail / crashes / adb) on a stdio transport, and exits
// cleanly. Every tool handler returns a structured "not implemented yet"
// error pointing at ADR-007; no behaviour is wired and no client traffic
// reaches `internal/client/` yet.
//
// What works:
//   - `--version` prints the build version (overridden via -ldflags
//     "-X main.version=...", same convention as cmd/cli and cmd/hub).
//   - Server constructs and registers the four placeholder tools.
//   - stdio transport (MCP default for local consumption by Claude Code)
//     starts and shuts down on EOF / context-cancel.
//
// What does NOT work yet:
//   - Tool names carry a `_stub` suffix on purpose. Final naming, JSON-Schema
//     surface, auth wiring, and the tool-vs-resource decision for `tail`
//     are an explicit S2 deliverable per ADR-007 and the Phase 2b plan.
//   - No `internal/client/` integration. The handlers return an error;
//     they do not call the hub.
//   - `crashes_stub` corresponds to a hub endpoint that does not exist
//     today (ADR-007 S6 risk, additive Hub-Schema-Change required —
//     Admin-confirm before S6 starts).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// version is overridden at build time via -ldflags "-X main.version=...".
// Shared VERSION variable in the Makefile covers hub/cli/mcp from one source.
var version = "0.1.0-dev"

// serverName is the MCP server identity advertised in the initialize
// handshake. Kept stable across versions so MCP clients can pin by name.
const serverName = "tracelab-mcp"

// stubNotImplemented is the canonical error message every S1 placeholder
// handler returns. Kept in one constant so the smoke test can assert the
// shape and a future S2 sweep can grep for the marker cleanly.
const stubNotImplemented = "not implemented yet — placeholder stub, see docs/ARCH.md ADR-007 (final tool surface decided in Phase 2b S2)"

// stubTool describes one placeholder tool. The `_stub` suffix on the name
// is intentional — final naming is an S2 decision and grep-able today.
type stubTool struct {
	name        string
	description string
}

// stubTools is the registration source of truth. Ordered to mirror the
// ADR-007 tool inventory (sessions / tail / crashes / adb).
var stubTools = []stubTool{
	{
		name:        "sessions_stub",
		description: "Placeholder for the sessions tool (list/get). Final shape decided in Phase 2b S2.",
	},
	{
		name:        "tail_stub",
		description: "Placeholder for the tail tool (live event stream). Tool-vs-resource decision deferred to Phase 2b S2.",
	},
	{
		name:        "crashes_stub",
		description: "Placeholder for the crashes tool. Requires hub-side /crashes endpoint (ADR-007 S6 risk, Admin-confirm pending).",
	},
	{
		name:        "adb_stub",
		description: "Placeholder for the adb tool (devices/start/stop). Will reuse the hub /adb/* surface from ADR-004 Option B.",
	},
}

// stubHandler returns a uniform "not implemented" error for every S1 tool.
// It is intentionally context-less and request-less: S1 only proves the
// registration path works, not any behaviour.
func stubHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError(stubNotImplemented), nil
}

// newServer builds and configures the MCP server with the four placeholder
// tools registered. Split out from main() so the smoke test can construct
// the server without hitting stdio.
func newServer() *server.MCPServer {
	s := server.NewMCPServer(
		serverName,
		version,
		server.WithToolCapabilities(false),
	)
	for _, st := range stubTools {
		s.AddTool(
			mcp.NewTool(st.name, mcp.WithDescription(st.description)),
			stubHandler,
		)
	}
	return s
}

func main() {
	fs := flag.NewFlagSet("tracelab-mcp", flag.ExitOnError)
	showVersion := fs.Bool("version", false, "print version and exit")
	// Allow `--version` (double-dash) too, matching the cobra-style UX of
	// cmd/cli / cmd/hub. Go's `flag` accepts both `-version` and `--version`
	// by default; we only need to surface the flag once.
	if err := fs.Parse(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "tracelab-mcp:", err)
		os.Exit(2)
	}
	if *showVersion {
		fmt.Println(version)
		return
	}

	if err := server.ServeStdio(newServer()); err != nil {
		fmt.Fprintln(os.Stderr, "tracelab-mcp:", err)
		os.Exit(1)
	}
}
