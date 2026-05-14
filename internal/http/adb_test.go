package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/VibeCodeSolutions/tracelab/internal/adb"
	httplayer "github.com/VibeCodeSolutions/tracelab/internal/http"
	"github.com/VibeCodeSolutions/tracelab/internal/store"
)

// fakeLister implements httplayer.ADBDeviceLister for tests. Devices and
// the error returned are pinned by the test; concurrent calls are safe.
type fakeLister struct {
	mu      sync.Mutex
	devices []adb.Device
	err     error
	calls   int
}

func (f *fakeLister) Devices(_ context.Context) ([]adb.Device, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	// Defensive copy so a test that mutates the input after construction
	// doesn't accidentally race the handler.
	out := make([]adb.Device, len(f.devices))
	copy(out, f.devices)
	return out, nil
}

// fakeManager implements httplayer.ADBManager. running[serial] = true
// indicates the bridge is currently up; Start/Stop mutate the map under a
// mutex and surface the manager-sentinel errors when appropriate.
type fakeManager struct {
	mu         sync.Mutex
	running    map[string]int64 // serial -> startedAt
	failStart  error
	failStop   error
	nextStart  int64 // pinned StartedAt for assertion; 0 = use a non-zero fallback
}

func newFakeManager() *fakeManager {
	return &fakeManager{running: map[string]int64{}}
}

func (m *fakeManager) Start(opts adb.BridgeStartOptions) (adb.BridgeStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failStart != nil {
		return adb.BridgeStatus{}, m.failStart
	}
	if existing, ok := m.running[opts.DeviceSerial]; ok {
		return adb.BridgeStatus{
			DeviceSerial: opts.DeviceSerial,
			StartedAt:    existing,
		}, adb.ErrBridgeAlreadyRunning
	}
	ts := m.nextStart
	if ts == 0 {
		ts = 1700000000_000000000
	}
	m.running[opts.DeviceSerial] = ts
	return adb.BridgeStatus{
		DeviceSerial: opts.DeviceSerial,
		StartedAt:    ts,
	}, nil
}

func (m *fakeManager) Stop(serial string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failStop != nil {
		return m.failStop
	}
	if _, ok := m.running[serial]; !ok {
		return adb.ErrBridgeNotRunning
	}
	delete(m.running, serial)
	return nil
}

// newADBTestServer wires a fresh hub HTTP server with the given device
// lister and bridge manager. Use nil for either to omit the corresponding
// routes (mirrors how cmd/hub conditionally wires them).
func newADBTestServer(t *testing.T, lister httplayer.ADBDeviceLister, manager httplayer.ADBManager) *httptest.Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tracelab.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	h := httplayer.New(st, httplayer.Config{
		AuthToken:       testToken,
		ADBManager:      manager,
		ADBDeviceLister: lister,
	})
	if h == nil {
		t.Fatal("httplayer.New returned nil")
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

// --- GET /adb/devices ---

func TestADBDevices_Happy(t *testing.T) {
	lister := &fakeLister{devices: []adb.Device{
		{Serial: "emulator-5554", State: "device", Model: "sdk_gphone64_x86_64"},
		{Serial: "AB12CD34", State: "unauthorized"},
	}}
	srv := newADBTestServer(t, lister, newFakeManager())

	resp := doJSON(t, srv, http.MethodGet, "/adb/devices", testToken, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0]["serial"] != "emulator-5554" || got[0]["state"] != "device" || got[0]["model"] != "sdk_gphone64_x86_64" {
		t.Errorf("first device unexpected: %+v", got[0])
	}
	// Second device has no model — omitempty must skip the key.
	if _, ok := got[1]["model"]; ok {
		t.Errorf("model key must be omitted for unauthorized device: %+v", got[1])
	}
}

func TestADBDevices_EmptyList_RendersArray(t *testing.T) {
	lister := &fakeLister{devices: nil}
	srv := newADBTestServer(t, lister, newFakeManager())

	resp := doJSON(t, srv, http.MethodGet, "/adb/devices", testToken, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	// The wire body must be `[]`, never `null` — clients range over the
	// response and would crash on null.
	body, _ := io.ReadAll(resp.Body)
	if strings.TrimSpace(string(body)) != "[]" {
		t.Errorf("body = %q, want `[]`", strings.TrimSpace(string(body)))
	}
}

func TestADBDevices_NoToken_Unauthorized(t *testing.T) {
	lister := &fakeLister{}
	srv := newADBTestServer(t, lister, newFakeManager())

	resp := doJSON(t, srv, http.MethodGet, "/adb/devices", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	if lister.calls != 0 {
		t.Errorf("lister must not be invoked when auth fails (got %d calls)", lister.calls)
	}
}

func TestADBDevices_ListerError_ServiceUnavailable(t *testing.T) {
	lister := &fakeLister{err: errors.New("adb daemon not running")}
	srv := newADBTestServer(t, lister, newFakeManager())

	resp := doJSON(t, srv, http.MethodGet, "/adb/devices", testToken, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
}

func TestADBDevices_NotRegistered_When_Lister_Nil(t *testing.T) {
	// Manager is wired but lister is nil — GET /adb/devices must 404 because
	// the route is not registered. (Bearer auth still applies, so we expect
	// 404 with a valid token — chi's NotFound bypasses the auth wrapper for
	// unmatched routes? actually no, the chi group wraps everything so an
	// unmatched route inside the group still goes through auth. With a
	// valid token it should be 404, with no token it should be 401. We
	// assert the valid-token case.)
	srv := newADBTestServer(t, nil, newFakeManager())
	resp := doJSON(t, srv, http.MethodGet, "/adb/devices", testToken, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (route not registered)", resp.StatusCode)
	}
}

// --- POST /adb/start ---

func TestADBStart_Fresh(t *testing.T) {
	mgr := newFakeManager()
	srv := newADBTestServer(t, &fakeLister{}, mgr)

	resp := doJSON(t, srv, http.MethodPost, "/adb/start", testToken, map[string]string{
		"serial": "emulator-5554",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Status    string `json:"status"`
		Serial    string `json:"serial"`
		StartedAt int64  `json:"started_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "started" {
		t.Errorf("status = %q, want started", body.Status)
	}
	if body.Serial != "emulator-5554" {
		t.Errorf("serial = %q", body.Serial)
	}
	if body.StartedAt == 0 {
		t.Errorf("started_at unset")
	}
}

func TestADBStart_AlreadyRunning_Idempotent(t *testing.T) {
	mgr := newFakeManager()
	mgr.running["emu-1"] = 1700000000_000000000
	srv := newADBTestServer(t, &fakeLister{}, mgr)

	resp := doJSON(t, srv, http.MethodPost, "/adb/start", testToken, map[string]string{
		"serial": "emu-1",
	})
	defer resp.Body.Close()
	// Idempotency-Entscheid: 200 OK + status="already_running" (siehe
	// adb.go-Kommentar).
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (idempotent)", resp.StatusCode)
	}
	var body struct {
		Status    string `json:"status"`
		StartedAt int64  `json:"started_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "already_running" {
		t.Errorf("status = %q, want already_running", body.Status)
	}
	if body.StartedAt != 1700000000_000000000 {
		t.Errorf("started_at = %d, want echo of original", body.StartedAt)
	}
}

func TestADBStart_NoToken_Unauthorized(t *testing.T) {
	mgr := newFakeManager()
	srv := newADBTestServer(t, &fakeLister{}, mgr)

	resp := doJSON(t, srv, http.MethodPost, "/adb/start", "", map[string]string{
		"serial": "emu-1",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestADBStart_EmptySerial_BadRequest(t *testing.T) {
	mgr := newFakeManager()
	srv := newADBTestServer(t, &fakeLister{}, mgr)

	resp := doJSON(t, srv, http.MethodPost, "/adb/start", testToken, map[string]string{
		"serial": "",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestADBStart_BadJSON_BadRequest(t *testing.T) {
	mgr := newFakeManager()
	srv := newADBTestServer(t, &fakeLister{}, mgr)

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/adb/start",
		bytes.NewReader([]byte(`{`)))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// --- POST /adb/stop ---

func TestADBStop_Running(t *testing.T) {
	mgr := newFakeManager()
	mgr.running["emu-1"] = 1700000000_000000000
	srv := newADBTestServer(t, &fakeLister{}, mgr)

	resp := doJSON(t, srv, http.MethodPost, "/adb/stop", testToken, map[string]string{
		"serial": "emu-1",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct{ Status, Serial string }
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "stopped" {
		t.Errorf("status = %q, want stopped", body.Status)
	}
	if body.Serial != "emu-1" {
		t.Errorf("serial = %q", body.Serial)
	}
	// Manager must reflect the change.
	if _, ok := mgr.running["emu-1"]; ok {
		t.Errorf("manager state still has emu-1 after stop")
	}
}

func TestADBStop_NotRunning_Idempotent(t *testing.T) {
	mgr := newFakeManager()
	srv := newADBTestServer(t, &fakeLister{}, mgr)

	resp := doJSON(t, srv, http.MethodPost, "/adb/stop", testToken, map[string]string{
		"serial": "ghost",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (idempotent)", resp.StatusCode)
	}
	var body struct{ Status string }
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "not_running" {
		t.Errorf("status = %q, want not_running", body.Status)
	}
}

func TestADBStop_NoToken_Unauthorized(t *testing.T) {
	mgr := newFakeManager()
	srv := newADBTestServer(t, &fakeLister{}, mgr)

	resp := doJSON(t, srv, http.MethodPost, "/adb/stop", "", map[string]string{
		"serial": "emu-1",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestADBStop_EmptySerial_BadRequest(t *testing.T) {
	mgr := newFakeManager()
	srv := newADBTestServer(t, &fakeLister{}, mgr)

	resp := doJSON(t, srv, http.MethodPost, "/adb/stop", testToken, map[string]string{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}
