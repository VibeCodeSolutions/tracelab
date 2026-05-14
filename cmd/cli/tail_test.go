package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// tailUpgrader for the CLI-side tests. Auth check is delegated to the
// fakeTailHub helper (it inspects the Authorization header before
// upgrading).
var tailTestUpgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true },
}

// fakeTailEvent mirrors internal/client.Event on the wire — we hand-roll
// the struct here rather than import the client package's type so the
// test stays a black-box consumer of the CLI's stdout.
type fakeTailEvent struct {
	SessionID string                 `json:"session_id"`
	TS        int64                  `json:"ts"`
	Source    string                 `json:"source"`
	Level     string                 `json:"level"`
	Msg       string                 `json:"msg"`
	Meta      map[string]interface{} `json:"meta,omitempty"`
}

// fakeTailHub starts an httptest.Server that handles /tail with bearer
// auth and streams a caller-supplied list of events, then closes
// cleanly. The path filter (?session=…) is recorded for assertions but
// not enforced against the events — tests that need filter behaviour
// can inspect lastQuery directly.
type fakeTailHub struct {
	*httptest.Server
	lastQuery string
	lastAuth  string
}

func startFakeTailHub(t *testing.T, wantToken string, events []fakeTailEvent) *fakeTailHub {
	t.Helper()
	hub := &fakeTailHub{}
	hub.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.lastQuery = r.URL.RawQuery
		hub.lastAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/tail" {
			http.NotFound(w, r)
			return
		}
		if wantToken != "" && r.Header.Get("Authorization") != "Bearer "+wantToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := tailTestUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		for _, e := range events {
			if err := conn.WriteJSON(e); err != nil {
				return
			}
		}
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"),
			time.Now().Add(time.Second),
		)
	}))
	t.Cleanup(hub.Server.Close)
	return hub
}

// writeTailConfig writes a tracelab.toml that pins [cli].color = "never"
// so all plain-format assertions are colour-stripped, deterministic
// substring checks. Tests that exercise the colour-on path override
// this explicitly.
func writeTailConfig(t *testing.T, dir, token, color string) string {
	t.Helper()
	path := filepath.Join(dir, "tracelab.toml")
	body := `
[server]
port = 8765
bind = "127.0.0.1"

[auth]
token = "` + token + `"

[cli]
color = "` + color + `"
tail_buffer = 64
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestTail_HelpMentionsFlags(t *testing.T) {
	t.Parallel()
	stdout, _, err := runCLI(t, "tail", "--help")
	if err != nil {
		t.Fatalf("help: %v", err)
	}
	for _, want := range []string{"--session", "--format"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help missing %s:\n%s", want, stdout)
		}
	}
}

func TestTail_RequiresSession(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := writeTailConfig(t, dir, "tok", "never")
	_, _, err := runCLI(t,
		"tail",
		"--config", cfg,
		"--url", "http://127.0.0.1:1",
	)
	if err == nil {
		t.Fatal("expected error when --session missing")
	}
	msg, ok := asUserError(err)
	if !ok {
		t.Fatalf("expected userError, got %T: %v", err, err)
	}
	if !strings.Contains(msg, "--session is required") {
		t.Errorf("wrong message: %q", msg)
	}
}

func TestTail_PlainOutput_NoColour(t *testing.T) {
	t.Parallel()
	events := []fakeTailEvent{
		{SessionID: "s1", TS: 1700000000_000000000, Source: "logcat", Level: "INFO", Msg: "boot"},
		{SessionID: "s1", TS: 1700000001_000000000, Source: "logcat", Level: "WARN", Msg: "slow"},
		{SessionID: "s1", TS: 1700000002_000000000, Source: "logcat", Level: "ERROR", Msg: "crash"},
	}
	hub := startFakeTailHub(t, "tok", events)
	dir := t.TempDir()
	cfg := writeTailConfig(t, dir, "tok", "never")
	stdout, _, err := runCLI(t,
		"tail",
		"--config", cfg,
		"--url", hub.URL,
		"--session", "s1",
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{"INFO", "WARN", "ERROR", "[logcat]", "boot", "slow", "crash"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output missing %q:\n%s", want, stdout)
		}
	}
	// colour=never must keep ANSI escapes out of the stream
	if strings.Contains(stdout, "\x1b[") {
		t.Errorf("colour=never leaked ANSI escapes: %q", stdout)
	}
}

func TestTail_PlainOutput_ColourAlways(t *testing.T) {
	t.Parallel()
	events := []fakeTailEvent{
		{SessionID: "s1", TS: 1700000000_000000000, Source: "app", Level: "ERROR", Msg: "boom"},
		{SessionID: "s1", TS: 1700000001_000000000, Source: "app", Level: "WARN", Msg: "slow"},
		{SessionID: "s1", TS: 1700000002_000000000, Source: "app", Level: "DEBUG", Msg: "frame"},
	}
	hub := startFakeTailHub(t, "tok", events)
	dir := t.TempDir()
	cfg := writeTailConfig(t, dir, "tok", "always")
	stdout, _, err := runCLI(t,
		"tail",
		"--config", cfg,
		"--url", hub.URL,
		"--session", "s1",
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// Colour=always must emit ANSI escapes around the level tokens.
	for _, want := range []string{"\x1b[31mERROR\x1b[0m", "\x1b[33mWARN\x1b[0m", "\x1b[2mDEBUG\x1b[0m"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("missing coloured level token %q in:\n%q", want, stdout)
		}
	}
}

func TestTail_JSONOutput_NDJSON(t *testing.T) {
	t.Parallel()
	events := []fakeTailEvent{
		{SessionID: "s1", TS: 1700000000_000000000, Source: "logcat", Level: "INFO", Msg: "one"},
		{SessionID: "s1", TS: 1700000001_000000000, Source: "logcat", Level: "INFO", Msg: "two"},
	}
	hub := startFakeTailHub(t, "tok", events)
	dir := t.TempDir()
	cfg := writeTailConfig(t, dir, "tok", "never")
	stdout, _, err := runCLI(t,
		"tail",
		"--config", cfg,
		"--url", hub.URL,
		"--session", "s1",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 NDJSON lines, got %d:\n%s", len(lines), stdout)
	}
	for i, line := range lines {
		var got fakeTailEvent
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Errorf("line %d not valid JSON: %v\n%s", i, err, line)
			continue
		}
		if got.Msg != events[i].Msg {
			t.Errorf("line %d Msg = %q, want %q", i, got.Msg, events[i].Msg)
		}
	}
}

func TestTail_InvalidFormat_UserError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := writeTailConfig(t, dir, "tok", "never")
	_, _, err := runCLI(t,
		"tail",
		"--config", cfg,
		"--url", "http://127.0.0.1:1",
		"--session", "s1",
		"--format", "xml",
	)
	if err == nil {
		t.Fatal("expected error for invalid --format")
	}
	msg, ok := asUserError(err)
	if !ok {
		t.Fatalf("expected userError, got %T: %v", err, err)
	}
	if !strings.Contains(msg, "xml") {
		t.Errorf("error must name bad format, got %q", msg)
	}
}

func TestTail_Unauthorized_UserError(t *testing.T) {
	t.Parallel()
	hub := startFakeTailHub(t, "real-token", nil)
	dir := t.TempDir()
	cfg := writeTailConfig(t, dir, "wrong-token", "never")
	_, _, err := runCLI(t,
		"tail",
		"--config", cfg,
		"--url", hub.URL,
		"--session", "s1",
	)
	if err == nil {
		t.Fatal("expected error for bad token")
	}
	msg, ok := asUserError(err)
	if !ok {
		t.Fatalf("expected userError (no stack trace), got %T: %v", err, err)
	}
	if !strings.Contains(msg, "unauthorized") {
		t.Errorf("error message must mention unauthorized, got %q", msg)
	}
	// Same leak-guards as TestSessions_Unauthorized_UserError — the tail
	// path must reuse translateClientError, not duplicate it.
	for _, leak := range []string{"goroutine", ".go:", "dial tcp", `Get "http`} {
		if strings.Contains(msg, leak) {
			t.Errorf("error message leaks %q: %q", leak, msg)
		}
	}
}

func TestTail_ConnectionRefused_UserError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := writeTailConfig(t, dir, "tok", "never")
	// Nothing listening on port 1 → handshake fails → translateClientError
	// must scrub the wrapped chain down to a clean message.
	_, _, err := runCLI(t,
		"tail",
		"--config", cfg,
		"--url", "http://127.0.0.1:1",
		"--session", "s1",
	)
	if err == nil {
		t.Fatal("expected error on dead URL")
	}
	msg, ok := asUserError(err)
	if !ok {
		t.Fatalf("expected userError, got %T: %v", err, err)
	}
	if !strings.Contains(msg, "cannot reach hub at") {
		t.Errorf("missing user-facing prefix: %q", msg)
	}
	for _, leak := range []string{"dial tcp", `Get "http`, "goroutine", ".go:"} {
		if strings.Contains(msg, leak) {
			t.Errorf("error message leaks %q: %q", leak, msg)
		}
	}
}

func TestTail_QueryPropagation(t *testing.T) {
	t.Parallel()
	hub := startFakeTailHub(t, "tok", nil)
	dir := t.TempDir()
	cfg := writeTailConfig(t, dir, "tok", "never")
	_, _, err := runCLI(t,
		"tail",
		"--config", cfg,
		"--url", hub.URL,
		"--session", "sess-42",
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if hub.lastQuery != "session=sess-42" {
		t.Errorf("query = %q, want session=sess-42", hub.lastQuery)
	}
	if hub.lastAuth != "Bearer tok" {
		t.Errorf("auth = %q, want Bearer tok", hub.lastAuth)
	}
}

// TestTail_ContextCancel_NoLeak exercises the SIGINT-equivalent path:
// signal handling is heavy to test directly (and platform-specific), so
// we drive the same shutdown path via root.ExecuteContext with a
// cancellable context — runTail observes the cancel via cmd.Context(),
// NotifyContext propagates it, the watcher in client.Tail sends the
// close frame, and the printer goroutine drains and returns.
//
// The test asserts:
//   - runTail returns nil within a short budget after cancel
//   - some events made it to stdout before cancel (proves the printer
//     actually drained, not that we just bailed before any IO)
func TestTail_ContextCancel_NoLeak(t *testing.T) {
	t.Parallel()
	// Server upgrades, writes one event, then blocks until the client
	// closes the conn.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "nope", http.StatusUnauthorized)
			return
		}
		conn, err := tailTestUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteJSON(fakeTailEvent{
			SessionID: "s1", TS: 1700000000_000000000, Source: "logcat", Level: "INFO", Msg: "stay",
		})
		// Block until the client sends its close frame.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	cfg := writeTailConfig(t, dir, "tok", "never")

	root := newRootCmd()
	var stdoutMu sync.Mutex
	stdout := &lockedBuffer{mu: &stdoutMu}
	root.SetOut(stdout)
	root.SetErr(&lockedBuffer{mu: &stdoutMu})
	root.SetArgs([]string{
		"tail",
		"--config", cfg,
		"--url", srv.URL,
		"--session", "s1",
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- root.ExecuteContext(ctx) }()

	// Wait for at least one event to land (proves the read loop is live)
	// before cancelling — bounded poll, no fixed sleep.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		stdoutMu.Lock()
		hasOutput := strings.Contains(stdout.String(), "stay")
		stdoutMu.Unlock()
		if hasOutput {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("tail returned %v, want nil on ctx-cancel", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("tail did not return within 3 s of ctx-cancel — leaked goroutine?")
	}

	stdoutMu.Lock()
	got := stdout.String()
	stdoutMu.Unlock()
	if !strings.Contains(got, "stay") {
		t.Errorf("expected at least one event before cancel in stdout, got:\n%s", got)
	}
}

// lockedBuffer is a tiny mutex-guarded bytes-buffer-alike — needed
// because cobra writes to stdout from the printer goroutine while the
// main test goroutine peeks at it for the "wait for first event" probe.
type lockedBuffer struct {
	mu  *sync.Mutex
	buf []byte
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	b.buf = append(b.buf, p...)
	b.mu.Unlock()
	return len(p), nil
}

func (b *lockedBuffer) String() string { return string(b.buf) }
