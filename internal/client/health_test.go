package client

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestHealth_OK(t *testing.T) {
	var sawAuth bool
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/healthz" {
			t.Errorf("unexpected req: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "" {
			sawAuth = true
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	c, _ := newTestServer(t, h)
	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
	if sawAuth {
		t.Error("Health must NOT send Authorization header (endpoint is no-auth)")
	}
}

func TestHealth_ServerError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	c, _ := newTestServer(t, h)
	err := c.Health(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrServerError) {
		t.Errorf("expected ErrServerError, got %v", err)
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *HTTPError, got %T", err)
	}
	if httpErr.Status != http.StatusInternalServerError {
		t.Errorf("Status = %d, want 500", httpErr.Status)
	}
	if httpErr.Endpoint != "/healthz" {
		t.Errorf("Endpoint = %q, want /healthz", httpErr.Endpoint)
	}
}
