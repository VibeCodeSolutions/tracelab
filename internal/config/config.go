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
type Config struct {
	Server  ServerConfig  `toml:"server"`
	Storage StorageConfig `toml:"storage"`
	Auth    AuthConfig    `toml:"auth"`
	ADB     ADBConfig     `toml:"adb"`
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
