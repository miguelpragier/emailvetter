package store

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// integrationStore returns a store pointed at a real rqlite cluster, or skips
// the test when EMAILVETTER_RQLITE_ADDR is not set.
func integrationStore(t *testing.T) Store {
	t.Helper()
	addr := os.Getenv("EMAILVETTER_RQLITE_ADDR")
	if addr == "" {
		t.Skip("EMAILVETTER_RQLITE_ADDR not set; run against a real rqlite")
	}
	return New(addr, "", "", 5*time.Second)
}

func TestIntegrationEnsureSchemaAndCRUD(t *testing.T) {
	st := integrationStore(t)
	ctx := t.Context()

	require.NoError(t, st.EnsureSchema(ctx))

	created, err := st.Create(ctx, "integration.test", "note", "test")
	require.NoError(t, err)
	require.Equal(t, "integration.test", created.Domain)

	got, err := st.Get(ctx, "integration.test")
	require.NoError(t, err)
	require.Equal(t, "note", got.Note)

	// Subdomain matching against a real SQLite IN clause.
	matched, blocked, err := st.Match(ctx, []string{"sub.integration.test", "integration.test"})
	require.NoError(t, err)
	require.True(t, blocked)
	require.Equal(t, "integration.test", matched)

	_, err = st.Create(ctx, "integration.test", "", "")
	require.ErrorIs(t, err, ErrExists)

	require.NoError(t, st.Delete(ctx, "integration.test"))
	_, err = st.Get(ctx, "integration.test")
	require.ErrorIs(t, err, ErrNotFound)
}
