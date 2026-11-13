// Package config loads runtime configuration from environment variables.
//
// For local development, load a .env file before starting:
//
//	godotenv -f .env go run ./cmd/emailvetter
package config

import (
	"fmt"
	"net/url"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config holds all runtime settings.
type Config struct {
	// Env is the deployment environment; "production" enables strict checks.
	Env string `env:"APP_ENV" envDefault:"development"`

	// Addr is the HTTP listen address.
	Addr string `env:"EMAILVETTER_ADDR" envDefault:":8080"`

	// RQLiteAddr is the rqlite HTTP endpoint (any node; writes are forwarded
	// to the leader).
	RQLiteAddr string `env:"EMAILVETTER_RQLITE_ADDR" envDefault:"http://127.0.0.1:4001"`

	// RQLiteUser and RQLitePass enable HTTP Basic Auth for rqlite when both
	// are set.
	RQLiteUser string `env:"EMAILVETTER_RQLITE_USER" envDefault:""`
	RQLitePass string `env:"EMAILVETTER_RQLITE_PASS" envDefault:""`

	// RQLiteTimeout is the per-request timeout for rqlite calls.
	RQLiteTimeout time.Duration `env:"EMAILVETTER_RQLITE_TIMEOUT" envDefault:"5s"`

	// ReadKeys are API keys (comma-separated) granted the vet:read scope.
	ReadKeys []string `env:"EMAILVETTER_READ_KEYS" envDefault:""`

	// WriteKeys are API keys (comma-separated) granted domains:write, which
	// also grants read.
	WriteKeys []string `env:"EMAILVETTER_WRITE_KEYS" envDefault:""`
}

// Load reads the configuration from the process environment.
func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// Validate checks the settings that must hold before the server starts.
func (c Config) Validate() error {
	u, err := url.Parse(c.RQLiteAddr)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("EMAILVETTER_RQLITE_ADDR must be an http(s) URL, got %q", c.RQLiteAddr)
	}
	if c.RQLiteTimeout <= 0 {
		return fmt.Errorf("EMAILVETTER_RQLITE_TIMEOUT must be positive")
	}
	if c.Env == "production" && !c.AuthEnabled() {
		return fmt.Errorf("EMAILVETTER_READ_KEYS or EMAILVETTER_WRITE_KEYS is required in production")
	}
	return nil
}

// AuthEnabled reports whether at least one API key is configured.
func (c Config) AuthEnabled() bool {
	return len(c.ReadKeys)+len(c.WriteKeys) > 0
}
