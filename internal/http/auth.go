package http

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// bearerAuth returns a middleware that enforces `Authorization: Bearer <token>`
// against the configured token using a constant-time comparison.
//
// 401 is returned for both missing and mismatched tokens. The body is a
// minimal JSON object — no detail leak about which check failed.
func bearerAuth(expected string) func(http.Handler) http.Handler {
	expectedBytes := []byte(expected)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			const prefix = "Bearer "
			if !strings.HasPrefix(h, prefix) {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			got := []byte(strings.TrimPrefix(h, prefix))
			if subtle.ConstantTimeCompare(got, expectedBytes) != 1 {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
