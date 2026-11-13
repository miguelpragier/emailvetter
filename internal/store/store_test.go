package store

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeRqlite emulates the subset of rqlite's HTTP API that the store uses,
// backed by an in-memory map. It lets store tests run without a live cluster;
// integration_test.go exercises the real thing when a cluster is available.
type fakeRqlite struct {
	mu      sync.Mutex
	domains map[string]fakeDomain
	meta    map[string]string
}

type fakeDomain struct {
	note, addedBy, addedAt string
}

func newFakeRqlite(t *testing.T) (*fakeRqlite, *httptest.Server) {
	t.Helper()
	f := &fakeRqlite{domains: map[string]fakeDomain{}, meta: map[string]string{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeRqlite) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var statements []any
	if err := json.NewDecoder(r.Body).Decode(&statements); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	results := make([]map[string]any, 0, len(statements))
	for _, st := range statements {
		results = append(results, f.run(st))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
}

func (f *fakeRqlite) run(st any) map[string]any {
	parts, ok := st.([]any)
	if !ok || len(parts) == 0 {
		return map[string]any{"error_v2": map[string]any{"message": "empty statement"}}
	}
	sql, _ := parts[0].(string)
	args := parts[1:]

	switch {
	case strings.Contains(sql, "CREATE TABLE"):
		return map[string]any{"rows_affected": 0}
	case strings.Contains(sql, "SELECT 1 AS ok"):
		return rowsResult([]map[string]any{{"ok": float64(1)}})
	case strings.Contains(sql, "SELECT value FROM app_meta"):
		k := strArg(args, 0)
		var rows []map[string]any
		if v, ok := f.meta[k]; ok {
			rows = append(rows, map[string]any{"value": v})
		}
		return rowsResult(rows)
	case strings.Contains(sql, "count(*) AS n"):
		return rowsResult([]map[string]any{{"n": float64(f.count(strArg(args, 0)))}})
	case strings.Contains(sql, "SELECT domain FROM blocked_domains WHERE domain IN"):
		return f.match(args)
	case strings.Contains(sql, "SELECT domain, note") && strings.Contains(sql, "ORDER BY"):
		return f.list(args)
	case strings.Contains(sql, "SELECT domain, note"):
		return f.get(args)
	case strings.Contains(sql, "INSERT OR IGNORE INTO blocked_domains"):
		return f.insert(args)
	case strings.Contains(sql, "INSERT OR IGNORE INTO app_meta"):
		f.meta[strArg(args, 0)] = strArg(args, 1)
		return map[string]any{"rows_affected": 1}
	case strings.Contains(sql, "UPDATE blocked_domains"):
		return f.update(args)
	case strings.Contains(sql, "DELETE FROM blocked_domains"):
		return f.delete(args)
	default:
		return map[string]any{"error_v2": map[string]any{"message": "unsupported statement: " + sql}}
	}
}

func rowsResult(rows []map[string]any) map[string]any { return map[string]any{"rows": rows} }

func strArg(args []any, i int) string {
	if i >= len(args) {
		return ""
	}
	s, _ := args[i].(string)
	return s
}

func intArg(args []any, i int) int {
	if i >= len(args) {
		return 0
	}
	switch n := args[i].(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

func (f *fakeRqlite) count(filter string) int {
	n := 0
	for d := range f.domains {
		if filter == "" || strings.Contains(d, filter) {
			n++
		}
	}
	return n
}

func (f *fakeRqlite) match(args []any) map[string]any {
	var rows []map[string]any
	for _, c := range args {
		d, _ := c.(string)
		if _, ok := f.domains[d]; ok {
			rows = append(rows, map[string]any{"domain": d})
		}
	}
	return rowsResult(rows)
}

func (f *fakeRqlite) list(args []any) map[string]any {
	filter := strArg(args, 0)
	limit := intArg(args, 2)
	offset := intArg(args, 3)
	keys := make([]string, 0, len(f.domains))
	for d := range f.domains {
		if filter == "" || strings.Contains(d, filter) {
			keys = append(keys, d)
		}
	}
	sort.Strings(keys)
	if offset >= len(keys) {
		return rowsResult([]map[string]any{})
	}
	end := offset + limit
	if end > len(keys) {
		end = len(keys)
	}
	rows := make([]map[string]any, 0, end-offset)
	for _, d := range keys[offset:end] {
		fd := f.domains[d]
		rows = append(rows, map[string]any{"domain": d, "note": fd.note, "added_by": fd.addedBy, "added_at": fd.addedAt})
	}
	return rowsResult(rows)
}

func (f *fakeRqlite) get(args []any) map[string]any {
	d := strArg(args, 0)
	fd, ok := f.domains[d]
	if !ok {
		return rowsResult([]map[string]any{})
	}
	return rowsResult([]map[string]any{{"domain": d, "note": fd.note, "added_by": fd.addedBy, "added_at": fd.addedAt}})
}

func (f *fakeRqlite) insert(args []any) map[string]any {
	d := strArg(args, 0)
	if _, ok := f.domains[d]; ok {
		return map[string]any{"rows_affected": 0}
	}
	f.domains[d] = fakeDomain{note: strArg(args, 1), addedBy: strArg(args, 2), addedAt: strArg(args, 3)}
	return map[string]any{"rows_affected": 1}
}

func (f *fakeRqlite) update(args []any) map[string]any {
	note := strArg(args, 0)
	d := strArg(args, 1)
	fd, ok := f.domains[d]
	if !ok {
		return map[string]any{"rows_affected": 0}
	}
	fd.note = note
	f.domains[d] = fd
	return map[string]any{"rows_affected": 1}
}

func (f *fakeRqlite) delete(args []any) map[string]any {
	d := strArg(args, 0)
	if _, ok := f.domains[d]; !ok {
		return map[string]any{"rows_affected": 0}
	}
	delete(f.domains, d)
	return map[string]any{"rows_affected": 1}
}

func newTestStore(t *testing.T) (Store, *fakeRqlite) {
	t.Helper()
	f, srv := newFakeRqlite(t)
	return New(srv.URL, "", "", time.Second), f
}

func TestCreateGetDelete(t *testing.T) {
	st, _ := newTestStore(t)
	ctx := t.Context()

	created, err := st.Create(ctx, "example.com", "test", "ops")
	require.NoError(t, err)
	require.Equal(t, "example.com", created.Domain)
	require.Equal(t, "test", created.Note)

	got, err := st.Get(ctx, "example.com")
	require.NoError(t, err)
	require.Equal(t, "test", got.Note)

	_, err = st.Create(ctx, "example.com", "", "")
	require.ErrorIs(t, err, ErrExists)

	require.NoError(t, st.Delete(ctx, "example.com"))
	_, err = st.Get(ctx, "example.com")
	require.ErrorIs(t, err, ErrNotFound)
	require.ErrorIs(t, st.Delete(ctx, "example.com"), ErrNotFound)
}

func TestUpdateNote(t *testing.T) {
	st, _ := newTestStore(t)
	ctx := t.Context()

	_, err := st.Create(ctx, "example.com", "old", "")
	require.NoError(t, err)

	updated, err := st.UpdateNote(ctx, "example.com", "new")
	require.NoError(t, err)
	require.Equal(t, "new", updated.Note)

	_, err = st.UpdateNote(ctx, "missing.com", "x")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestList(t *testing.T) {
	st, _ := newTestStore(t)
	ctx := t.Context()
	for _, d := range []string{"beta.com", "alpha.com", "gamma.com"} {
		_, err := st.Create(ctx, d, "", "")
		require.NoError(t, err)
	}

	items, total, err := st.List(ctx, "", 2, 0)
	require.NoError(t, err)
	require.Equal(t, 3, total)
	require.Len(t, items, 2)
	require.Equal(t, "alpha.com", items[0].Domain)
	require.Equal(t, "beta.com", items[1].Domain)

	items, total, err = st.List(ctx, "alp", 10, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, items, 1)
	require.Equal(t, "alpha.com", items[0].Domain)
}

func TestMatch(t *testing.T) {
	st, _ := newTestStore(t)
	ctx := t.Context()
	_, err := st.Create(ctx, "example.com", "", "")
	require.NoError(t, err)

	matched, blocked, err := st.Match(ctx, []string{"mx.example.com", "example.com"})
	require.NoError(t, err)
	require.True(t, blocked)
	require.Equal(t, "example.com", matched)

	_, blocked, err = st.Match(ctx, []string{"other.com"})
	require.NoError(t, err)
	require.False(t, blocked)
}

func TestBulkCreate(t *testing.T) {
	st, _ := newTestStore(t)
	ctx := t.Context()
	_, err := st.Create(ctx, "existing.com", "", "")
	require.NoError(t, err)

	added, existed, err := st.BulkCreate(ctx, []BulkItem{
		{Domain: "new1.com"},
		{Domain: "existing.com"},
		{Domain: "new2.com"},
	})
	require.NoError(t, err)
	require.Equal(t, 2, added)
	require.Equal(t, []string{"existing.com"}, existed)
}

func TestEnsureSchemaSeedsOnce(t *testing.T) {
	st, f := newTestStore(t)
	ctx := t.Context()

	require.NoError(t, st.EnsureSchema(ctx))
	require.Len(t, f.domains, 814)
	require.Equal(t, "1", f.meta[seedKey])

	// A deleted domain must not come back on a later bootstrap.
	delete(f.domains, "mailinator.com")
	require.NoError(t, st.EnsureSchema(ctx))
	_, ok := f.domains["mailinator.com"]
	require.False(t, ok)
}
