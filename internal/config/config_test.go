package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	t.Setenv("EMAILVETTER_RQLITE_ADDR", "http://example.test:4001")
	t.Setenv("EMAILVETTER_ADDR", ":9999")
	t.Setenv("EMAILVETTER_READ_KEYS", "")
	t.Setenv("EMAILVETTER_WRITE_KEYS", "")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "http://example.test:4001", cfg.RQLiteAddr)
	require.Equal(t, ":9999", cfg.Addr)
	require.False(t, cfg.AuthEnabled())
}

func TestValidate(t *testing.T) {
	base := Config{RQLiteAddr: "http://127.0.0.1:4001", RQLiteTimeout: 5 * time.Second}
	require.NoError(t, base.Validate())

	bad := base
	bad.RQLiteAddr = "not-a-url"
	require.Error(t, bad.Validate())

	bad = base
	bad.RQLiteAddr = "ftp://host"
	require.Error(t, bad.Validate())

	bad = base
	bad.RQLiteTimeout = 0
	require.Error(t, bad.Validate())

	// Production without any API key is rejected.
	prod := base
	prod.Env = "production"
	require.Error(t, prod.Validate())

	prod.WriteKeys = []string{"k"}
	require.NoError(t, prod.Validate())
}

func TestAuthEnabled(t *testing.T) {
	require.False(t, Config{}.AuthEnabled())
	require.True(t, Config{ReadKeys: []string{"a"}}.AuthEnabled())
	require.True(t, Config{WriteKeys: []string{"b"}}.AuthEnabled())
}
