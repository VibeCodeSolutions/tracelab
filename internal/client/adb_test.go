package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// --- ListADBDevices ---

func TestListADBDevices_OK(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/adb/devices" {
			t.Errorf("unexpected req: %s %s", r.Method, r.URL.Path)
		}
		assertBearer(t, r, "test-token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"serial":"emulator-5554","state":"device","model":"sdk_gphone64_x86_64"},
			{"serial":"AB12CD34","state":"unauthorized"}
		]`))
	})
	c, _ := newTestServer(t, h)
	got, err := c.ListADBDevices(context.Background())
	if err != nil {
		t.Fatalf("ListADBDevices: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Serial != "emulator-5554" || got[0].State != "device" || got[0].Model != "sdk_gphone64_x86_64" {
		t.Errorf("first device: %+v", got[0])
	}
	if got[1].Model != "" {
		t.Errorf("second device Model should be empty (omitempty wire), got %q", got[1].Model)
	}
}

func TestListADBDevices_Empty_NonNil(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})
	c, _ := newTestServer(t, h)
	got, err := c.ListADBDevices(context.Background())
	if err != nil {
		t.Fatalf("ListADBDevices: %v", err)
	}
	if got == nil {
		t.Fatalf("expected non-nil empty slice, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(got))
	}
}

func TestListADBDevices_Unauthorized(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	})
	c, _ := newTestServer(t, h)
	_, err := c.ListADBDevices(context.Background())
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestListADBDevices_ServiceUnavailable(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"adb unavailable"}`, http.StatusServiceUnavailable)
	})
	c, _ := newTestServer(t, h)
	_, err := c.ListADBDevices(context.Background())
	// 503 maps to ErrServerError via the >=500 branch in doRequest.
	if !errors.Is(err, ErrServerError) {
		t.Errorf("503 must map to ErrServerError, got %v", err)
	}
}

func TestListADBDevices_ContextCancel(t *testing.T) {
	srvCtx, srvCancel := context.WithCancel(context.Background())
	defer srvCancel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-srvCtx.Done()
	})
	c, _ := newTestServer(t, h)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := c.ListADBDevices(ctx)
		errCh <- err
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ListADBDevices did not return within 2s after cancel")
	}
}

// --- StartADBBridge ---

func TestStartADBBridge_Fresh(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/adb/start" {
			t.Errorf("unexpected req: %s %s", r.Method, r.URL.Path)
		}
		assertBearer(t, r, "test-token")
		body, _ := io.ReadAll(r.Body)
		var got adbStartReqWire
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("bad body %s: %v", body, err)
		}
		if got.Serial != "emulator-5554" || got.Session != "sess-9" {
			t.Errorf("body unexpected: %+v", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"started","serial":"emulator-5554","started_at":1700000000000000000}`))
	})
	c, _ := newTestServer(t, h)
	if err := c.StartADBBridge(context.Background(), "emulator-5554", "sess-9"); err != nil {
		t.Errorf("StartADBBridge: %v", err)
	}
}

func TestStartADBBridge_AlreadyRunning_Idempotent(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"already_running","serial":"emulator-5554","started_at":1700000000000000000}`))
	})
	c, _ := newTestServer(t, h)
	// Idempotent contract: an already-running bridge surfaces as nil.
	if err := c.StartADBBridge(context.Background(), "emulator-5554", ""); err != nil {
		t.Errorf("idempotent already_running should not error, got: %v", err)
	}
}

func TestStartADBBridge_EmptySerial(t *testing.T) {
	c, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("hub should not be called when serial is empty")
	}))
	err := c.StartADBBridge(context.Background(), "", "")
	if err == nil {
		t.Fatal("expected error for empty serial")
	}
}

func TestStartADBBridge_Unauthorized(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	})
	c, _ := newTestServer(t, h)
	err := c.StartADBBridge(context.Background(), "x", "")
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestStartADBBridge_BadRequest(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"serial required"}`, http.StatusBadRequest)
	})
	c, _ := newTestServer(t, h)
	err := c.StartADBBridge(context.Background(), "x", "")
	if err == nil {
		t.Fatal("expected error for 400")
	}
	var he *HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("expected *HTTPError, got %T", err)
	}
	if he.Status != http.StatusBadRequest {
		t.Errorf("Status = %d", he.Status)
	}
	if !strings.Contains(he.Body, "serial required") {
		t.Errorf("Body lost: %q", he.Body)
	}
}

func TestStartADBBridge_ContextCancel(t *testing.T) {
	srvCtx, srvCancel := context.WithCancel(context.Background())
	defer srvCancel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-srvCtx.Done()
	})
	c, _ := newTestServer(t, h)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.StartADBBridge(ctx, "emu", "")
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StartADBBridge did not return within 2s after cancel")
	}
}

// --- StopADBBridge ---

func TestStopADBBridge_Stopped(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/adb/stop" {
			t.Errorf("unexpected req: %s %s", r.Method, r.URL.Path)
		}
		assertBearer(t, r, "test-token")
		body, _ := io.ReadAll(r.Body)
		var got adbStopReqWire
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("bad body: %v", err)
		}
		if got.Serial != "emulator-5554" {
			t.Errorf("Serial = %q", got.Serial)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"stopped","serial":"emulator-5554"}`))
	})
	c, _ := newTestServer(t, h)
	if err := c.StopADBBridge(context.Background(), "emulator-5554"); err != nil {
		t.Errorf("StopADBBridge: %v", err)
	}
}

func TestStopADBBridge_NotRunning_Idempotent(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"not_running","serial":"ghost"}`))
	})
	c, _ := newTestServer(t, h)
	if err := c.StopADBBridge(context.Background(), "ghost"); err != nil {
		t.Errorf("idempotent not_running should not error, got: %v", err)
	}
}

func TestStopADBBridge_EmptySerial(t *testing.T) {
	c, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("hub should not be called when serial is empty")
	}))
	if err := c.StopADBBridge(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty serial")
	}
}

func TestStopADBBridge_Unauthorized(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	})
	c, _ := newTestServer(t, h)
	err := c.StopADBBridge(context.Background(), "x")
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestStopADBBridge_ContextCancel(t *testing.T) {
	srvCtx, srvCancel := context.WithCancel(context.Background())
	defer srvCancel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-srvCtx.Done()
	})
	c, _ := newTestServer(t, h)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.StopADBBridge(ctx, "emu")
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StopADBBridge did not return within 2s after cancel")
	}
}
