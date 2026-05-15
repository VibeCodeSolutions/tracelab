// Command tracelab-mcp is the Tracelab MCP server.
//
// Phase 2b status:
//
//   - S1 (skeleton): four placeholder tools on a stdio transport.
//   - S2 (ADR-007):  final tool surface pinned (sessions_list, tail_*,
//     crashes_*, adb_*).
//   - S3 (current):  sessions_list is the first real tool — registration,
//     bearer-auth plumbing, client-side since-filter, hub
//     wiring against internal/client.ListSessions.
//
// Stubs still in place: adb_stub, crashes_stub, tail_stub. They are
// replaced sucessively in S4 (tail), S5 (crashes — pending Hub-Schema
// change) and S6 (adb).
//
// Bearer-auth wiring (per ADR-007):
//
//   - At process start, newServer calls cliconfig.Resolve(Sources{}) with
//     all override fields empty. That picks up the bearer token via the
//     5-step ADR-002 discovery order (--config flag, $TRACELAB_CONFIG,
//     ./tracelab.toml, $XDG_CONFIG_HOME/tracelab/tracelab.toml,
//     ~/.config/tracelab/tracelab.toml).
//   - The resolved URL + token are passed to client.New; the same Client
//     is reused for every tool invocation (the *Client is concurrency-
//     safe per its package doc).
//   - When no token is configured (ErrNoToken / CHANGEME placeholder),
//     the process exits with a log.Fatal naming the discovery paths so
//     operators get an actionable hint without a stack trace.
//
// Build-time --version is overridden via -ldflags "-X main.version=...".
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/VibeCodeSolutions/tracelab/internal/client"
	"github.com/VibeCodeSolutions/tracelab/internal/cliconfig"
)

// version is overridden at build time via -ldflags "-X main.version=...".
// Shared VERSION variable in the Makefile covers hub/cli/mcp from one source.
var version = "0.1.0-dev"

// serverName is the MCP server identity advertised in the initialize
// handshake. Kept stable across versions so MCP clients can pin by name.
const serverName = "tracelab-mcp"

// hubTimeout caps every hub round-trip from the MCP layer. Same envelope
// as cmd/cli's sessionTimeout — long enough for an over-the-LAN hub,
// short enough for "is the hub up?" to fail fast on the LLM side.
const hubTimeout = 30 * time.Second

// stubNotImplemented is the canonical error message every remaining
// placeholder handler returns. Once tail/crashes/adb land (S4-S6) this
// constant and the matching stubTools entries will retire.
const stubNotImplemented = "not implemented yet — placeholder stub, see docs/ARCH.md ADR-007 (final tool surface decided in Phase 2b S2)"

// stubTool describes one placeholder tool.
type stubTool struct {
	name        string
	description string
}

// stubTools is the registration source of truth for the remaining
// placeholders. sessions_list (S3, this commit) moved out into
// cmd/mcp/sessions.go; only adb / crashes / tail remain as stubs.
var stubTools = []stubTool{
	{
		name:        "tail_stub",
		description: "Placeholder for the tail tool (live event stream). Tool-vs-resource decision deferred to Phase 2b S4.",
	},
	{
		name:        "crashes_stub",
		description: "Placeholder for the crashes tool. Requires hub-side /crashes endpoint (ADR-007 S5 risk, Admin-confirm pending).",
	},
	{
		name:        "adb_stub",
		description: "Placeholder for the adb tool (devices/start/stop). Will reuse the hub /adb/* surface from ADR-004 Option B.",
	},
}

// stubHandler returns a uniform "not implemented" error for every
// remaining placeholder. Context-less and request-less on purpose: the
// stubs only prove the registration path; behaviour lives in the real
// per-tool handlers (see cmd/mcp/sessions.go for the S3 pattern).
func stubHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError(stubNotImplemented), nil
}

// newServer builds and configures the MCP server for production use.
//
// Behaviour:
//
//   - Resolves the hub URL + bearer via cliconfig (ADR-002 discovery).
//   - Constructs the shared *client.Client (concurrency-safe; one
//     instance for every tool invocation).
//   - Registers the real sessions_list tool plus the remaining stub
//     placeholders.
//
// On config errors (no token, no URL, missing config file pointed at by
// --config / $TRACELAB_CONFIG) the function calls log.Fatal — main()
// never sees the error because there is nothing useful to do with it at
// the stdio transport layer.
func newServer() *server.MCPServer {
	resolved, err := cliconfig.Resolve(cliconfig.Sources{})
	if err != nil {
		// Single-line, actionable error. cliconfig.ErrNoToken /
		// ErrNoURL already enumerate the relevant discovery paths.
		log.Fatalf("tracelab-mcp: %v", err)
	}

	hubClient, err := client.New(client.Config{
		BaseURL: resolved.BaseURL,
		Token:   resolved.Token,
		Timeout: hubTimeout,
	})
	if err != nil {
		log.Fatalf("tracelab-mcp: %v", err)
	}

	return buildServer(hubClient)
}

// buildServer is the assembly step factored out of newServer so the
// smoke tests can construct a server against a httptest-backed Client
// without touching cliconfig discovery.
func buildServer(hubClient *client.Client) *server.MCPServer {
	s := server.NewMCPServer(
		serverName,
		version,
		server.WithToolCapabilities(false),
	)

	// Real tools (S3+). AddTools is the variadic ServerTool registration
	// path; we use it because newSessionsListTool returns a ServerTool
	// (Tool + Handler bundled).
	s.AddTools(newSessionsListTool(hubClient))

	// Remaining stubs (replaced in S4 / S5 / S6).
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
