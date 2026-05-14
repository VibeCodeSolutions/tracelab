package cliconfig

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkConfig writes a minimal tracelab.toml under dir and returns the path.
// The file carries a server section + auth token so Resolve can derive
// a URL and Token from it.
func mkConfig(t *testing.T, dir, token string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, "tracelab.toml")
	content := `
[server]
port = 8765
bind = "127.0.0.1"

[auth]
token = "` + token + `"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// hermeticResolver builds a resolver with stubbed env/fs/home/cwd so a
// test never touches the user's real $HOME or shell env. Files are still
// real (in t.TempDir()), so fsExists is the live os.Stat.
func hermeticResolver(env map[string]string, home, cwd string) *resolver {
	return &resolver{
		getenv: func(k string) (string, bool) {
			v, ok := env[k]
			return v, ok
		},
		exists: func(p string) bool {
			_, err := os.Stat(p)
			return err == nil
		},
		homeDir: func() (string, error) { return home, nil },
		getwd:   func() (string, error) { return cwd, nil },
	}
}

// ---- Discover: each of the 5 layers ----

func TestDiscover_Layer1_ExplicitFlag(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := mkConfig(t, dir, "tok")
	r := hermeticResolver(map[string]string{}, "", "")
	got, err := r.discover(path)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if got != path {
		t.Errorf("got %q, want %q", got, path)
	}
}

func TestDiscover_Layer1_FlagMissingFile_Errors(t *testing.T) {
	t.Parallel()
	r := hermeticResolver(map[string]string{}, "", "")
	_, err := r.discover("/no/such/file.toml")
	if err == nil {
		t.Fatal("expected error for missing --config target")
	}
	if !strings.Contains(err.Error(), "--config") {
		t.Errorf("err = %v, want mention of --config", err)
	}
}

func TestDiscover_Layer2_EnvVar(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := mkConfig(t, dir, "tok")
	r := hermeticResolver(map[string]string{EnvConfig: path}, "", "")
	got, err := r.discover("")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if got != path {
		t.Errorf("got %q, want %q", got, path)
	}
}

func TestDiscover_Layer2_EnvMissingFile_Errors(t *testing.T) {
	t.Parallel()
	r := hermeticResolver(map[string]string{EnvConfig: "/no/such/file.toml"}, "", "")
	_, err := r.discover("")
	if err == nil {
		t.Fatal("expected error for missing TRACELAB_CONFIG target")
	}
	if !strings.Contains(err.Error(), EnvConfig) {
		t.Errorf("err = %v, want mention of %s", err, EnvConfig)
	}
}

func TestDiscover_Layer3_Cwd(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	want := mkConfig(t, cwd, "tok")
	r := hermeticResolver(map[string]string{}, "", cwd)
	got, err := r.discover("")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDiscover_Layer4_XDG(t *testing.T) {
	t.Parallel()
	xdg := t.TempDir()
	want := mkConfig(t, filepath.Join(xdg, "tracelab"), "tok")
	// cwd is set to a separate empty tmpdir so layer 3 cannot match.
	cwd := t.TempDir()
	r := hermeticResolver(map[string]string{"XDG_CONFIG_HOME": xdg}, "", cwd)
	got, err := r.discover("")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDiscover_Layer5_HomeDotConfig(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	want := mkConfig(t, filepath.Join(home, ".config", "tracelab"), "tok")
	cwd := t.TempDir() // empty
	r := hermeticResolver(map[string]string{}, home, cwd)
	got, err := r.discover("")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDiscover_NoFileFound_ReturnsEmptyNoError(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	home := t.TempDir()
	r := hermeticResolver(map[string]string{}, home, cwd)
	got, err := r.discover("")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty path, got %q", got)
	}
}

// ---- Discover: precedence between layers ----

func TestDiscover_FlagBeatsEnv(t *testing.T) {
	t.Parallel()
	flagDir := t.TempDir()
	envDir := t.TempDir()
	flagPath := mkConfig(t, flagDir, "flag")
	envPath := mkConfig(t, envDir, "env")
	r := hermeticResolver(map[string]string{EnvConfig: envPath}, "", "")
	got, err := r.discover(flagPath)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if got != flagPath {
		t.Errorf("flag must beat env: got %q", got)
	}
}

func TestDiscover_EnvBeatsCwd(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	envDir := t.TempDir()
	_ = mkConfig(t, cwd, "cwd")
	envPath := mkConfig(t, envDir, "env")
	r := hermeticResolver(map[string]string{EnvConfig: envPath}, "", cwd)
	got, err := r.discover("")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if got != envPath {
		t.Errorf("env must beat cwd: got %q", got)
	}
}

func TestDiscover_CwdBeatsXDG(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	xdg := t.TempDir()
	want := mkConfig(t, cwd, "cwd")
	_ = mkConfig(t, filepath.Join(xdg, "tracelab"), "xdg")
	r := hermeticResolver(map[string]string{"XDG_CONFIG_HOME": xdg}, "", cwd)
	got, err := r.discover("")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if got != want {
		t.Errorf("cwd must beat XDG: got %q", got)
	}
}

func TestDiscover_XDGBeatsHomeDotConfig(t *testing.T) {
	t.Parallel()
	xdg := t.TempDir()
	home := t.TempDir()
	want := mkConfig(t, filepath.Join(xdg, "tracelab"), "xdg")
	_ = mkConfig(t, filepath.Join(home, ".config", "tracelab"), "home")
	cwd := t.TempDir() // empty
	r := hermeticResolver(map[string]string{"XDG_CONFIG_HOME": xdg}, home, cwd)
	got, err := r.discover("")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if got != want {
		t.Errorf("XDG must beat $HOME/.config: got %q", got)
	}
}

// ---- Resolve: override precedence flag > env > config ----

func TestResolve_ConfigOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := mkConfig(t, dir, "from-config")
	r := hermeticResolver(map[string]string{}, "", "")
	got, err := r.resolve(Sources{FlagConfigPath: path})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.BaseURL != "http://127.0.0.1:8765" {
		t.Errorf("BaseURL = %q", got.BaseURL)
	}
	if got.Token != "from-config" {
		t.Errorf("Token = %q", got.Token)
	}
	if got.ConfigPath != path {
		t.Errorf("ConfigPath = %q", got.ConfigPath)
	}
	// CLI defaults must be filled even when [cli] is absent.
	if got.CLI.DefaultFormat == "" {
		t.Errorf("CLI.DefaultFormat must default")
	}
}

func TestResolve_EnvBeatsConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := mkConfig(t, dir, "from-config")
	r := hermeticResolver(map[string]string{
		EnvURL:   "http://env.example:9000",
		EnvToken: "from-env",
	}, "", "")
	got, err := r.resolve(Sources{FlagConfigPath: path})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.BaseURL != "http://env.example:9000" {
		t.Errorf("BaseURL = %q", got.BaseURL)
	}
	if got.Token != "from-env" {
		t.Errorf("Token = %q", got.Token)
	}
}

func TestResolve_FlagBeatsEnv(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := mkConfig(t, dir, "from-config")
	r := hermeticResolver(map[string]string{
		EnvURL:   "http://env.example:9000",
		EnvToken: "from-env",
	}, "", "")
	got, err := r.resolve(Sources{
		FlagConfigPath: path,
		FlagURL:        "http://flag.example:1234",
		FlagToken:      "from-flag",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.BaseURL != "http://flag.example:1234" {
		t.Errorf("BaseURL = %q", got.BaseURL)
	}
	if got.Token != "from-flag" {
		t.Errorf("Token = %q", got.Token)
	}
}

func TestResolve_NoConfig_FlagOnly(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir() // empty — no tracelab.toml here
	home := t.TempDir()
	r := hermeticResolver(map[string]string{}, home, cwd)
	got, err := r.resolve(Sources{
		FlagURL:   "http://only-flag:7777",
		FlagToken: "only-flag-token",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.BaseURL != "http://only-flag:7777" || got.Token != "only-flag-token" {
		t.Errorf("flag-only resolve mismatch: %+v", got)
	}
	if got.ConfigPath != "" {
		t.Errorf("ConfigPath must be empty when no file found, got %q", got.ConfigPath)
	}
}

func TestResolve_NoURL_Errors(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	home := t.TempDir()
	r := hermeticResolver(map[string]string{}, home, cwd)
	_, err := r.resolve(Sources{FlagToken: "tok"})
	if !errors.Is(err, ErrNoURL) {
		t.Errorf("expected ErrNoURL, got %v", err)
	}
}

func TestResolve_NoToken_Errors(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	home := t.TempDir()
	r := hermeticResolver(map[string]string{}, home, cwd)
	_, err := r.resolve(Sources{FlagURL: "http://x:1"})
	if !errors.Is(err, ErrNoToken) {
		t.Errorf("expected ErrNoToken, got %v", err)
	}
}

func TestResolve_BindWildcard_RewritesToLoopback(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "tracelab.toml")
	if err := os.WriteFile(path, []byte(`
[server]
port = 9999
bind = "0.0.0.0"
[auth]
token = "tok"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	r := hermeticResolver(map[string]string{}, "", "")
	got, err := r.resolve(Sources{FlagConfigPath: path})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.BaseURL != "http://127.0.0.1:9999" {
		t.Errorf("wildcard bind must be rewritten to loopback, got %q", got.BaseURL)
	}
}

func TestResolve_CLISectionParsed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "tracelab.toml")
	if err := os.WriteFile(path, []byte(`
[server]
port = 8765
[auth]
token = "tok"
[cli]
default_format = "json"
tail_buffer = 4096
`), 0o600); err != nil {
		t.Fatal(err)
	}
	r := hermeticResolver(map[string]string{}, "", "")
	got, err := r.resolve(Sources{FlagConfigPath: path})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.CLI.DefaultFormat != "json" {
		t.Errorf("DefaultFormat = %q, want json", got.CLI.DefaultFormat)
	}
	if got.CLI.TailBuffer != 4096 {
		t.Errorf("TailBuffer = %d, want 4096", got.CLI.TailBuffer)
	}
	// Color was not set in the TOML — ApplyDefaults must fill it.
	if got.CLI.Color != "auto" {
		t.Errorf("Color = %q, want default auto", got.CLI.Color)
	}
}
