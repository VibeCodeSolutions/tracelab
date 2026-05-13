package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultTimeout is applied to outgoing requests when Config.Timeout is
// zero. Long-lived endpoints (WebSocket /tail in S4) will override this
// on a per-method basis.
const DefaultTimeout = 10 * time.Second

// errBodyLimit caps how many bytes of a non-2xx response we read into
// HTTPError.Body. 1 KiB is enough for the hub's compact JSON error
// objects without unbounded growth from a misbehaving upstream.
const errBodyLimit = 1024

// placeholderToken is the example value carried by tracelab.toml.example.
// New() rejects it explicitly so an unconfigured CLI never silently
// authenticates against a real hub.
const placeholderToken = "CHANGEME"

// Config configures a Client. BaseURL must be a parsable http or https
// URL (no trailing slash required; either form is accepted). Token is
// the shared bearer secret from the hub's [auth].token. Timeout defaults
// to DefaultTimeout when zero.
type Config struct {
	BaseURL string
	Token   string
	Timeout time.Duration
}

// Client is a thread-safe HTTP client for the tracelab hub. All methods
// take a context.Context — cancellation cancels the in-flight request.
//
// The zero value is not usable; construct via New().
type Client struct {
	baseURL *url.URL
	token   string
	httpC   *http.Client
}

// New validates cfg and returns a ready-to-use Client. It returns an
// error if BaseURL is empty / unparsable / has a non-http(s) scheme, or
// if Token is empty or still set to the example placeholder
// ("CHANGEME") — the hub applies the same refusal at startup so the
// client side mirrors that contract.
func New(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("client: BaseURL is required")
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("client: invalid BaseURL %q: %w", cfg.BaseURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("client: BaseURL scheme must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("client: BaseURL %q has no host", cfg.BaseURL)
	}
	if cfg.Token == "" {
		return nil, errors.New("client: Token is required")
	}
	if cfg.Token == placeholderToken {
		return nil, fmt.Errorf("client: Token is still the placeholder %q — set [auth].token in tracelab.toml", placeholderToken)
	}
	// Drop any trailing slash so url-join is stable.
	u.Path = strings.TrimRight(u.Path, "/")

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	return &Client{
		baseURL: u,
		token:   cfg.Token,
		httpC:   &http.Client{Timeout: timeout},
	}, nil
}

// requestOpts bundles the per-request knobs for doRequest. The zero
// value is a GET to `path` with no body, no auth, no response decoding.
type requestOpts struct {
	method   string
	path     string // begins with "/"
	body     any    // marshalled to JSON if non-nil
	auth     bool   // attach Authorization: Bearer <token>
	respInto any    // pointer the 2xx JSON body decodes into; nil = drop body
}

// doRequest is the shared HTTP path. It builds the request URL by
// joining baseURL with opts.path, JSON-encodes the body (when non-nil),
// sets the bearer header (when auth is true), executes the call, and
// maps the response status:
//
//   - 2xx with respInto != nil  → decode body into respInto
//   - 2xx with respInto == nil  → drain & discard
//   - 401/403                    → ErrUnauthorized (wrapped in *HTTPError)
//   - 5xx                        → ErrServerError  (wrapped in *HTTPError)
//   - other 4xx                  → *HTTPError without sentinel
func (c *Client) doRequest(ctx context.Context, opts requestOpts) error {
	endpoint := c.baseURL.String() + opts.path

	var body io.Reader
	if opts.body != nil {
		buf, err := json.Marshal(opts.body)
		if err != nil {
			return fmt.Errorf("client: marshal %s body: %w", opts.path, err)
		}
		body = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, opts.method, endpoint, body)
	if err != nil {
		return fmt.Errorf("client: build request %s %s: %w", opts.method, opts.path, err)
	}
	if opts.body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if opts.auth {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpC.Do(req)
	if err != nil {
		// Surface context errors verbatim so callers can branch on
		// errors.Is(err, context.Canceled) / context.DeadlineExceeded.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("client: %s %s: %w", opts.method, opts.path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if opts.respInto == nil {
			// Drain so the connection can be reused.
			_, _ = io.Copy(io.Discard, resp.Body)
			return nil
		}
		if err := json.NewDecoder(resp.Body).Decode(opts.respInto); err != nil {
			return fmt.Errorf("client: %s %s: decode response: %w", opts.method, opts.path, err)
		}
		return nil
	}

	// Non-2xx: capture a body snippet for diagnostics.
	snippet := readSnippet(resp.Body)
	httpErr := &HTTPError{
		Status:   resp.StatusCode,
		Endpoint: opts.path,
		Body:     snippet,
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		httpErr.inner = ErrUnauthorized
	case resp.StatusCode >= 500:
		httpErr.inner = ErrServerError
	}
	return httpErr
}

// readSnippet reads up to errBodyLimit bytes from r and returns the
// trimmed string. Errors are swallowed — the snippet is best-effort
// diagnostic context, not load-bearing.
func readSnippet(r io.Reader) string {
	buf, _ := io.ReadAll(io.LimitReader(r, errBodyLimit))
	return strings.TrimSpace(string(buf))
}
