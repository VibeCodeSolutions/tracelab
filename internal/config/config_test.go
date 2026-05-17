package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTOML is a small helper for table tests: writes content to a fresh
// tmp file under t.TempDir() and returns the path.
func writeTOML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tracelab.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write tmp toml: %v", err)
	}
	return path
}

func TestLoad_ParsesCLISection(t *testing.T) {
	t.Parallel()
	path := writeTOML(t, `
[auth]
token = "abc"

[cli]
default_format = "json"
color          = "never"
tail_buffer    = 2048
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CLI.DefaultFormat != "json" {
		t.Errorf("DefaultFormat = %q, want %q", cfg.CLI.DefaultFormat, "json")
	}
	if cfg.CLI.Color != "never" {
		t.Errorf("Color = %q, want %q", cfg.CLI.Color, "never")
	}
	if cfg.CLI.TailBuffer != 2048 {
		t.Errorf("TailBuffer = %d, want %d", cfg.CLI.TailBuffer, 2048)
	}
}

func TestLoad_CLISectionMissing_DoesNotError(t *testing.T) {
	t.Parallel()
	// A hub-only TOML must still load — the [cli] block is optional and
	// hub deployments may not carry it.
	path := writeTOML(t, `
[server]
port = 8765

[auth]
token = "abc"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Zero values: parser did not invent anything.
	if cfg.CLI.DefaultFormat != "" || cfg.CLI.Color != "" || cfg.CLI.TailBuffer != 0 {
		t.Errorf("CLI zero-value expected, got %+v", cfg.CLI)
	}
}

func TestCLIConfig_ApplyDefaults_FillsZeroFields(t *testing.T) {
	t.Parallel()
	var cli CLIConfig
	cli.ApplyDefaults()
	if cli.DefaultFormat != DefaultCLIFormat {
		t.Errorf("DefaultFormat = %q, want %q", cli.DefaultFormat, DefaultCLIFormat)
	}
	if cli.Color != DefaultCLIColor {
		t.Errorf("Color = %q, want %q", cli.Color, DefaultCLIColor)
	}
	if cli.TailBuffer != DefaultCLITailBuffer {
		t.Errorf("TailBuffer = %d, want %d", cli.TailBuffer, DefaultCLITailBuffer)
	}
}

func TestCLIConfig_ApplyDefaults_KeepsExplicitValues(t *testing.T) {
	t.Parallel()
	cli := CLIConfig{DefaultFormat: "json", Color: "always", TailBuffer: 512}
	cli.ApplyDefaults()
	if cli.DefaultFormat != "json" || cli.Color != "always" || cli.TailBuffer != 512 {
		t.Errorf("ApplyDefaults must not overwrite explicit values, got %+v", cli)
	}
}

func TestCLIConfig_ApplyDefaults_Idempotent(t *testing.T) {
	t.Parallel()
	var cli CLIConfig
	cli.ApplyDefaults()
	first := cli
	cli.ApplyDefaults()
	if cli != first {
		t.Errorf("ApplyDefaults not idempotent: first=%+v second=%+v", first, cli)
	}
}

func TestLoad_ParsesAgentsTranscriptSection(t *testing.T) {
	t.Parallel()
	path := writeTOML(t, `
[auth]
token = "abc"

[agents.transcript]
enabled = true
projects_root = "/tmp/test-projects"
poll_interval_ms = 250
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Agents.Transcript.Enabled {
		t.Errorf("Transcript.Enabled = false, want true")
	}
	if cfg.Agents.Transcript.ProjectsRoot != "/tmp/test-projects" {
		t.Errorf("ProjectsRoot = %q", cfg.Agents.Transcript.ProjectsRoot)
	}
	if cfg.Agents.Transcript.PollIntervalMs != 250 {
		t.Errorf("PollIntervalMs = %d, want 250", cfg.Agents.Transcript.PollIntervalMs)
	}
}

func TestLoad_AgentsSectionMissing_AppliesDefaults(t *testing.T) {
	t.Parallel()
	// A hub TOML without any [agents.*] block must still load, with
	// the transcript bridge disabled but defaults applied.
	path := writeTOML(t, `
[auth]
token = "abc"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Agents.Transcript.Enabled {
		t.Errorf("Transcript.Enabled = true, want false (off by default)")
	}
	if cfg.Agents.Transcript.ProjectsRoot != DefaultTranscriptProjectsRoot {
		t.Errorf("ProjectsRoot default not applied, got %q", cfg.Agents.Transcript.ProjectsRoot)
	}
	if cfg.Agents.Transcript.PollIntervalMs != DefaultTranscriptPollIntervalMs {
		t.Errorf("PollIntervalMs default not applied, got %d", cfg.Agents.Transcript.PollIntervalMs)
	}
}

func TestTranscriptConfig_applyDefaults_KeepsExplicitValues(t *testing.T) {
	t.Parallel()
	tc := TranscriptConfig{Enabled: true, ProjectsRoot: "/x", PollIntervalMs: 50}
	tc.applyDefaults()
	if tc.ProjectsRoot != "/x" || tc.PollIntervalMs != 50 || !tc.Enabled {
		t.Errorf("applyDefaults must not overwrite explicit values, got %+v", tc)
	}
}
