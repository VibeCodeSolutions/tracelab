package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/VibeCodeSolutions/tracelab/internal/client"
)

// --- shared test helpers ---

// callADBDevices drives the adb_devices handler closure with a constructed
// CallToolRequest. Mirrors callSessionsList / callTailSince in the sibling
// test files — we test the handler directly so failure messages name the
// tool semantics, not the transport.
func callADBDevices(t *testing.T, c *client.Client, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	tool := newADBDevicesTool(c)
	req := mcp.CallToolRequest{}
	req.Params.Name = adbDevicesToolName
	req.Params.Arguments = args
	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned unexpected Go error: %v", err)
	}
	if res == nil {
		t.Fatal("handler returned nil result")
	}
	return res
}

func callADBStart(t *testing.T, c *client.Client, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	tool := newADBStartTool(c)
	req := mcp.CallToolRequest{}
	req.Params.Name = adbStartToolName
	req.Params.Arguments = args
	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned unexpected Go error: %v", err)
	}
	if res == nil {
		t.Fatal("handler returned nil result")
	}
	return res
}

func callADBStop(t *testing.T, c *client.Client, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	tool := newADBStopTool(c)
	req := mcp.CallToolRequest{}
	req.Params.Name = adbStopToolName
	req.Params.Arguments = args
	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned unexpected Go error: %v", err)
	}
	if res == nil {
		t.Fatal("handler returned nil result")
	}
	return res
}

// decodeADBDevicesBody extracts the JSON-encoded {"devices":[...]} payload.
func decodeADBDevicesBody(t *testing.T, res *mcp.CallToolResult) adbDevicesResult {
	t.Helper()
	if res.IsError {
		t.Fatalf("expected success, got IsError=true: %v", res.Content)
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] type = %T, want mcp.TextContent", res.Content[0])
	}
	var out adbDevicesResult
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("decode body %q: %v", tc.Text, err)
	}
	return out
}

func decodeADBStartBody(t *testing.T, res *mcp.CallToolResult) adbStartResult {
	t.Helper()
	if res.IsError {
		t.Fatalf("expected success, got IsError=true: %v", res.Content)
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] type = %T, want mcp.TextContent", res.Content[0])
	}
	var out adbStartResult
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("decode body %q: %v", tc.Text, err)
	}
	return out
}

func decodeADBStopBody(t *testing.T, res *mcp.CallToolResult) adbStopResult {
	t.Helper()
	if res.IsError {
		t.Fatalf("expected success, got IsError=true: %v", res.Content)
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] type = %T, want mcp.TextContent", res.Content[0])
	}
	var out adbStopResult
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("decode body %q: %v", tc.Text, err)
	}
	return out
}

// ===========================================================================
// adb_devices
// ===========================================================================

// TestADBDevicesToolRegistered confirms the real tool is registered, and
// crucially that the S1 adb_stub placeholder retired in S5.
func TestADBDevicesToolRegistered(t *testing.T) {
	t.Parallel()
	c := newTestHubServer(t, http.NotFoundHandler())
	s := buildServer(c)
	tools := s.ListTools()
	if _, ok := tools["adb_devices"]; !ok {
		t.Errorf("adb_devices missing from registry; got %v", toolNames(tools))
	}
	if _, ok := tools["adb_stub"]; ok {
		t.Errorf("adb_stub should have retired in S5 but is still registered")
	}
}

// TestADBDevicesDescriptionPresent guards a non-empty description.
func TestADBDevicesDescriptionPresent(t *testing.T) {
	t.Parallel()
	c := newTestHubServer(t, http.NotFoundHandler())
	s := buildServer(c)
	st := s.ListTools()["adb_devices"]
	if st == nil {
		t.Fatal("adb_devices not registered")
	}
	desc := strings.TrimSpace(st.Tool.Description)
	if desc == "" {
		t.Fatal("adb_devices has empty Description")
	}
	if !strings.Contains(strings.ToLower(desc), "adb") {
		t.Errorf("description %q does not mention 'adb'", desc)
	}
}

// TestADBDevicesInputSchemaAccepts exercises the no-args shape (ADR-007:
// adb_devices takes no parameters).
func TestADBDevicesInputSchemaAccepts(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})
	c := newTestHubServer(t, h)
	res := callADBDevices(t, c, map[string]any{})
	if res.IsError {
		t.Errorf("unexpected error result: %s", errorText(t, res))
	}
}

// TestADBDevicesHandlerCallsHub exercises the happy path: bearer-attached
// GET /adb/devices, two devices returned, envelope passes through.
func TestADBDevicesHandlerCallsHub(t *testing.T) {
	t.Parallel()
	var gotAuth, gotPath, gotMethod string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"serial":"emulator-5554","state":"device","model":"sdk_gphone64_x86_64"},
			{"serial":"AB12CD34","state":"unauthorized"}
		]`))
	})
	c := newTestHubServer(t, h)

	res := callADBDevices(t, c, map[string]any{})
	body := decodeADBDevicesBody(t, res)

	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want Bearer test-token", gotAuth)
	}
	if gotMethod != http.MethodGet || gotPath != "/adb/devices" {
		t.Errorf("hub req = %s %s, want GET /adb/devices", gotMethod, gotPath)
	}
	if len(body.Devices) != 2 {
		t.Fatalf("len = %d, want 2", len(body.Devices))
	}
	if body.Devices[0].Serial != "emulator-5554" || body.Devices[0].Model != "sdk_gphone64_x86_64" {
		t.Errorf("device[0] unexpected: %+v", body.Devices[0])
	}
	if body.Devices[1].Model != "" {
		t.Errorf("device[1].Model should be empty (omitempty), got %q", body.Devices[1].Model)
	}
}

// TestADBDevicesAuthFail asserts a 401 surfaces as a tool-result error
// carrying the unauthorized hint.
func TestADBDevicesAuthFail(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	})
	c := newTestHubServer(t, h)
	res := callADBDevices(t, c, map[string]any{})
	msg := errorText(t, res)
	if !strings.Contains(msg, "unauthorized") {
		t.Errorf("error %q missing 'unauthorized'", msg)
	}
}

// TestADBDevicesEmptyResultEmitsArray asserts an empty hub response
// renders as `{"devices":[]}` not `{"devices":null}`.
func TestADBDevicesEmptyResultEmitsArray(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})
	c := newTestHubServer(t, h)
	res := callADBDevices(t, c, map[string]any{})
	if res.IsError {
		t.Fatalf("unexpected error: %s", errorText(t, res))
	}
	tc := res.Content[0].(mcp.TextContent)
	if !strings.Contains(tc.Text, `"devices":[]`) {
		t.Errorf("expected devices:[] in body, got %q", tc.Text)
	}
}

// ===========================================================================
// adb_start
// ===========================================================================

// TestADBStartToolRegistered confirms the real tool is registered.
func TestADBStartToolRegistered(t *testing.T) {
	t.Parallel()
	c := newTestHubServer(t, http.NotFoundHandler())
	s := buildServer(c)
	tools := s.ListTools()
	if _, ok := tools["adb_start"]; !ok {
		t.Errorf("adb_start missing from registry; got %v", toolNames(tools))
	}
}

// TestADBStartDescriptionPresent guards a non-empty description that
// mentions the key knobs (device_serial / tag_filter).
func TestADBStartDescriptionPresent(t *testing.T) {
	t.Parallel()
	c := newTestHubServer(t, http.NotFoundHandler())
	s := buildServer(c)
	st := s.ListTools()["adb_start"]
	if st == nil {
		t.Fatal("adb_start not registered")
	}
	desc := strings.TrimSpace(st.Tool.Description)
	if desc == "" {
		t.Fatal("adb_start has empty Description")
	}
	for _, want := range []string{"device_serial", "tag_filter"} {
		if !strings.Contains(strings.ToLower(desc), want) {
			t.Errorf("description %q does not mention %q", desc, want)
		}
	}
}

// TestADBStartInputSchemaAccepts exercises the canonical argument shapes
// per ADR-007: serial-only and serial+tag_filter.
func TestADBStartInputSchemaAccepts(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"started","serial":"emulator-5554","started_at":1700000000000000000}`))
	})
	c := newTestHubServer(t, h)
	cases := []struct {
		name string
		args map[string]any
	}{
		{"serial only", map[string]any{"device_serial": "emulator-5554"}},
		{"with tag_filter", map[string]any{"device_serial": "emulator-5554", "tag_filter": "MyTag"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := callADBStart(t, c, tc.args)
			if res.IsError {
				t.Errorf("unexpected error result: %s", errorText(t, res))
			}
		})
	}
}

// TestADBStartInputSchemaWrongTypesTolerated documents the mcp-go v0.45.0
// behaviour: string-where-string-expected is fine; an unexpected number for
// device_serial coerces to "" via GetString's default and fails fast with
// the "device_serial required" tool-result error. Tripwire test.
func TestADBStartInputSchemaWrongTypesTolerated(t *testing.T) {
	t.Parallel()
	called := false
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"started","serial":"x","started_at":0}`))
	})
	c := newTestHubServer(t, h)
	// Wrong-typed device_serial: number-where-string → GetString returns ""
	// → handler fail-fast, no hub round-trip.
	res := callADBStart(t, c, map[string]any{"device_serial": float64(12345)})
	if !res.IsError {
		t.Errorf("expected fail-fast IsError for wrong-typed device_serial")
	}
	if called {
		t.Error("hub was contacted despite wrong-typed device_serial — expected fail-fast")
	}
}

// TestADBStartMissingDeviceSerialFailsFast asserts an absent or empty
// device_serial fails inside the handler with a tool-result error — no
// hub round-trip.
func TestADBStartMissingDeviceSerialFailsFast(t *testing.T) {
	t.Parallel()
	called := false
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		http.Error(w, `nope`, http.StatusInternalServerError)
	})
	c := newTestHubServer(t, h)
	for _, args := range []map[string]any{
		{},                          // absent
		{"device_serial": ""},       // empty
		{"tag_filter": "irrelevant"}, // serial absent, other args present
	} {
		res := callADBStart(t, c, args)
		msg := errorText(t, res)
		if !strings.Contains(msg, "device_serial required") {
			t.Errorf("args %v: error %q missing 'device_serial required'", args, msg)
		}
	}
	if called {
		t.Error("hub was contacted despite missing device_serial — expected fail-fast")
	}
}

// TestADBStartHandlerCallsHub exercises the happy path: bearer-attached
// POST /adb/start, status discriminator passed through.
func TestADBStartHandlerCallsHub(t *testing.T) {
	t.Parallel()
	var gotAuth, gotPath, gotMethod string
	var gotBody string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotMethod = r.Method
		buf := make([]byte, 256)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"started","serial":"emulator-5554","started_at":1700000000000000000}`))
	})
	c := newTestHubServer(t, h)

	res := callADBStart(t, c, map[string]any{"device_serial": "emulator-5554"})
	body := decodeADBStartBody(t, res)

	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want Bearer test-token", gotAuth)
	}
	if gotMethod != http.MethodPost || gotPath != "/adb/start" {
		t.Errorf("hub req = %s %s, want POST /adb/start", gotMethod, gotPath)
	}
	if !strings.Contains(gotBody, `"serial":"emulator-5554"`) {
		t.Errorf("hub req body missing serial: %s", gotBody)
	}
	if body.Status != "started" {
		t.Errorf("status = %q, want 'started' (discriminator pass-through)", body.Status)
	}
	if body.DeviceSerial != "emulator-5554" {
		t.Errorf("device_serial = %q, want emulator-5554", body.DeviceSerial)
	}
}

// TestADBStartAlreadyRunningDiscriminator asserts that an idempotent
// already_running response from the hub surfaces with status="already_
// running" — the discriminator pass-through is the entire point of the
// P2b-S5 client signature extension.
func TestADBStartAlreadyRunningDiscriminator(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"already_running","serial":"emulator-5554","started_at":1700000000000000000}`))
	})
	c := newTestHubServer(t, h)
	res := callADBStart(t, c, map[string]any{"device_serial": "emulator-5554"})
	body := decodeADBStartBody(t, res)
	if body.Status != "already_running" {
		t.Errorf("status = %q, want 'already_running'", body.Status)
	}
	if body.DeviceSerial != "emulator-5554" {
		t.Errorf("device_serial = %q, want emulator-5554", body.DeviceSerial)
	}
}

// TestADBStartTagFilterAcceptedAndIgnored asserts that a non-empty
// tag_filter argument is accepted (no error), does NOT show up in the
// hub request body (the hub /adb/start endpoint has no tag_filter
// field), and the call still succeeds. This pins the P2b-S5 decision
// to accept-and-log-warn rather than reject — the warn itself goes to
// slog and is not asserted here (would require a log handler swap).
func TestADBStartTagFilterAcceptedAndIgnored(t *testing.T) {
	t.Parallel()
	var gotBody string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 256)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"started","serial":"emulator-5554","started_at":1700000000000000000}`))
	})
	c := newTestHubServer(t, h)

	res := callADBStart(t, c, map[string]any{
		"device_serial": "emulator-5554",
		"tag_filter":    "MyTag",
	})
	if res.IsError {
		t.Fatalf("expected success, got error: %s", errorText(t, res))
	}
	// tag_filter must NOT appear in the hub request body — the bridge
	// takes tag_filter from hub-side [adb] config.
	if strings.Contains(gotBody, "tag_filter") {
		t.Errorf("tag_filter leaked into hub request body: %s", gotBody)
	}
	if strings.Contains(gotBody, "MyTag") {
		t.Errorf("tag_filter value leaked into hub request body: %s", gotBody)
	}
}

// TestADBStartAuthFail asserts a 401 surfaces as a tool-result error.
func TestADBStartAuthFail(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	})
	c := newTestHubServer(t, h)
	res := callADBStart(t, c, map[string]any{"device_serial": "x"})
	msg := errorText(t, res)
	if !strings.Contains(msg, "unauthorized") {
		t.Errorf("error %q missing 'unauthorized'", msg)
	}
}

// ===========================================================================
// adb_stop
// ===========================================================================

// TestADBStopToolRegistered confirms the real tool is registered.
func TestADBStopToolRegistered(t *testing.T) {
	t.Parallel()
	c := newTestHubServer(t, http.NotFoundHandler())
	s := buildServer(c)
	tools := s.ListTools()
	if _, ok := tools["adb_stop"]; !ok {
		t.Errorf("adb_stop missing from registry; got %v", toolNames(tools))
	}
}

// TestADBStopDescriptionPresent guards a non-empty description that
// mentions device_serial.
func TestADBStopDescriptionPresent(t *testing.T) {
	t.Parallel()
	c := newTestHubServer(t, http.NotFoundHandler())
	s := buildServer(c)
	st := s.ListTools()["adb_stop"]
	if st == nil {
		t.Fatal("adb_stop not registered")
	}
	desc := strings.TrimSpace(st.Tool.Description)
	if desc == "" {
		t.Fatal("adb_stop has empty Description")
	}
	if !strings.Contains(strings.ToLower(desc), "device_serial") {
		t.Errorf("description %q does not mention device_serial", desc)
	}
}

// TestADBStopInputSchemaAccepts exercises the serial-only shape.
func TestADBStopInputSchemaAccepts(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"stopped","serial":"emulator-5554"}`))
	})
	c := newTestHubServer(t, h)
	res := callADBStop(t, c, map[string]any{"device_serial": "emulator-5554"})
	if res.IsError {
		t.Errorf("unexpected error result: %s", errorText(t, res))
	}
}

// TestADBStopMissingDeviceSerialFailsFast asserts an absent or empty
// device_serial fails inside the handler with a tool-result error — no
// hub round-trip.
func TestADBStopMissingDeviceSerialFailsFast(t *testing.T) {
	t.Parallel()
	called := false
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		http.Error(w, `nope`, http.StatusInternalServerError)
	})
	c := newTestHubServer(t, h)
	for _, args := range []map[string]any{
		{},                    // absent
		{"device_serial": ""}, // empty
	} {
		res := callADBStop(t, c, args)
		msg := errorText(t, res)
		if !strings.Contains(msg, "device_serial required") {
			t.Errorf("args %v: error %q missing 'device_serial required'", args, msg)
		}
	}
	if called {
		t.Error("hub was contacted despite missing device_serial — expected fail-fast")
	}
}

// TestADBStopHandlerCallsHub exercises the happy path: bearer-attached
// POST /adb/stop, status discriminator passed through.
func TestADBStopHandlerCallsHub(t *testing.T) {
	t.Parallel()
	var gotAuth, gotPath, gotMethod string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"stopped","serial":"emulator-5554"}`))
	})
	c := newTestHubServer(t, h)

	res := callADBStop(t, c, map[string]any{"device_serial": "emulator-5554"})
	body := decodeADBStopBody(t, res)

	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want Bearer test-token", gotAuth)
	}
	if gotMethod != http.MethodPost || gotPath != "/adb/stop" {
		t.Errorf("hub req = %s %s, want POST /adb/stop", gotMethod, gotPath)
	}
	if body.Status != "stopped" {
		t.Errorf("status = %q, want 'stopped' (discriminator pass-through)", body.Status)
	}
	if body.DeviceSerial != "emulator-5554" {
		t.Errorf("device_serial = %q, want emulator-5554", body.DeviceSerial)
	}
}

// TestADBStopNotRunningDiscriminator asserts the idempotent not_running
// response from the hub surfaces with status="not_running" — same
// pattern as TestADBStartAlreadyRunningDiscriminator.
func TestADBStopNotRunningDiscriminator(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"not_running","serial":"ghost"}`))
	})
	c := newTestHubServer(t, h)
	res := callADBStop(t, c, map[string]any{"device_serial": "ghost"})
	body := decodeADBStopBody(t, res)
	if body.Status != "not_running" {
		t.Errorf("status = %q, want 'not_running'", body.Status)
	}
	if body.DeviceSerial != "ghost" {
		t.Errorf("device_serial = %q, want ghost", body.DeviceSerial)
	}
}

// TestADBStopAuthFail asserts a 401 surfaces as a tool-result error.
func TestADBStopAuthFail(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	})
	c := newTestHubServer(t, h)
	res := callADBStop(t, c, map[string]any{"device_serial": "x"})
	msg := errorText(t, res)
	if !strings.Contains(msg, "unauthorized") {
		t.Errorf("error %q missing 'unauthorized'", msg)
	}
}
