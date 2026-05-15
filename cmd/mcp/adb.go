// adb_devices / adb_start / adb_stop — three real MCP tools (Phase 2b S5).
//
// Per ADR-007 (Admin-confirmed 2026-05-15):
//
//   - Tool names: snake_case <noun>_<verb> for adb_devices, <noun>_<verb>
//     for adb_start / adb_stop. No `tracelab_` prefix (MCP ecosystem
//     convention).
//
//   - adb_devices:
//
//   - Input:    {} — no parameters.
//   - Output:   { "devices": ADBDevice[] }
//   - Hub call: GET /adb/devices via internal/client.ListADBDevices.
//
//   - adb_start:
//
//   - Input:    { "device_serial": string (required),
//     "tag_filter"?: string }
//     tag_filter is accepted at the tool surface for ADR-007
//     conformance but is currently NOT supported by the hub's
//     /adb/start endpoint — the bridge picks up tag_filter from
//     the hub's [adb] TOML configuration, not from the start
//     request. A non-empty tag_filter argument is accepted and
//     logged but does not influence the bridge. Tightening the
//     hub contract to surface tag_filter per-call is a future
//     sprint (tracked as a P2b-S5 open item — see WORKLOG #022).
//   - Output:   { "status": "started" | "already_running",
//     "device_serial": string }
//   - Hub call: POST /adb/start via internal/client.StartADBBridge.
//     The hub's status discriminator is passed through so MCP
//     consumers can distinguish a fresh start from an idempotent
//     ensure-running. Both map to a successful tool result.
//
//   - adb_stop:
//
//   - Input:    { "device_serial": string (required) }
//   - Output:   { "status": "stopped" | "not_running",
//     "device_serial": string }
//   - Hub call: POST /adb/stop via internal/client.StopADBBridge.
//     Idempotent: stopping a not-running bridge returns
//     status="not_running" with a successful tool result, mirroring
//     the hub's /adb/stop contract.
//
// Bearer auth: every hub call carries the bearer token resolved at
// process start via cliconfig (see main.go). Status discriminators flow
// from the hub's /adb/start and /adb/stop responses through the client
// (extended in P2b-S5 to return status as a string) to the tool result.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/VibeCodeSolutions/tracelab/internal/client"
)

// Tool name constants — pinned in one place so tests, registration, and
// any future error message all reference the same literal.
const (
	adbDevicesToolName = "adb_devices"
	adbStartToolName   = "adb_start"
	adbStopToolName    = "adb_stop"
)

// Tool description constants. One sentence each, mentioning the key
// knobs so the tools/list UX is informative without consulting docs.
const (
	adbDevicesDescription = "List the adb devices currently visible to the tracelab hub (no parameters)."
	adbStartDescription   = "Start an adb logcat bridge for a device. Requires device_serial (string); optional tag_filter (string) is accepted but currently ignored — bridge tag_filter is taken from the hub's [adb] config."
	adbStopDescription    = "Stop the adb logcat bridge for a device. Requires device_serial (string). Idempotent: a not-running bridge returns status \"not_running\"."
)

// adbDevicesResult is the public output envelope for adb_devices.
// JSON-encoded into a single TextContent per ADR-007.
type adbDevicesResult struct {
	Devices []client.ADBDevice `json:"devices"`
}

// adbStartResult is the public output envelope for adb_start. The hub-
// side status discriminator ("started" / "already_running") is passed
// through so consumers can distinguish a fresh start from an idempotent
// ensure-running.
type adbStartResult struct {
	Status       string `json:"status"`
	DeviceSerial string `json:"device_serial"`
}

// adbStopResult is the public output envelope for adb_stop. Mirrors
// adbStartResult: status is "stopped" or "not_running".
type adbStopResult struct {
	Status       string `json:"status"`
	DeviceSerial string `json:"device_serial"`
}

// newADBDevicesTool builds the ServerTool for adb_devices.
func newADBDevicesTool(c *client.Client) server.ServerTool {
	tool := mcp.NewTool(
		adbDevicesToolName,
		mcp.WithDescription(adbDevicesDescription),
	)
	return server.ServerTool{
		Tool:    tool,
		Handler: adbDevicesHandler(c),
	}
}

// adbDevicesHandler is the typed handler closure for adb_devices.
func adbDevicesHandler(c *client.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		devices, err := c.ListADBDevices(ctx)
		if err != nil {
			return mcp.NewToolResultError(translateHubError(err)), nil
		}
		// Defensive: never marshal a nil slice as "null" — emit "[]".
		// (The client already normalises, but a double-check here keeps
		// the tool contract self-evident.)
		if devices == nil {
			devices = []client.ADBDevice{}
		}
		body, err := json.Marshal(adbDevicesResult{Devices: devices})
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("encode response: %v", err),
			), nil
		}
		return mcp.NewToolResultText(string(body)), nil
	}
}

// newADBStartTool builds the ServerTool for adb_start.
func newADBStartTool(c *client.Client) server.ServerTool {
	tool := mcp.NewTool(
		adbStartToolName,
		mcp.WithDescription(adbStartDescription),
		mcp.WithString("device_serial",
			mcp.Description("ADB device serial (required). Discover available serials via the adb_devices tool."),
			mcp.Required(),
		),
		mcp.WithString("tag_filter",
			mcp.Description("Optional logcat tag filter. Accepted for ADR-007 conformance but currently ignored — bridge tag_filter is taken from the hub's [adb] TOML config."),
		),
	)
	return server.ServerTool{
		Tool:    tool,
		Handler: adbStartHandler(c),
	}
}

// adbStartHandler is the typed handler closure for adb_start.
func adbStartHandler(c *client.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// device_serial is required. mcp-go v0.45.0 does not enforce
		// required-fields at the dispatch layer (verified for the
		// sessions_list tripwire test); fail-fast here so the consumer
		// sees an actionable message and no hub round-trip is wasted.
		serial := req.GetString("device_serial", "")
		if serial == "" {
			return mcp.NewToolResultError("device_serial required"), nil
		}

		// tag_filter is accepted at the tool surface but NOT forwarded
		// to the hub — the hub's /adb/start endpoint does not carry a
		// tag_filter field today (the bridge takes tag_filter from
		// hub-side [adb] config). Log a warning so operators see the
		// drop, then proceed.
		if tagFilter := req.GetString("tag_filter", ""); tagFilter != "" {
			slog.Warn("adb_start: tag_filter argument ignored — hub /adb/start does not accept tag_filter; configure [adb].tag_filter in the hub's TOML instead",
				slog.String("tool", adbStartToolName),
				slog.String("device_serial", serial),
				slog.String("tag_filter", tagFilter),
			)
		}

		status, err := c.StartADBBridge(ctx, serial, "")
		if err != nil {
			return mcp.NewToolResultError(translateHubError(err)), nil
		}
		body, err := json.Marshal(adbStartResult{
			Status:       status,
			DeviceSerial: serial,
		})
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("encode response: %v", err),
			), nil
		}
		return mcp.NewToolResultText(string(body)), nil
	}
}

// newADBStopTool builds the ServerTool for adb_stop.
func newADBStopTool(c *client.Client) server.ServerTool {
	tool := mcp.NewTool(
		adbStopToolName,
		mcp.WithDescription(adbStopDescription),
		mcp.WithString("device_serial",
			mcp.Description("ADB device serial (required). Use adb_devices to inspect attached serials."),
			mcp.Required(),
		),
	)
	return server.ServerTool{
		Tool:    tool,
		Handler: adbStopHandler(c),
	}
}

// adbStopHandler is the typed handler closure for adb_stop.
func adbStopHandler(c *client.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		serial := req.GetString("device_serial", "")
		if serial == "" {
			return mcp.NewToolResultError("device_serial required"), nil
		}

		status, err := c.StopADBBridge(ctx, serial)
		if err != nil {
			return mcp.NewToolResultError(translateHubError(err)), nil
		}
		body, err := json.Marshal(adbStopResult{
			Status:       status,
			DeviceSerial: serial,
		})
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("encode response: %v", err),
			), nil
		}
		return mcp.NewToolResultText(string(body)), nil
	}
}
