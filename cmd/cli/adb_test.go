package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

// fakeADBHub is a small httptest.Server harness for the adb sub-cmd tests.
// It mimics the hub's /adb/* endpoints with hand-rolled bodies so the test
// is a black-box consumer of the CLI's stdout/stderr — no client-package
// import on the wire-shape side.
type fakeADBHub struct {
	*httptest.Server
	mu             sync.Mutex
	lastAuth       string
	lastPath       string
	lastMethod     string
	devicesBody    string
	startRespBody  string
	stopRespBody   string
	devicesStatus  int
	startRespCode  int
	stopRespCode   int
	requireToken   string
}

func startFakeADBHub(t *testing.T, requireToken string) *fakeADBHub {
	t.Helper()
	hub := &fakeADBHub{
		requireToken:  requireToken,
		devicesBody:   `[{"serial":"emulator-5554","state":"device","model":"sdk_gphone64_x86_64"},{"serial":"AB12CD34","state":"unauthorized"}]`,
		startRespBody: `{"status":"started","serial":"emulator-5554","started_at":1700000000000000000}`,
		stopRespBody:  `{"status":"stopped","serial":"emulator-5554"}`,
		devicesStatus: http.StatusOK,
		startRespCode: http.StatusOK,
		stopRespCode:  http.StatusOK,
	}
	hub.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.mu.Lock()
		hub.lastAuth = r.Header.Get("Authorization")
		hub.lastPath = r.URL.Path
		hub.lastMethod = r.Method
		require := hub.requireToken
		hub.mu.Unlock()
		if require != "" && r.Header.Get("Authorization") != "Bearer "+require {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/adb/devices":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(hub.devicesStatus)
			_, _ = w.Write([]byte(hub.devicesBody))
		case "/adb/start":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(hub.startRespCode)
			_, _ = w.Write([]byte(hub.startRespBody))
		case "/adb/stop":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(hub.stopRespCode)
			_, _ = w.Write([]byte(hub.stopRespBody))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(hub.Server.Close)
	return hub
}

// --- tracelab adb devices ---

func TestADBDevices_TableOutput(t *testing.T) {
	t.Parallel()
	hub := startFakeADBHub(t, "tok")
	dir := t.TempDir()
	cfg := writeMinimalConfig(t, dir, "tok")

	stdout, _, err := runCLI(t,
		"adb", "devices",
		"--config", cfg,
		"--url", hub.URL,
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{"SERIAL", "STATE", "MODEL", "emulator-5554", "device", "sdk_gphone64_x86_64", "AB12CD34", "unauthorized"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("missing %q in table output:\n%s", want, stdout)
		}
	}
	// Unauthorized row has no model — placeholder "-" must show.
	if !strings.Contains(stdout, "-") {
		t.Errorf("expected '-' placeholder for missing model:\n%s", stdout)
	}
}

func TestADBDevices_JSONOutput(t *testing.T) {
	t.Parallel()
	hub := startFakeADBHub(t, "tok")
	dir := t.TempDir()
	cfg := writeMinimalConfig(t, dir, "tok")

	stdout, _, err := runCLI(t,
		"adb", "devices",
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
		t.Fatalf("expected 2 devices, got %d", len(got))
	}
	if got[0]["serial"] != "emulator-5554" {
		t.Errorf("first device serial: %+v", got[0])
	}
	if _, ok := got[1]["model"]; ok {
		t.Errorf("second device should not carry model (omitempty): %+v", got[1])
	}
}

func TestADBDevices_EmptyList_RendersJSONArray(t *testing.T) {
	t.Parallel()
	hub := startFakeADBHub(t, "tok")
	hub.devicesBody = `[]`
	dir := t.TempDir()
	cfg := writeMinimalConfig(t, dir, "tok")

	stdout, _, err := runCLI(t,
		"adb", "devices",
		"--config", cfg,
		"--url", hub.URL,
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.TrimSpace(stdout) != "[]" {
		t.Errorf("expected `[]` for empty list, got %q", strings.TrimSpace(stdout))
	}
}

func TestADBDevices_DefaultFormatFromCLISection(t *testing.T) {
	t.Parallel()
	hub := startFakeADBHub(t, "tok")
	dir := t.TempDir()
	path := dir + "/tracelab.toml"
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
		"adb", "devices",
		"--config", path,
		"--url", hub.URL,
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(stdout), "[") {
		t.Errorf("expected JSON output from [cli].default_format=json:\n%s", stdout)
	}
}

func TestADBDevices_Unauthorized_UserError(t *testing.T) {
	t.Parallel()
	hub := startFakeADBHub(t, "real-token")
	dir := t.TempDir()
	cfg := writeMinimalConfig(t, dir, "wrong-token")

	_, _, err := runCLI(t,
		"adb", "devices",
		"--config", cfg,
		"--url", hub.URL,
	)
	if err == nil {
		t.Fatal("expected error for bad token")
	}
	msg, ok := asUserError(err)
	if !ok {
		t.Fatalf("expected userError, got %T: %v", err, err)
	}
	if !strings.Contains(msg, "unauthorized") {
		t.Errorf("error must mention unauthorized: %q", msg)
	}
	// Same leak-guards as sessions / tail — translateClientError reuse.
	for _, leak := range []string{"goroutine", ".go:", "dial tcp", `Get "http`} {
		if strings.Contains(msg, leak) {
			t.Errorf("error leaks %q: %q", leak, msg)
		}
	}
}

func TestADBDevices_InvalidFormat_UserError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := writeMinimalConfig(t, dir, "tok")

	_, _, err := runCLI(t,
		"adb", "devices",
		"--config", cfg,
		"--url", "http://127.0.0.1:1",
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
		t.Errorf("error must name bad format: %q", msg)
	}
}

// --- tracelab adb start ---

func TestADBStart_Happy(t *testing.T) {
	t.Parallel()
	hub := startFakeADBHub(t, "tok")
	dir := t.TempDir()
	cfg := writeMinimalConfig(t, dir, "tok")

	stdout, _, err := runCLI(t,
		"adb", "start", "emulator-5554",
		"--config", cfg,
		"--url", hub.URL,
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "bridge started for emulator-5554") {
		t.Errorf("expected success message, got: %q", stdout)
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if hub.lastPath != "/adb/start" || hub.lastMethod != http.MethodPost {
		t.Errorf("wrong endpoint hit: %s %s", hub.lastMethod, hub.lastPath)
	}
	if hub.lastAuth != "Bearer tok" {
		t.Errorf("Authorization = %q", hub.lastAuth)
	}
}

func TestADBStart_AlreadyRunning_StillSuccess(t *testing.T) {
	t.Parallel()
	hub := startFakeADBHub(t, "tok")
	hub.startRespBody = `{"status":"already_running","serial":"emulator-5554","started_at":1700000000000000000}`
	dir := t.TempDir()
	cfg := writeMinimalConfig(t, dir, "tok")

	stdout, _, err := runCLI(t,
		"adb", "start", "emulator-5554",
		"--config", cfg,
		"--url", hub.URL,
	)
	if err != nil {
		t.Fatalf("idempotent already_running should succeed, got: %v", err)
	}
	// Same success-line output — caller doesn't differentiate started vs.
	// already-running at the CLI surface.
	if !strings.Contains(stdout, "bridge started for emulator-5554") {
		t.Errorf("expected success line, got: %q", stdout)
	}
}

func TestADBStart_MissingSerial(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := writeMinimalConfig(t, dir, "tok")

	_, stderr, err := runCLI(t,
		"adb", "start",
		"--config", cfg,
		"--url", "http://127.0.0.1:1",
	)
	if err == nil {
		t.Fatal("expected error for missing serial")
	}
	// cobra's Args=ExactArgs(1) surfaces the error itself (not a userError).
	// We don't require it to be a userError — only that the CLI exits with
	// a non-nil error and prints something useful.
	if stderr == "" && err.Error() == "" {
		t.Errorf("expected diagnostic output for missing serial")
	}
}

// --- tracelab adb stop ---

func TestADBStop_Happy(t *testing.T) {
	t.Parallel()
	hub := startFakeADBHub(t, "tok")
	dir := t.TempDir()
	cfg := writeMinimalConfig(t, dir, "tok")

	stdout, _, err := runCLI(t,
		"adb", "stop", "emulator-5554",
		"--config", cfg,
		"--url", hub.URL,
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "bridge stopped for emulator-5554") {
		t.Errorf("expected stop message, got: %q", stdout)
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if hub.lastPath != "/adb/stop" {
		t.Errorf("wrong endpoint: %s", hub.lastPath)
	}
}

func TestADBStop_NotRunning_StillSuccess(t *testing.T) {
	t.Parallel()
	hub := startFakeADBHub(t, "tok")
	hub.stopRespBody = `{"status":"not_running","serial":"ghost"}`
	dir := t.TempDir()
	cfg := writeMinimalConfig(t, dir, "tok")

	stdout, _, err := runCLI(t,
		"adb", "stop", "ghost",
		"--config", cfg,
		"--url", hub.URL,
	)
	if err != nil {
		t.Fatalf("idempotent not_running should succeed, got: %v", err)
	}
	if !strings.Contains(stdout, "bridge stopped for ghost") {
		t.Errorf("expected stop line, got: %q", stdout)
	}
}

func TestADBStop_MissingSerial(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := writeMinimalConfig(t, dir, "tok")

	_, _, err := runCLI(t,
		"adb", "stop",
		"--config", cfg,
		"--url", "http://127.0.0.1:1",
	)
	if err == nil {
		t.Fatal("expected error for missing serial")
	}
}

// --- help smoke ---

func TestADBHelp_ListsSubcommands(t *testing.T) {
	t.Parallel()
	stdout, _, err := runCLI(t, "adb", "--help")
	if err != nil {
		t.Fatalf("help: %v", err)
	}
	for _, want := range []string{"devices", "start", "stop"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help missing sub-command %q:\n%s", want, stdout)
		}
	}
}

