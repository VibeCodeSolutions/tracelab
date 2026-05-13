package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestServer is a tiny convenience for the per-endpoint tests in this
// package: it wires an httptest.Server with the given handler and
// returns a Client pointed at it plus the server (caller closes via
// t.Cleanup).
func newTestServer(t *testing.T, h http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := New(Config{BaseURL: srv.URL, Token: "test-token"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c, srv
}

func TestNew_Validation(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantSub string
	}{
		{"empty BaseURL", Config{Token: "x"}, "BaseURL is required"},
		{"bad scheme", Config{BaseURL: "ftp://nope", Token: "x"}, "scheme must be http or https"},
		{"no host", Config{BaseURL: "http://", Token: "x"}, "no host"},
		{"empty token", Config{BaseURL: "http://127.0.0.1:1", Token: ""}, "Token is required"},
		{"placeholder token", Config{BaseURL: "http://127.0.0.1:1", Token: "CHANGEME"}, "placeholder"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.cfg)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestNew_DefaultTimeout(t *testing.T) {
	c, err := New(Config{BaseURL: "http://127.0.0.1:1", Token: "x"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.httpC.Timeout != DefaultTimeout {
		t.Errorf("Timeout = %v, want %v", c.httpC.Timeout, DefaultTimeout)
	}
}

func TestNew_CustomTimeout(t *testing.T) {
	want := 3 * time.Second
	c, err := New(Config{BaseURL: "http://127.0.0.1:1", Token: "x", Timeout: want})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.httpC.Timeout != want {
		t.Errorf("Timeout = %v, want %v", c.httpC.Timeout, want)
	}
}

func TestNew_TrimsTrailingSlash(t *testing.T) {
	c, err := New(Config{BaseURL: "http://h.local:1234/", Token: "x"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := c.baseURL.String(); strings.HasSuffix(got, "/") {
		t.Errorf("baseURL %q still has trailing slash", got)
	}
}

// TestDoRequest_ContextCanceled drives the cancellation path by hanging
// the server until ctx is cancelled. The expectation is that the call
// returns ctx.Err() promptly.
func TestDoRequest_ContextCanceled(t *testing.T) {
	srvCtx, srvCancel := context.WithCancel(context.Background())
	defer srvCancel()
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-srvCtx.Done() // hang until server is torn down
	})
	c, _ := newTestServer(t, h)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.Health(ctx)
	}()
	// Cancel after a short delay so the request is already in-flight.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("call did not return within 2s after cancel")
	}
}

// TestHTTPError_Unwrap exercises errors.Is on both sentinels.
func TestHTTPError_Unwrap(t *testing.T) {
	e := &HTTPError{Status: 401, Endpoint: "/x", inner: ErrUnauthorized}
	if !errors.Is(e, ErrUnauthorized) {
		t.Errorf("errors.Is(ErrUnauthorized) = false, want true")
	}
	e2 := &HTTPError{Status: 500, Endpoint: "/x", inner: ErrServerError}
	if !errors.Is(e2, ErrServerError) {
		t.Errorf("errors.Is(ErrServerError) = false, want true")
	}
}
