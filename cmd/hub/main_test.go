package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/VibeCodeSolutions/tracelab/internal/store"
)

// TestHub_End2End_AdbBridge spawns the real tracelab-hub binary with a
// fake `adb` shim on PATH, points the toml at a tempdir, and verifies:
//
//  1. bridge starts (slog "adb bridge starting")
//  2. lines from fake adb arrive in events table with source="adb"
//  3. SIGTERM produces the documented stop ordering in slog:
//     "adb bridge stopped" → "websocket hub closed" → "http server stopped".
//
// Skipped on Windows (the fake adb is a POSIX shell script). This is a
// medium-weight integration test: it builds the binary, runs it, and
// reads the stderr stream — heavier than unit tests but the only way
// to verify the slog stop-ordering of the real main.go.
func TestHub_End2End_AdbBridge(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake adb harness is POSIX-only; covered by ballard/barclay seam")
	}
	if os.Getenv("CI_SKIP_INTEGRATION") == "1" {
		t.Skip("integration test disabled via CI_SKIP_INTEGRATION")
	}

	root := repoRoot(t)
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "tracelab-hub")
	build := exec.Command(goBinary(t), "build", "-o", binPath, "./cmd/hub")
	build.Dir = root
	build.Env = append(os.Environ(), "GOTOOLCHAIN=auto")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build cmd/hub: %v\n%s", err, out)
	}

	// Fake adb that emits five threadtime lines and exits — bridge will
	// then enter backoff (we'll SIGTERM before a reconnect noisily fires).
	fakeAdbDir := t.TempDir()
	fakeAdb := filepath.Join(fakeAdbDir, "adb")
	now := time.Now().Format("01-02 15:04:05.000")
	logBody := strings.Join([]string{
		"#!/bin/sh",
		"# fake-adb: respond to `devices` and stream a few logcat lines",
		`if [ "$1" = "devices" ]; then`,
		`  echo "List of devices attached"`,
		`  echo "emulator-fake          device"`,
		`  exit 0`,
		`fi`,
		`# any logcat invocation: emit five lines and exit cleanly`,
		fmt.Sprintf(`echo "%s  111  222 I FakeTag: hello-1"`, now),
		fmt.Sprintf(`echo "%s  111  222 W FakeTag: warning-2"`, now),
		fmt.Sprintf(`echo "%s  111  222 E FakeTag: oops-3"`, now),
		fmt.Sprintf(`echo "%s  111  222 D FakeTag: dbg-4"`, now),
		fmt.Sprintf(`echo "%s  111  222 V FakeTag: verb-5"`, now),
		`exit 0`,
	}, "\n") + "\n"
	if err := os.WriteFile(fakeAdb, []byte(logBody), 0o755); err != nil {
		t.Fatalf("write fake adb: %v", err)
	}

	// Pick a free port so parallel tests / leftover daemons can't clash.
	port, err := freeTCPPort()
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	dataDir := t.TempDir()
	cfgPath := filepath.Join(t.TempDir(), "tracelab.toml")
	cfg := strings.Join([]string{
		"[server]",
		fmt.Sprintf("port = %d", port),
		`bind = "127.0.0.1"`,
		"",
		"[storage]",
		fmt.Sprintf(`datastore_path = %q`, dataDir),
		"",
		"[auth]",
		`token = "integration-token-1234"`,
		"",
		"[adb]",
		`enabled = true`,
		`device_serial = "emulator-fake"`,
		`tag_filter = ""`,
		"",
	}, "\n")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Run the daemon.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binPath, "-config", cfgPath)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeAdbDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stderr safeBuf
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	// New process group so SIGTERM hits children too.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start hub: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	})

	// Wait until /healthz responds.
	addr := "127.0.0.1:" + strconv.Itoa(port)
	if err := waitForHealthz("http://"+addr+"/healthz", 5*time.Second); err != nil {
		t.Fatalf("hub did not become healthy: %v\n--- log ---\n%s", err, stderr.String())
	}

	// Wait until the events table has 5 adb rows.
	dbPath := filepath.Join(dataDir, "tracelab.db")
	deadline := time.Now().Add(5 * time.Second)
	var got []store.Event
	for time.Now().Before(deadline) {
		got = readAdbEvents(t, dbPath)
		if len(got) >= 5 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(got) < 5 {
		t.Fatalf("only %d adb events in db, want ≥5\n--- log ---\n%s", len(got), stderr.String())
	}
	wantLevels := []string{"info", "warn", "error", "debug", "debug"}
	for i, e := range got[:5] {
		if e.Source != "adb" {
			t.Errorf("event[%d].source=%q want adb", i, e.Source)
		}
		if e.Level != wantLevels[i] {
			t.Errorf("event[%d].level=%q want %q", i, e.Level, wantLevels[i])
		}
	}

	// Send SIGTERM and observe stop-ordering in slog.
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err != nil {
		t.Fatalf("SIGTERM: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		// Exit on signal is expected; ignore *exec.ExitError, surface the rest.
		var ee *exec.ExitError
		if !asExitError(err, &ee) {
			t.Fatalf("hub wait: %v\n--- log ---\n%s", err, stderr.String())
		}
	}

	logs := stderr.String()
	bridgeIdx := strings.Index(logs, `"adb bridge stopped"`)
	hubIdx := strings.Index(logs, `"websocket hub closed"`)
	httpIdx := strings.Index(logs, `"http server stopped"`)
	if bridgeIdx < 0 || hubIdx < 0 || httpIdx < 0 {
		t.Fatalf("missing stop markers in log:\n%s", logs)
	}
	if !(bridgeIdx < hubIdx && hubIdx < httpIdx) {
		t.Errorf("stop ordering wrong: bridge=%d hub=%d http=%d (want ascending)\n%s",
			bridgeIdx, hubIdx, httpIdx, logs)
	}
}

// safeBuf is a goroutine-safe bytes.Buffer wrapper for capturing
// concurrent stdout/stderr writes from os/exec.
type safeBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *safeBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}
func (s *safeBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// readAdbEvents opens the same SQLite db the hub wrote to and returns
// every event with source="adb", oldest first.
func readAdbEvents(t *testing.T, dbPath string) []store.Event {
	t.Helper()
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	rows, err := st.DB().QueryContext(t.Context(), `
		SELECT id, session_id, ts, source, level, msg, meta
		FROM events
		WHERE source = 'adb'
		ORDER BY id ASC
	`)
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	defer rows.Close()
	var out []store.Event
	for rows.Next() {
		var e store.Event
		var tsNano int64
		var meta sqlNullStr
		if err := rows.Scan(&e.ID, &e.SessionID, &tsNano, &e.Source, &e.Level, &e.Msg, &meta); err != nil {
			t.Fatalf("scan: %v", err)
		}
		e.TS = time.Unix(0, tsNano)
		if meta.valid {
			e.Meta = json.RawMessage(meta.s)
		}
		out = append(out, e)
	}
	return out
}

type sqlNullStr struct {
	s     string
	valid bool
}

func (n *sqlNullStr) Scan(v any) error {
	if v == nil {
		n.valid = false
		return nil
	}
	switch x := v.(type) {
	case string:
		n.s = x
		n.valid = true
	case []byte:
		n.s = string(x)
		n.valid = true
	}
	return nil
}

func waitForHealthz(url string, max time.Duration) error {
	deadline := time.Now().Add(max)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("healthz did not respond within %v", max)
}

func freeTCPPort() (int, error) {
	// Use port 0 trick via net.Listen.
	ln, err := newListener()
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.port, nil
}

// repoRoot walks up from the test file looking for go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found from %s upward", dir)
		}
		dir = parent
	}
}

// goBinary returns the path to the `go` executable used to build the
// hub. Honours $GOROOT/bin/go first, then $PATH.
func goBinary(t *testing.T) string {
	t.Helper()
	if root := os.Getenv("GOROOT"); root != "" {
		candidate := filepath.Join(root, "bin", "go")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	t.Skip("no `go` binary found in GOROOT or PATH; skipping integration test")
	return ""
}

// asExitError is a tiny errors.As helper that doesn't pull errors into
// the import block here.
func asExitError(err error, target **exec.ExitError) bool {
	for err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			*target = e
			return true
		}
		// no errors.Unwrap here — *exec.ExitError shows up directly
		// from os/exec, no wrapping involved.
		return false
	}
	return false
}
