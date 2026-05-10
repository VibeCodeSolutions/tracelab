// Package adb is a thin Go wrapper around the `adb` command-line tool.
//
// It currently exposes two pieces of functionality used by the tracelab hub:
//
//   - Devices: a snapshot of attached Android devices via `adb devices -l`.
//   - LogcatStream: a context-cancellable stream of parsed logcat lines from
//     a given device via `adb -s <serial> logcat -v threadtime`.
//
// The package shells out to a real `adb` binary located via $PATH. No CGO,
// no transport-level ADB re-implementation — the upstream tool already
// does that job well and ships on every dev box that talks to Android
// hardware. Tests that need to redirect the binary use t.Setenv("PATH",
// ...) to inject a fake adb into PATH; there is no production-visible
// override hook.
//
// Lifecycle: callers own the context. Cancelling the context passed to
// LogcatStream causes the underlying adb subprocess to be terminated
// and the returned channel to be closed by the reader goroutine.
// Termination is POSIX-first: SIGTERM, escalated to SIGKILL after 3s.
// On Windows, Process.Signal(SIGTERM) is a no-op (returns an error that
// is intentionally ignored) and the 3-second grace period elapses before
// Process.Kill (TerminateProcess) ends the subprocess — i.e. cancellation
// on Windows is functionally a hard kill with a delay, not a graceful
// shutdown. Devices() uses a short timeout (10s by default) so a hung
// adb daemon can't block the caller forever.
package adb

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// adbBinary is the executable name passed to exec.LookPath. The test
// suite injects a fake by manipulating PATH (see withFakeAdb in
// adb_test.go); there is no production override hook. A test-only
// helper for swapping the binary name lives in export_test.go and only
// exists during `go test`.
var (
	adbBinaryMu sync.RWMutex
	adbBinary   = "adb"
)

func currentBinary() string {
	adbBinaryMu.RLock()
	defer adbBinaryMu.RUnlock()
	return adbBinary
}

// defaultRunTimeout bounds how long a one-shot adb invocation (e.g.
// `adb devices -l`) is allowed to take before runAdb cancels it.
const defaultRunTimeout = 10 * time.Second

// killGracePeriod is how long LogcatStream waits between SIGTERM and
// the SIGKILL escalation when the caller cancels the context.
const killGracePeriod = 3 * time.Second

// Device describes one entry from `adb devices -l`. All fields beyond
// Serial and State are best-effort — adb only emits them when known.
//
// Example raw line (single-device default emulator):
//
//	emulator-5554 device product:sdk_gphone64_x86_64 model:sdk_gphone64_x86_64 device:emu64x transport_id:1
type Device struct {
	// Serial is the adb device identifier (e.g. "emulator-5554",
	// "AB12CD34", "192.168.1.42:5555"). Always populated.
	Serial string
	// State is the connection state reported by adb: "device",
	// "offline", "unauthorized", "no permissions", "recovery",
	// "sideload", "bootloader". Always populated.
	State string
	// Transport is the transport kind when reported (e.g. "usb").
	// Empty if adb did not include it.
	Transport string
	// Product is the Android product name (e.g. "sdk_gphone64_x86_64").
	Product string
	// Model is the human-readable device model.
	Model string
	// Device is the internal device codename (e.g. "emu64x").
	Device string
	// TransportID is the numeric transport id assigned by the local
	// adb server. Zero if not reported by adb.
	TransportID int
}

// LogcatLine is one parsed logcat record in `threadtime` format.
//
// Sample raw line:
//
//	06-01 12:34:56.789  1234  5678 I MyTag   : hello world
//
// The timestamp comes without a year, so callers should treat
// LogcatLine.Timestamp as a "current-year" value — the parser fills in
// time.Now().Year() as a sane default.
type LogcatLine struct {
	// Timestamp parsed from the line. Year is the current local year
	// because logcat does not emit one.
	Timestamp time.Time
	// PID is the process id reported by logcat.
	PID int
	// TID is the thread id reported by logcat.
	TID int
	// Level is the single-letter priority (V, D, I, W, E, F, S).
	Level rune
	// Tag is the logcat tag (whitespace-trimmed).
	Tag string
	// Message is the rest of the line after `Tag: `.
	Message string
}

// Devices runs `adb devices -l` and parses the result.
//
// Edge cases handled:
//   - empty device list (only the "List of devices attached" header line)
//   - "* daemon not running" / "* daemon started successfully" preamble
//     emitted when the local adb server has to be auto-started
//   - mixed states (device / offline / unauthorized) on the same line
//   - extra `-l` key=value pairs in any order
func Devices(ctx context.Context) ([]Device, error) {
	out, err := runAdb(ctx, "devices", "-l")
	if err != nil {
		return nil, err
	}
	return parseDevices(out)
}

// parseDevices is exported via tests through Devices; kept private so
// the public surface stays narrow.
func parseDevices(raw []byte) ([]Device, error) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	// adb output is small but a logcat-style framework reuses the
	// same buffer; bump it for safety on weird daemon banners.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var devices []Device
	headerSeen := false
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Daemon preamble lines start with "* ". Skip them silently;
		// they are noise from the local adb server bootstrap.
		if strings.HasPrefix(trimmed, "* ") {
			continue
		}
		if !headerSeen {
			// The header is "List of devices attached". Anything
			// before it is preamble we already filtered above; once
			// we see it, switch to parsing device rows.
			if strings.HasPrefix(trimmed, "List of devices") {
				headerSeen = true
				continue
			}
			// Tolerate missing header (some adb versions in CI
			// containers emit only device rows). Fall through to
			// parsing this line as a device entry.
			headerSeen = true
		}
		dev, ok := parseDeviceLine(trimmed)
		if !ok {
			// Unparseable row — ignore rather than fail the whole
			// listing. Surfacing this as a hard error would make
			// `Devices()` brittle against future adb additions.
			continue
		}
		devices = append(devices, dev)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("adb: scan devices output: %w", err)
	}
	return devices, nil
}

// parseDeviceLine parses one `adb devices -l` row. Format:
//
//	<serial> <state> [key:value ...]
//
// Most states are single-word ("device", "offline", "unauthorized",
// "recovery", "sideload", "bootloader") but adb emits at least one
// multi-word state in the wild — "no permissions" — and follows it with
// a free-form, semicolon-prefixed hint such as:
//
//	0123456789ABCDEF  no permissions; user in plugdev group; see [http://...]
//
// Treating that line with naive Fields-splitting would pin State="no"
// and pollute the key:value scan with permission-text tokens. The fix:
// detect known multi-word states explicitly and consume the trailer up
// to the next genuine key:value token (or EOL), then let the key:value
// loop process whatever remains. Unknown trailing tokens that don't
// contain a ':' are silently ignored, matching the original tolerance.
//
// Returns (zero, false) if the line does not have the minimum two
// whitespace-separated tokens.
func parseDeviceLine(line string) (Device, bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return Device{}, false
	}
	dev := Device{Serial: fields[0]}

	// State + index of the first token after the state. By default the
	// state is one word at fields[1]; multi-word states extend this.
	state, kvStart := extractState(fields[1:])
	dev.State = state
	kvStart++ // account for fields[0] (serial) we sliced past above
	for _, kv := range fields[kvStart:] {
		idx := strings.IndexByte(kv, ':')
		if idx <= 0 || idx == len(kv)-1 {
			// Free-form trailer tokens after a multi-word state (e.g.
			// "user", "in", "plugdev") fall through here and are
			// dropped. Same path silently skips anything else that
			// doesn't carry a ':' value pair.
			continue
		}
		key, val := kv[:idx], kv[idx+1:]
		switch key {
		case "transport":
			dev.Transport = val
		case "product":
			dev.Product = val
		case "model":
			dev.Model = val
		case "device":
			dev.Device = val
		case "transport_id":
			n, err := strconv.Atoi(val)
			if err == nil {
				dev.TransportID = n
			}
		}
	}
	return dev, true
}

// extractState resolves the device state from the fields slice that
// starts immediately after the serial. It returns the canonical state
// string and the index (within the input slice) of the first token
// after the state. For the common single-word case the state is
// fields[0] and the returned index is 1. For known multi-word states
// it collapses the words and additionally skips any free-form trailer
// tokens that follow until the next key:value pair or end of line.
//
// The list is deliberately small — only states adb is known to emit
// as multi-word. New entries here should match upstream's
// transport_state_name table.
func extractState(fields []string) (state string, next int) {
	if len(fields) == 0 {
		return "", 0
	}
	// Two-word states. The cheapest robust check is a literal table:
	// adb emits a tiny finite set, no need for regex or generic logic.
	if len(fields) >= 2 {
		two := fields[0] + " " + strings.TrimRight(fields[1], ";")
		switch two {
		case "no permissions":
			// Trailer after "no permissions" is free-form text like
			// "; user in plugdev group; see [http://...]" — consume
			// tokens until we hit a real key:value pair.
			idx := 2
			for idx < len(fields) {
				if isKeyValueToken(fields[idx]) {
					break
				}
				idx++
			}
			return two, idx
		}
	}
	// Default: single-word state.
	return fields[0], 1
}

// isKeyValueToken reports whether tok looks like a `key:value` pair
// emitted by `adb devices -l` (e.g. `product:foo`, `transport_id:1`).
// We use the same minimal heuristic as the kv-loop: a non-empty key,
// a single ':' at a non-edge position, and a non-empty value.
func isKeyValueToken(tok string) bool {
	idx := strings.IndexByte(tok, ':')
	if idx <= 0 || idx == len(tok)-1 {
		return false
	}
	// Reject `http://` style URLs by requiring the key prefix to be
	// alpha/underscore only — adb keys are simple identifiers.
	for i := 0; i < idx; i++ {
		c := tok[i]
		if !(c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}
	return true
}

// LogcatStream starts `adb -s <serial> logcat -v threadtime [<tagFilter>:V *:S]`
// and returns a channel that emits one parsed LogcatLine per logcat record.
//
// Cancellation: when ctx is cancelled the underlying adb subprocess is sent
// SIGTERM; if it does not exit within killGracePeriod (3s), SIGKILL is sent.
// On Windows, SIGTERM is not deliverable (Process.Signal returns an
// ignored error) and the kill path runs after the 3s grace period via
// Process.Kill (TerminateProcess). The reader goroutine drains and
// closes the returned channel before returning, so receivers can safely
// range over it.
//
// Errors during startup (binary not found, exec.Start failure) are returned
// synchronously. Errors during streaming (parse failure, subprocess exit
// with non-zero status) are logged via slog and cause the channel to close.
//
// deviceSerial may be empty, in which case adb picks "the only attached
// device" (and errors if there is more than one).
func LogcatStream(ctx context.Context, deviceSerial, tagFilter string) (<-chan LogcatLine, error) {
	binary, err := exec.LookPath(currentBinary())
	if err != nil {
		return nil, fmt.Errorf("adb: lookup %q: %w", currentBinary(), err)
	}

	args := make([]string, 0, 8)
	if deviceSerial != "" {
		args = append(args, "-s", deviceSerial)
	}
	args = append(args, "logcat", "-v", "threadtime")
	if tagFilter != "" {
		// Only show records for tagFilter at any level, silence the rest.
		args = append(args, tagFilter+":V", "*:S")
	}

	// We deliberately do *not* use exec.CommandContext here because its
	// default cancellation semantic is os.Process.Kill (SIGKILL on Unix),
	// and we want a graceful SIGTERM-then-SIGKILL escalation. We manage
	// the lifecycle by hand below.
	cmd := exec.Command(binary, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("adb: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdout.Close()
		return nil, fmt.Errorf("adb: stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, fmt.Errorf("adb: start logcat: %w", err)
	}

	out := make(chan LogcatLine, 64)

	// Stderr drainer: we don't surface stderr to callers but we do log it
	// so daemon-side debugging is possible. Owns its own goroutine,
	// terminates when the pipe is closed (which happens on cmd.Wait).
	go func() {
		buf, _ := io.ReadAll(stderr)
		if len(buf) > 0 {
			slog.Debug("adb logcat stderr",
				slog.String("serial", deviceSerial),
				slog.String("output", strings.TrimSpace(string(buf))),
			)
		}
	}()

	// Cancel watcher: blocks on ctx.Done, then escalates SIGTERM → SIGKILL
	// (POSIX); on Windows the SIGTERM call no-ops and the kill path runs
	// after killGracePeriod. Owned by the goroutine itself; exits when the
	// watched ctx fires *or* when stopCancel is closed (which the reader
	// goroutine does on natural subprocess exit, to release this goroutine).
	stopCancel := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
		case <-stopCancel:
			return
		}
		// Best-effort SIGTERM. If the process is already gone, the
		// signal call returns an error we don't care about.
		if cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGTERM)
		}
		select {
		case <-stopCancel:
			return
		case <-time.After(killGracePeriod):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		}
	}()

	// Reader goroutine: parses lines until stdout EOFs or parse fails,
	// then waits for the subprocess and closes the output channel. This
	// is the *single* owner of the channel close.
	go func() {
		defer close(out)
		defer close(stopCancel)

		scanner := bufio.NewScanner(stdout)
		// logcat lines are typically <512B, but a wedged process can
		// emit very long messages. 1MB is generous but bounded.
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()
			rec, ok := parseLogcatLine(line)
			if !ok {
				// Skip non-threadtime lines (logcat banners,
				// "--------- beginning of main", reset markers).
				continue
			}
			select {
			case out <- rec:
			case <-ctx.Done():
				// Drain stdout in the background so the
				// subprocess doesn't block on a full pipe;
				// cmd.Wait below collects it.
				go io.Copy(io.Discard, stdout)
				goto done
			}
		}
		if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
			slog.Debug("adb logcat scan error",
				slog.String("serial", deviceSerial),
				slog.Any("error", err),
			)
		}

	done:
		// cmd.Wait reaps the process and releases all pipe FDs. It
		// returns *exec.ExitError on non-zero exit, which we surface
		// as a debug log because cancel-by-signal is the normal path
		// and we don't want to spam.
		if err := cmd.Wait(); err != nil {
			if ctx.Err() != nil {
				slog.Debug("adb logcat exited (context cancelled)",
					slog.String("serial", deviceSerial),
					slog.Any("error", err),
				)
			} else {
				slog.Warn("adb logcat exited unexpectedly",
					slog.String("serial", deviceSerial),
					slog.Any("error", err),
				)
			}
		}
	}()

	return out, nil
}

// parseLogcatLine parses one threadtime-formatted line into a LogcatLine.
// Returns (zero, false) for lines that don't match the format (banners,
// "--------- beginning of main", blank lines).
//
// threadtime format:
//
//	MM-DD HH:MM:SS.sss  PID  TID L Tag: Message
//
// where columns are whitespace-separated (variable spacing for alignment).
func parseLogcatLine(line string) (LogcatLine, bool) {
	// Strip the trailing CR if adb is somehow connected via a Windows
	// terminal that adds \r\n. logcat itself uses \n.
	line = strings.TrimRight(line, "\r")
	if len(line) < 19 {
		return LogcatLine{}, false
	}
	// "MM-DD" = 5, " " = 1, "HH:MM:SS.sss" = 12 → first 18 chars
	// are timestamp + space. Quick structural check on the dashes
	// and colons before we parse, to discard banners cheaply.
	if line[2] != '-' || line[5] != ' ' || line[8] != ':' || line[11] != ':' {
		return LogcatLine{}, false
	}

	// Parse timestamp.
	const tsLayout = "01-02 15:04:05.000"
	tsStr := line[:18]
	ts, err := time.ParseInLocation(tsLayout, tsStr, time.Local)
	if err != nil {
		return LogcatLine{}, false
	}
	// logcat omits the year; fill in current year so callers can use
	// the timestamp as-is for "recent past" reasoning. This is wrong
	// across year boundaries by up to a few seconds but acceptable.
	now := time.Now()
	ts = time.Date(now.Year(), ts.Month(), ts.Day(),
		ts.Hour(), ts.Minute(), ts.Second(), ts.Nanosecond(),
		time.Local)

	rest := strings.TrimLeft(line[18:], " ")
	// rest = "PID  TID L Tag: Message"
	pidEnd := strings.IndexByte(rest, ' ')
	if pidEnd <= 0 {
		return LogcatLine{}, false
	}
	pid, err := strconv.Atoi(rest[:pidEnd])
	if err != nil {
		return LogcatLine{}, false
	}

	rest = strings.TrimLeft(rest[pidEnd:], " ")
	tidEnd := strings.IndexByte(rest, ' ')
	if tidEnd <= 0 {
		return LogcatLine{}, false
	}
	tid, err := strconv.Atoi(rest[:tidEnd])
	if err != nil {
		return LogcatLine{}, false
	}

	rest = strings.TrimLeft(rest[tidEnd:], " ")
	if len(rest) < 3 {
		return LogcatLine{}, false
	}
	level := rune(rest[0])
	if rest[1] != ' ' {
		return LogcatLine{}, false
	}

	// "Tag: Message" — find the first ": " separator. Tags can
	// contain spaces in pathological cases, so we anchor on the
	// colon-space pair.
	tagAndMsg := rest[2:]
	sepIdx := strings.Index(tagAndMsg, ": ")
	var tag, msg string
	if sepIdx < 0 {
		// "Tag:" with empty message at end-of-line is also valid.
		if strings.HasSuffix(tagAndMsg, ":") {
			tag = strings.TrimSpace(tagAndMsg[:len(tagAndMsg)-1])
			msg = ""
		} else {
			return LogcatLine{}, false
		}
	} else {
		tag = strings.TrimSpace(tagAndMsg[:sepIdx])
		msg = tagAndMsg[sepIdx+2:]
	}

	return LogcatLine{
		Timestamp: ts,
		PID:       pid,
		TID:       tid,
		Level:     level,
		Tag:       tag,
		Message:   msg,
	}, true
}

// runAdb invokes the adb binary with args and returns combined stdout.
// stderr is logged at debug level via slog. Bounded by defaultRunTimeout
// unless ctx already carries a shorter deadline.
//
// Used by Devices() and intended for any future one-shot helpers like
// `adb shell getprop`. Streaming commands (logcat) bypass this and own
// the subprocess directly to keep the cancellation contract clean.
func runAdb(ctx context.Context, args ...string) ([]byte, error) {
	binary, err := exec.LookPath(currentBinary())
	if err != nil {
		return nil, fmt.Errorf("adb: lookup %q: %w", currentBinary(), err)
	}

	// Apply the package-level timeout unless the caller already has one
	// that fires sooner.
	timeoutCtx, cancel := context.WithTimeout(ctx, defaultRunTimeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Prefer the context error if the timeout fired — gives the
		// caller a more actionable message ("deadline exceeded") than
		// "exit status -1 / signal: killed".
		if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
			slog.Debug("adb invocation timed out",
				slog.Any("args", args),
				slog.String("stderr", strings.TrimSpace(stderr.String())),
			)
			return nil, fmt.Errorf("adb %s: %w", strings.Join(args, " "), context.DeadlineExceeded)
		}
		slog.Debug("adb invocation failed",
			slog.Any("args", args),
			slog.Any("error", err),
			slog.String("stderr", strings.TrimSpace(stderr.String())),
		)
		return nil, fmt.Errorf("adb %s: %w (stderr: %s)",
			strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	if stderr.Len() > 0 {
		slog.Debug("adb stderr",
			slog.Any("args", args),
			slog.String("output", strings.TrimSpace(stderr.String())),
		)
	}
	return stdout.Bytes(), nil
}
