// Package config loads tracelab.toml into a typed Config struct.
//
// Defaults are filled in for any missing field so callers can always use the
// returned struct directly. Path is resolved relative to the current working
// directory; absolute paths are kept as-is.
package config

import (
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is the top-level tracelab configuration.
//
// Both the hub daemon and the `tracelab` CLI read the same TOML file (see
// ADR-002, docs/ARCH.md). The hub ignores the [cli] section; the CLI
// ignores [server]/[storage]/[adb]. [auth] is shared so token rotation
// happens in one place.
type Config struct {
	Server  ServerConfig  `toml:"server"`
	Storage StorageConfig `toml:"storage"`
	Auth    AuthConfig    `toml:"auth"`
	ADB     ADBConfig     `toml:"adb"`
	CLI     CLIConfig     `toml:"cli"`
}

// ServerConfig controls the HTTP listener.
type ServerConfig struct {
	Port          int           `toml:"port"`
	Bind          string        `toml:"bind"`
	ReadTimeout   time.Duration `toml:"read_timeout"`
	WriteTimeout  time.Duration `toml:"write_timeout"`
}

// StorageConfig points at the on-disk datastore location.
type StorageConfig struct {
	DatastorePath string `toml:"datastore_path"`
}

// AuthConfig holds the shared bearer token.
type AuthConfig struct {
	Token string `toml:"token"`
}

// ADBConfig configures the optional adb-logcat bridge.
//
// When Enabled is true the hub spawns a background bridge goroutine that
// runs `adb logcat -v threadtime` against DeviceSerial (or the only
// attached device when DeviceSerial is empty), maps each line into a
// store.Event with source="adb" and persists+broadcasts it. The bridge
// reconnects with exponential backoff on subprocess exit.
type ADBConfig struct {
	// Enabled toggles the bridge on/off. Default false: tracelab-hub
	// works without any adb installed when this is left disabled.
	Enabled bool `toml:"enabled"`
	// DeviceSerial pins the bridge to a specific adb device (e.g.
	// "emulator-5554", "192.168.1.42:5555"). Empty means "let adb
	// pick the only attached device" — adb errors on >1 device when
	// no -s is given, which the bridge surfaces as a reconnect.
	DeviceSerial string `toml:"device_serial"`
	// TagFilter restricts logcat to one tag (passed as `<tag>:V *:S`
	// to the adb subprocess). Empty means stream every tag.
	TagFilter string `toml:"tag_filter"`
}

// CLIConfig holds knobs consumed by the `tracelab` CLI only. The hub
// daemon parses but ignores this section — single source of truth per
// ADR-002.
//
// Defaults (applied when a key is missing or zero-valued):
//
//	default_format = "table"  # table | json
//	color          = "auto"   # auto | always | never
//	tail_buffer    = 1024
//
// The `tail_buffer` key is reserved for the S4 `tail` sub-command and is
// not consumed by the S3 `sessions` path — it is parsed here only so a
// shared tracelab.toml carrying a populated [cli] block does not fail
// the hub's strict-config path.
type CLIConfig struct {
	// DefaultFormat is the default output renderer when --format is not
	// passed. Recognised values are "table" and "json". Empty string is
	// treated as "table".
	DefaultFormat string `toml:"default_format"`
	// Color controls ANSI-colour output. Recognised values: "auto",
	// "always", "never". Empty string is treated as "auto". Not used by
	// the S3 `sessions` path (no level-coloured output yet) but parsed
	// here so an [cli] block with a populated colour key does not error.
	Color string `toml:"color"`
	// TailBuffer is the per-subscriber buffered-channel size used by the
	// S4 `tail` sub-command. Zero is treated as 1024. Parsed here for
	// completeness; not consumed by the S3 path.
	TailBuffer int `toml:"tail_buffer"`
}

// Defaults for CLIConfig — exported so callers (e.g. cliconfig.Resolve)
// can apply them consistently without duplicating the literals.
const (
	DefaultCLIFormat     = "table"
	DefaultCLIColor      = "auto"
	DefaultCLITailBuffer = 1024
)

// ApplyDefaults fills zero-valued CLIConfig fields with their defaults.
// Safe to call multiple times; idempotent.
func (c *CLIConfig) ApplyDefaults() {
	if c.DefaultFormat == "" {
		c.DefaultFormat = DefaultCLIFormat
	}
	if c.Color == "" {
		c.Color = DefaultCLIColor
	}
	if c.TailBuffer == 0 {
		c.TailBuffer = DefaultCLITailBuffer
	}
}

// Load reads the TOML config at path and applies defaults.
func Load(path string) (*Config, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	var c Config
	if err := toml.Unmarshal(buf, &c); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	c.applyDefaults()
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Port == 0 {
		c.Server.Port = 8765
	}
	if c.Server.Bind == "" {
		c.Server.Bind = "127.0.0.1"
	}
	if c.Server.ReadTimeout == 0 {
		c.Server.ReadTimeout = 15 * time.Second
	}
	if c.Server.WriteTimeout == 0 {
		c.Server.WriteTimeout = 15 * time.Second
	}
}
