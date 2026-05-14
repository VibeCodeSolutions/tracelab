// Package cliconfig wires the `tracelab` CLI's runtime configuration: it
// discovers a tracelab.toml on disk (5-step search order from ADR-002),
// loads it via internal/config, and lets per-invocation flags / env vars
// override the resulting URL + token.
//
// Discovery order (highest priority wins; first non-empty path that
// actually exists is used):
//
//  1. --config <path>             (flagPath argument)
//  2. $TRACELAB_CONFIG            (env)
//  3. ./tracelab.toml             (cwd)
//  4. $XDG_CONFIG_HOME/tracelab/tracelab.toml
//  5. ~/.config/tracelab/tracelab.toml
//
// If steps 1 or 2 are set but the file does not exist, Discover returns
// the path as a hard error — explicit user intent that points at nothing
// must fail loudly. Steps 3-5 are tried in order and silently skipped
// when missing. If no file is found at all, Discover returns ("", nil)
// and Resolve will operate purely on flags/env.
//
// Override precedence (after a config file is loaded, or when none is):
//
//	flag  >  env  >  config file
//
// for both URL and token. The resulting Resolved struct is what
// cmd/cli/sessions.go feeds into client.New.
package cliconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/VibeCodeSolutions/tracelab/internal/config"
)

// Env var names — exposed as constants so cmd/cli/ and tests reference
// the same literals.
const (
	EnvConfig = "TRACELAB_CONFIG"
	EnvURL    = "TRACELAB_URL"
	EnvToken  = "TRACELAB_TOKEN"
)

// Sources describes per-invocation overrides. Empty strings mean "not
// set"; callers populate them from cobra flags before calling Resolve.
type Sources struct {
	// FlagConfigPath is the value of --config (empty if not passed).
	FlagConfigPath string
	// FlagURL is the value of --url (empty if not passed).
	FlagURL string
	// FlagToken is the value of --token (empty if not passed).
	FlagToken string
}

// Resolved is the merged, ready-to-use CLI runtime configuration.
type Resolved struct {
	// BaseURL is the hub URL the client should connect to, e.g.
	// "http://127.0.0.1:8765". Never empty when Resolve returns nil.
	BaseURL string
	// Token is the shared bearer secret. Never empty when Resolve
	// returns nil.
	Token string
	// CLI carries the [cli] section with defaults applied. The struct
	// is always populated, even when no config file was found.
	CLI config.CLIConfig
	// ConfigPath is the path of the config file that was loaded, or ""
	// when no file was found. Useful for diagnostics ("--token missing
	// in <path>").
	ConfigPath string
}

// envLookup matches os.LookupEnv. Tests pass a stub to keep them
// hermetic.
type envLookup func(string) (string, bool)

// fsExists is the file-existence probe. Tests stub it for paths that
// genuinely cannot be created (e.g. simulating a missing $HOME).
type fsExists func(string) bool

// resolver bundles the side-effect hooks so Discover and Resolve are
// testable without touching the real filesystem or environment.
type resolver struct {
	getenv envLookup
	exists fsExists
	// homeDir returns the user's home directory. Tests stub it.
	homeDir func() (string, error)
	// getwd returns the current working directory. Tests stub it.
	getwd func() (string, error)
}

// defaultResolver wires the real os.* implementations.
func defaultResolver() *resolver {
	return &resolver{
		getenv: os.LookupEnv,
		exists: func(p string) bool {
			_, err := os.Stat(p)
			return err == nil
		},
		homeDir: os.UserHomeDir,
		getwd:   os.Getwd,
	}
}

// Discover walks the 5-step search order and returns the first path that
// applies. Returns ("", nil) when no file is found in steps 3-5 and no
// explicit pointer was set. Returns ("", error) when an explicit pointer
// (--config or $TRACELAB_CONFIG) points at a non-existent file.
//
// flagPath is the raw --config value (empty if not passed).
func Discover(flagPath string) (string, error) {
	return defaultResolver().discover(flagPath)
}

func (r *resolver) discover(flagPath string) (string, error) {
	// 1. --config <path>
	if flagPath != "" {
		if !r.exists(flagPath) {
			return "", fmt.Errorf("cliconfig: --config %q: file does not exist", flagPath)
		}
		return flagPath, nil
	}

	// 2. $TRACELAB_CONFIG
	if v, ok := r.getenv(EnvConfig); ok && v != "" {
		if !r.exists(v) {
			return "", fmt.Errorf("cliconfig: %s=%q: file does not exist", EnvConfig, v)
		}
		return v, nil
	}

	// 3. ./tracelab.toml (cwd)
	if cwd, err := r.getwd(); err == nil {
		candidate := filepath.Join(cwd, "tracelab.toml")
		if r.exists(candidate) {
			return candidate, nil
		}
	}

	// 4. $XDG_CONFIG_HOME/tracelab/tracelab.toml
	if v, ok := r.getenv("XDG_CONFIG_HOME"); ok && v != "" {
		candidate := filepath.Join(v, "tracelab", "tracelab.toml")
		if r.exists(candidate) {
			return candidate, nil
		}
	}

	// 5. ~/.config/tracelab/tracelab.toml
	if home, err := r.homeDir(); err == nil && home != "" {
		candidate := filepath.Join(home, ".config", "tracelab", "tracelab.toml")
		if r.exists(candidate) {
			return candidate, nil
		}
	}

	return "", nil
}

// ErrNoURL is returned by Resolve when neither flag, env, nor config
// file provided a hub URL. The user must point the CLI at something.
var ErrNoURL = errors.New("cliconfig: hub URL is required (set [server].port + [server].bind in tracelab.toml, or pass --url, or set TRACELAB_URL)")

// ErrNoToken is returned by Resolve when no bearer token is configured.
var ErrNoToken = errors.New("cliconfig: hub token is required (set [auth].token in tracelab.toml, or pass --token, or set TRACELAB_TOKEN)")

// Resolve performs the full discovery + override pipeline.
//
// Precedence (highest first):
//
//	URL:    src.FlagURL    >  $TRACELAB_URL    >  http://<server.bind>:<server.port>
//	Token:  src.FlagToken  >  $TRACELAB_TOKEN  >  [auth].token
//
// The returned Resolved is fully populated; callers can pass it straight
// into client.New.
func Resolve(src Sources) (*Resolved, error) {
	return defaultResolver().resolve(src)
}

func (r *resolver) resolve(src Sources) (*Resolved, error) {
	out := &Resolved{}

	// 1. Discover + load config file (if any).
	path, err := r.discover(src.FlagConfigPath)
	if err != nil {
		return nil, err
	}
	if path != "" {
		cfg, lerr := config.Load(path)
		if lerr != nil {
			return nil, fmt.Errorf("cliconfig: %w", lerr)
		}
		out.ConfigPath = path
		out.CLI = cfg.CLI
		// Derive URL from [server] section if non-zero. Bind "0.0.0.0"
		// is a listener address; for outbound connect we prefer
		// 127.0.0.1 to avoid issuing requests to the wildcard.
		if cfg.Server.Port != 0 {
			host := cfg.Server.Bind
			if host == "" || host == "0.0.0.0" || host == "::" {
				host = "127.0.0.1"
			}
			out.BaseURL = fmt.Sprintf("http://%s:%d", host, cfg.Server.Port)
		}
		out.Token = cfg.Auth.Token
	}
	out.CLI.ApplyDefaults()

	// 2. Env overrides (only when set non-empty).
	if v, ok := r.getenv(EnvURL); ok && v != "" {
		out.BaseURL = v
	}
	if v, ok := r.getenv(EnvToken); ok && v != "" {
		out.Token = v
	}

	// 3. Flag overrides (highest precedence).
	if src.FlagURL != "" {
		out.BaseURL = src.FlagURL
	}
	if src.FlagToken != "" {
		out.Token = src.FlagToken
	}

	// 4. Validation.
	if out.BaseURL == "" {
		return nil, ErrNoURL
	}
	if out.Token == "" {
		return nil, ErrNoToken
	}
	return out, nil
}
