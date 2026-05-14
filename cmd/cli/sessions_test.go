package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeHub starts an httptest.Server that mimics the hub's /sessions
// endpoint. The recorded request is returned via the closure-captured
// slice so tests can assert on Authorization headers, query strings, etc.
type fakeHub struct {
	*httptest.Server
	// lastQuery is the raw URL query string of the most recent request
	// (e.g. "limit=5"). Useful for asserting that --limit propagates.
	lastQuery string
	// lastAuth is the most recent Authorization header value.
	lastAuth string
}

// startFakeHub returns a hub that responds with two sessions on a 200,
// or 401 when the bearer doesn't match wantToken. wantToken="" allows
// any token (smoke mode).
func startFakeHub(t *testing.T, wantToken string) *fakeHub {
	t.Helper()
	hub := &fakeHub{}
	hub.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.lastQuery = r.URL.RawQuery
		hub.lastAuth = r.Header.Get("Authorization")
		if wantToken != "" && r.Header.Get("Authorization") != "Bearer "+wantToken {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sessions":[
			{"id":"sess-a","label":"smoke","started_at":1700000000000000000,"ended_at":1700000060000000000},
			{"id":"sess-b","label":"open","started_at":1700001000000000000}
		]}`))
	}))
	t.Cleanup(hub.Server.Close)
	return hub
}

// writeMinimalConfig writes a tracelab.toml with the bare minimum the
// CLI needs (server + auth blocks) and returns its path. Used by tests
// that drive the discovery layer.
func writeMinimalConfig(t *testing.T, dir, token string) string {
	t.Helper()
	path := filepath.Join(dir, "tracelab.toml")
	if err := os.WriteFile(path, []byte(`
[server]
port = 8765
bind = "127.0.0.1"

[auth]
token = "`+token+`"
`), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// runCLI builds the root command, pipes its stdout/stderr to buffers,
// applies argv, and returns (stdout, err).
//
// HOME and the locale env are not touched here — callers that need a
// hermetic discovery layer set TRACELAB_CONFIG (or pass --config)
// explicitly to bypass the cwd/XDG/$HOME layers. Where a test really
// needs to exercise those layers we use t.Setenv to neutralise
// TRACELAB_* variables before invocation.
func runCLI(t *testing.T, argv ...string) (string, string, error) {
	t.Helper()
	root := newRootCmd()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(argv)
	// Bind a fresh context so each test is cancellable.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	err := root.ExecuteContext(ctx)
	return stdout.String(), stderr.String(), err
}

func TestSessions_TableOutput_FromConfigFile(t *testing.T) {
	t.Parallel()
	hub := startFakeHub(t, "tok")
	dir := t.TempDir()
	cfg := writeMinimalConfig(t, dir, "tok")
	// Force the config path so the test never depends on $HOME / cwd.
	stdout, _, err := runCLI(t,
		"sessions",
		"--config", cfg,
		"--url", hub.URL, // override the loopback URL from the config
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "ID") || !strings.Contains(stdout, "LABEL") {
		t.Errorf("missing header in table output:\n%s", stdout)
	}
	if !strings.Contains(stdout, "sess-a") || !strings.Contains(stdout, "sess-b") {
		t.Errorf("missing session rows:\n%s", stdout)
	}
	if !strings.Contains(stdout, "running") {
		t.Errorf("expected 'running' marker for open session:\n%s", stdout)
	}
	if hub.lastAuth != "Bearer tok" {
		t.Errorf("Authorization = %q, want Bearer tok", hub.lastAuth)
	}
}

func TestSessions_JSONOutput(t *testing.T) {
	t.Parallel()
	hub := startFakeHub(t, "tok")
	dir := t.TempDir()
	cfg := writeMinimalConfig(t, dir, "tok")
	stdout, _, err := runCLI(t,
		"sessions",
		"--config", cfg,
		"--url", hub.URL,
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, stdout)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(got))
	}
	// First session has an ended_at, second has not.
	if _, ok := got[0]["ended_at"]; !ok {
		t.Errorf("first session must carry ended_at: %+v", got[0])
	}
	if _, ok := got[1]["ended_at"]; ok {
		t.Errorf("second session must NOT carry ended_at (omitempty): %+v", got[1])
	}
}

func TestSessions_LimitFlagPropagates(t *testing.T) {
	t.Parallel()
	hub := startFakeHub(t, "tok")
	dir := t.TempDir()
	cfg := writeMinimalConfig(t, dir, "tok")
	_, _, err := runCLI(t,
		"sessions",
		"--config", cfg,
		"--url", hub.URL,
		"--limit", "5",
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if hub.lastQuery != "limit=5" {
		t.Errorf("expected limit=5 in query, got %q", hub.lastQuery)
	}
}

func TestSessions_DefaultLimit_Is20(t *testing.T) {
	t.Parallel()
	hub := startFakeHub(t, "tok")
	dir := t.TempDir()
	cfg := writeMinimalConfig(t, dir, "tok")
	_, _, err := runCLI(t,
		"sessions",
		"--config", cfg,
		"--url", hub.URL,
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if hub.lastQuery != "limit=20" {
		t.Errorf("expected limit=20 (default), got %q", hub.lastQuery)
	}
}

func TestSessions_DefaultFormat_FromCLISection(t *testing.T) {
	t.Parallel()
	// CLI section requests JSON; --format flag is not passed.
	hub := startFakeHub(t, "tok")
	dir := t.TempDir()
	path := filepath.Join(dir, "tracelab.toml")
	if err := os.WriteFile(path, []byte(`
[server]
port = 8765
bind = "127.0.0.1"
[auth]
token = "tok"
[cli]
default_format = "json"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := runCLI(t,
		"sessions",
		"--config", path,
		"--url", hub.URL,
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// JSON output starts with '[' — table starts with 'ID'.
	if !strings.HasPrefix(strings.TrimSpace(stdout), "[") {
		t.Errorf("expected JSON output from [cli].default_format=json, got:\n%s", stdout)
	}
}

func TestSessions_FlagBeatsCLIDefault(t *testing.T) {
	t.Parallel()
	hub := startFakeHub(t, "tok")
	dir := t.TempDir()
	path := filepath.Join(dir, "tracelab.toml")
	if err := os.WriteFile(path, []byte(`
[server]
port = 8765
[auth]
token = "tok"
[cli]
default_format = "json"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := runCLI(t,
		"sessions",
		"--config", path,
		"--url", hub.URL,
		"--format", "table", // explicit override
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "ID") || !strings.Contains(stdout, "LABEL") {
		t.Errorf("--format=table must beat [cli].default_format=json:\n%s", stdout)
	}
}

func TestSessions_InvalidFormat_UserError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := writeMinimalConfig(t, dir, "tok")
	_, _, err := runCLI(t,
		"sessions",
		"--config", cfg,
		"--url", "http://nope.invalid:1",
		"--format", "xml",
	)
	if err == nil {
		t.Fatal("expected error for invalid --format")
	}
	msg, ok := asUserError(err)
	if !ok {
		t.Errorf("expected userError, got %T: %v", err, err)
	}
	if !strings.Contains(msg, "xml") {
		t.Errorf("error must name the bad format, got %q", msg)
	}
}

func TestSessions_Unauthorized_UserError(t *testing.T) {
	t.Parallel()
	hub := startFakeHub(t, "real-token")
	dir := t.TempDir()
	cfg := writeMinimalConfig(t, dir, "wrong-token")
	_, _, err := runCLI(t,
		"sessions",
		"--config", cfg,
		"--url", hub.URL,
	)
	if err == nil {
		t.Fatal("expected error for bad token")
	}
	msg, ok := asUserError(err)
	if !ok {
		t.Errorf("expected userError (no stack trace), got %T: %v", err, err)
	}
	if !strings.Contains(msg, "unauthorized") {
		t.Errorf("error message must mention unauthorized, got %q", msg)
	}
	if strings.Contains(msg, "goroutine") || strings.Contains(msg, ".go:") {
		t.Errorf("error message leaks Go internals: %q", msg)
	}
}

func TestSessions_ServerError_UserError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	cfg := writeMinimalConfig(t, dir, "tok")
	_, _, err := runCLI(t,
		"sessions",
		"--config", cfg,
		"--url", srv.URL,
	)
	if err == nil {
		t.Fatal("expected error for 500")
	}
	msg, ok := asUserError(err)
	if !ok {
		t.Errorf("expected userError, got %T: %v", err, err)
	}
	if !strings.Contains(msg, "500") && !strings.Contains(msg, "hub error") {
		t.Errorf("message should reference status / hub error, got %q", msg)
	}
}

// ---- Override precedence: all three layers ----

func TestSessions_Precedence_FlagBeatsEnv(t *testing.T) {
	// No t.Parallel: t.Setenv is incompatible with parallel tests.
	hub := startFakeHub(t, "from-flag")
	// Env says "from-env", flag says "from-flag" — flag wins.
	t.Setenv("TRACELAB_TOKEN", "from-env")
	dir := t.TempDir()
	cfg := writeMinimalConfig(t, dir, "from-config")
	_, _, err := runCLI(t,
		"sessions",
		"--config", cfg,
		"--url", hub.URL,
		"--token", "from-flag",
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if hub.lastAuth != "Bearer from-flag" {
		t.Errorf("flag token must win: got %q", hub.lastAuth)
	}
}

func TestSessions_Precedence_EnvBeatsConfig(t *testing.T) {
	// No t.Parallel: t.Setenv is incompatible with parallel tests.
	hub := startFakeHub(t, "from-env")
	t.Setenv("TRACELAB_TOKEN", "from-env")
	dir := t.TempDir()
	cfg := writeMinimalConfig(t, dir, "from-config")
	_, _, err := runCLI(t,
		"sessions",
		"--config", cfg,
		"--url", hub.URL,
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if hub.lastAuth != "Bearer from-env" {
		t.Errorf("env token must beat config: got %q", hub.lastAuth)
	}
}

func TestSessions_Precedence_ConfigOnly(t *testing.T) {
	// No t.Parallel: t.Setenv is incompatible with parallel tests.
	// Neither TRACELAB_TOKEN nor --token set; config is the only source.
	hub := startFakeHub(t, "from-config")
	t.Setenv("TRACELAB_TOKEN", "")
	t.Setenv("TRACELAB_URL", "")
	dir := t.TempDir()
	cfg := writeMinimalConfig(t, dir, "from-config")
	_, _, err := runCLI(t,
		"sessions",
		"--config", cfg,
		"--url", hub.URL,
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if hub.lastAuth != "Bearer from-config" {
		t.Errorf("config token must apply when no override: got %q", hub.lastAuth)
	}
}

func TestSessions_URLPrecedence_EnvBeatsConfig(t *testing.T) {
	// No t.Parallel: t.Setenv is incompatible with parallel tests.
	hub := startFakeHub(t, "tok")
	t.Setenv("TRACELAB_URL", hub.URL)
	dir := t.TempDir()
	cfg := writeMinimalConfig(t, dir, "tok")
	// Pass --config but NOT --url. URL must come from env.
	_, _, err := runCLI(t, "sessions", "--config", cfg)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if hub.lastAuth != "Bearer tok" {
		t.Errorf("hub never reached: lastAuth=%q", hub.lastAuth)
	}
}

// ---- asUserError + error-translation helpers ----

func TestAsUserError_RecognisesWrappedSentinel(t *testing.T) {
	t.Parallel()
	wrapped := userError("plain message")
	if msg, ok := asUserError(wrapped); !ok || msg != "plain message" {
		t.Errorf("asUserError(direct) = (%q, %v)", msg, ok)
	}
	if _, ok := asUserError(errors.New("regular")); ok {
		t.Errorf("asUserError on regular error must be false")
	}
}

// Smoke: the help text mentions both --limit and --format.
func TestSessions_HelpMentionsFlags(t *testing.T) {
	t.Parallel()
	stdout, _, err := runCLI(t, "sessions", "--help")
	if err != nil {
		t.Fatalf("help: %v", err)
	}
	if !strings.Contains(stdout, "--limit") {
		t.Errorf("help missing --limit:\n%s", stdout)
	}
	if !strings.Contains(stdout, "--format") {
		t.Errorf("help missing --format:\n%s", stdout)
	}
}
