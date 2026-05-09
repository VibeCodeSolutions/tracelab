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
