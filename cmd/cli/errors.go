// Error-translation helpers shared by every sub-command that talks to the
// hub via internal/client. Lives in its own file because three consumers
// (sessions, tail, adb) all need the same client→user-message mapping —
// extracted in S5 when adb became the third caller, per the Belanna
// bookmark recorded in WORKLOG #014.
//
// Contract:
//
//   - userErrorMsg + userError + asUserError model the "user-facing message"
//     pattern. main() inspects asUserError to render a clean
//     `tracelab: <msg>` stderr line instead of cobra's default error
//     formatting (which would leak Go internals on a wrapped chain).
//   - translateClientError maps a *client.HTTPError / sentinel /
//     context-timeout / generic transport error to a userErrorMsg. Hub URL
//     is included for the "which place is wrong" anchor.
//   - leafErrorMessage walks errors.Unwrap to the deepest leaf so generic
//     transport failures surface as a clean root cause (e.g. "connection
//     refused") rather than the full nested net/http wrap chain.
//
// All three signatures are unchanged from the inline form that previously
// lived in sessions.go — see the git history for the pre-extraction
// version (#013/#014). Tests in sessions_test.go and tail_test.go exercise
// this path; their behaviour MUST stay green after the move (regression
// check is part of the S5 DoD).
package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/VibeCodeSolutions/tracelab/internal/client"
	"github.com/VibeCodeSolutions/tracelab/internal/cliconfig"
)

// userErrorMsg is a sentinel-typed error that signals "this is a
// user-facing message; do NOT print a stack trace". main() inspects this
// via asUserError to decide whether to render `tracelab: <msg>` (clean)
// or the default cobra rendering.
type userErrorMsg string

// Error implements error.
func (u userErrorMsg) Error() string { return string(u) }

// userError wraps a plain message into a userErrorMsg.
func userError(msg string) error { return userErrorMsg(msg) }

// asUserError returns the message and true when err carries a userErrorMsg
// somewhere in its chain. main() uses this to choose between clean
// stderr-print and the default cobra rendering.
func asUserError(err error) (string, bool) {
	var u userErrorMsg
	if errors.As(err, &u) {
		return string(u), true
	}
	return "", false
}

// translateClientError maps a *client.HTTPError or sentinel to a short,
// actionable user message. Hub URL is included to help the user verify
// they are talking to the right place.
func translateClientError(err error, resolved *cliconfig.Resolved) error {
	if errors.Is(err, client.ErrUnauthorized) {
		return userError("unauthorized — check token in tracelab.toml or TRACELAB_TOKEN")
	}
	if errors.Is(err, client.ErrServerError) {
		var he *client.HTTPError
		if errors.As(err, &he) {
			return userError(fmt.Sprintf("hub error (HTTP %d) from %s", he.Status, resolved.BaseURL))
		}
		return userError(fmt.Sprintf("hub error from %s", resolved.BaseURL))
	}
	var he *client.HTTPError
	if errors.As(err, &he) {
		return userError(fmt.Sprintf("hub responded HTTP %d for %s", he.Status, he.Endpoint))
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return userError(fmt.Sprintf("timeout contacting hub at %s", resolved.BaseURL))
	}
	// Generic connection failure — surface the BaseURL plus only the
	// innermost cause string, never the wrapped chain. The Go HTTP stack
	// wraps transport errors as:
	//
	//   client: GET /sessions: Get "http://host:port/sessions":
	//       dial tcp 127.0.0.1:1234: connect: connection refused
	//
	// Walking errors.Unwrap to the leaf yields just "connection refused"
	// (or "i/o timeout", "no such host", …) — actionable to the user,
	// without leaking host/port/syscall details that change between OSes.
	return userError(fmt.Sprintf("cannot reach hub at %s: %s", resolved.BaseURL, leafErrorMessage(err)))
}

// leafErrorMessage walks the errors.Unwrap chain to the deepest non-nil
// link and returns its Error() string. Used by translateClientError to
// strip the Go HTTP stack's nested "GET /foo: Get \"http://...\": dial
// tcp <addr>: connect: …" wrap noise down to the actionable root cause.
//
// Falls back to err.Error() when the chain is single-link.
func leafErrorMessage(err error) string {
	for {
		next := errors.Unwrap(err)
		if next == nil {
			return err.Error()
		}
		err = next
	}
}
