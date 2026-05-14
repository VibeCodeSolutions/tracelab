package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/VibeCodeSolutions/tracelab/internal/client"
	"github.com/VibeCodeSolutions/tracelab/internal/cliconfig"
)

// adbTimeout bounds each `tracelab adb …` round-trip. Devices is bounded
// hub-side by a 5s adb-probe timeout; start/stop are short manager calls.
// Keeping a single CLI-side cap simplifies reasoning and matches the
// session sub-command's pattern.
const adbTimeout = 10 * time.Second

// adbDevicesFlags bundles the per-invocation knobs for `adb devices`.
type adbDevicesFlags struct {
	format string
}

// newADBCmd returns the wired "adb" sub-command and its three sub-sub-cmds:
//
//   - tracelab adb devices [--format=table|json]
//   - tracelab adb start <serial> [--session=<id>]
//   - tracelab adb stop <serial>
//
// All three drive the hub's /adb/* HTTP endpoints (ADR-004 Option B,
// landed in S5). Auth, URL, and timeout discovery flow through the shared
// cliconfig.Resolve helper — the same pipeline `sessions` and `tail` use,
// no duplication.
func newADBCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "adb",
		Short: "Manage hub-side adb bridges",
		Long: `Manage hub-side adb bridges (ADR-004 Option B).

The hub talks to adb on its own host and runs a logcat bridge per device
serial; this sub-command surfaces three lifecycle operations:

  tracelab adb devices            list attached adb devices
  tracelab adb start <serial>     start a logcat bridge for the given device
  tracelab adb stop  <serial>     stop the bridge for the given device

Bearer token and hub URL are resolved via the same tracelab.toml /
TRACELAB_URL / TRACELAB_TOKEN pipeline the other sub-commands use.`,
	}
	cmd.AddCommand(newADBDevicesCmd())
	cmd.AddCommand(newADBStartCmd())
	cmd.AddCommand(newADBStopCmd())
	return cmd
}

// newADBDevicesCmd returns `tracelab adb devices`.
func newADBDevicesCmd() *cobra.Command {
	var flags adbDevicesFlags
	cmd := &cobra.Command{
		Use:   "devices",
		Short: "List attached adb devices as the hub sees them",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runADBDevices(cmd, flags)
		},
	}
	cmd.Flags().StringVarP(&flags.format, "format", "f", "",
		"output format: table | json (default from [cli].default_format, falls back to table)")
	return cmd
}

// newADBStartCmd returns `tracelab adb start <serial>`.
func newADBStartCmd() *cobra.Command {
	var sessionID string
	cmd := &cobra.Command{
		Use:   "start <serial>",
		Short: "Start a hub-managed adb logcat bridge for <serial>",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runADBStart(cmd, args[0], sessionID)
		},
	}
	cmd.Flags().StringVarP(&sessionID, "session", "s", "",
		"optional session id hint for the hub (informational)")
	return cmd
}

// newADBStopCmd returns `tracelab adb stop <serial>`.
func newADBStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <serial>",
		Short: "Stop the hub-managed adb logcat bridge for <serial>",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runADBStop(cmd, args[0])
		},
	}
}

// resolveADBClient is the shared "config-resolve + client.New" prelude
// used by all three sub-sub-cmds. Returns the constructed client plus the
// resolved record (kept for translateClientError, which embeds the hub
// BaseURL into the user-facing message).
func resolveADBClient(cmd *cobra.Command) (*client.Client, *cliconfig.Resolved, error) {
	root := cmd.Root()
	cfgPath, _ := root.PersistentFlags().GetString("config")
	flagURL, _ := root.PersistentFlags().GetString("url")
	flagToken, _ := root.PersistentFlags().GetString("token")

	resolved, err := cliconfig.Resolve(cliconfig.Sources{
		FlagConfigPath: cfgPath,
		FlagURL:        flagURL,
		FlagToken:      flagToken,
	})
	if err != nil {
		return nil, nil, userError(err.Error())
	}

	c, err := client.New(client.Config{
		BaseURL: resolved.BaseURL,
		Token:   resolved.Token,
		Timeout: adbTimeout,
	})
	if err != nil {
		return nil, nil, userError(err.Error())
	}
	return c, resolved, nil
}

// runADBDevices is the testable core of `tracelab adb devices`.
func runADBDevices(cmd *cobra.Command, flags adbDevicesFlags) error {
	c, resolved, err := resolveADBClient(cmd)
	if err != nil {
		return err
	}

	format := flags.format
	if format == "" {
		format = resolved.CLI.DefaultFormat
	}
	if format != formatTable && format != formatJSON {
		return userError(fmt.Sprintf("invalid --format %q (must be table or json)", format))
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), adbTimeout)
	defer cancel()

	devices, err := c.ListADBDevices(ctx)
	if err != nil {
		return translateClientError(err, resolved)
	}

	out := cmd.OutOrStdout()
	switch format {
	case formatTable:
		return writeADBDevicesTable(out, devices)
	case formatJSON:
		return writeADBDevicesJSON(out, devices)
	}
	return nil // unreachable — guarded above
}

// runADBStart is the testable core of `tracelab adb start <serial>`.
func runADBStart(cmd *cobra.Command, serial, sessionID string) error {
	c, resolved, err := resolveADBClient(cmd)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), adbTimeout)
	defer cancel()

	if err := c.StartADBBridge(ctx, serial, sessionID); err != nil {
		return translateClientError(err, resolved)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "bridge started for %s\n", serial)
	return nil
}

// runADBStop is the testable core of `tracelab adb stop <serial>`.
func runADBStop(cmd *cobra.Command, serial string) error {
	c, resolved, err := resolveADBClient(cmd)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), adbTimeout)
	defer cancel()

	if err := c.StopADBBridge(ctx, serial); err != nil {
		return translateClientError(err, resolved)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "bridge stopped for %s\n", serial)
	return nil
}

// writeADBDevicesTable renders the table format: SERIAL, STATE, MODEL.
// Columns are tab-separated and rendered through text/tabwriter so they
// align on stdout even when device entries vary widely in width.
func writeADBDevicesTable(w io.Writer, devices []client.ADBDevice) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SERIAL\tSTATE\tMODEL")
	for _, d := range devices {
		model := d.Model
		if model == "" {
			model = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", d.Serial, d.State, model)
	}
	return tw.Flush()
}

// writeADBDevicesJSON renders the JSON format: an array of ADBDevice DTOs
// pretty-printed with two-space indent. Empty input is encoded as `[]`
// (never `null`) so consumers can range without a nil check.
func writeADBDevicesJSON(w io.Writer, devices []client.ADBDevice) error {
	if devices == nil {
		devices = []client.ADBDevice{}
	}
	buf, err := json.MarshalIndent(devices, "", "  ")
	if err != nil {
		return fmt.Errorf("cli: encode json: %w", err)
	}
	if _, err := w.Write(append(buf, '\n')); err != nil {
		return err
	}
	return nil
}
