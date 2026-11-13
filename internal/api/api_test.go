package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miguelpragier/emailvetter/internal/api"
	"github.com/miguelpragier/emailvetter/internal/config"
	"github.com/miguelpragier/emailvetter/internal/store"
)

const (
	readKey  = "read-key"
	writeKey = "write-key"
)

// fakeStore is an in-memory Store for exercising the API layer without a DB.
type fakeStore struct {
	mu      sync.Mutex
	domains map[string]store.Domain
}

func newFakeStore() *fakeStore { return &fakeStore{domains: map[string]store.Domain{}} }

func (f *fakeStore) Ping(context.Context) error         { return nil }
func (f *fakeStore) EnsureSchema(context.Context) error { return nil }

func (f *fakeStore) List(_ context.Context, filter string, limit, offset int) ([]store.Domain, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	keys := make([]string, 0, len(f.domains))
	for d := range f.domains {
		if filter == "" || strings.Contains(d, filter) {
			keys = append(keys, d)
		}
	}
	sort.Strings(keys)
	total := len(keys)
	if offset >= len(keys) {
		return []store.Domain{}, total, nil
	}
	end := offset + limit
	if end > len(keys) {
		end = len(keys)
	}
	out := make([]store.Domain, 0, end-offset)
	for _, d := range keys[offset:end] {
		out = append(out, f.domains[d])
	}
	return out, total, nil
}

func (f *fakeStore) Get(_ context.Context, domain string) (store.Domain, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.domains[domain]
	if !ok {
		return store.Domain{}, store.ErrNotFound
	}
	return d, nil
}

func (f *fakeStore) Create(_ context.Context, domain, note, addedBy string) (store.Domain, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.domains[domain]; ok {
		return store.Domain{}, store.ErrExists
	}
	d := store.Domain{Domain: domain, Note: note, AddedBy: addedBy, AddedAt: time.Now()}
	f.domains[domain] = d
	return d, nil
}

func (f *fakeStore) UpdateNote(_ context.Context, domain, note string) (store.Domain, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.domains[domain]
	if !ok {
		return store.Domain{}, store.ErrNotFound
	}
	d.Note = note
	f.domains[domain] = d
	return d, nil
}

func (f *fakeStore) Delete(_ context.Context, domain string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.domains[domain]; !ok {
		return store.ErrNotFound
	}
	delete(f.domains, domain)
	return nil
}

func (f *fakeStore) BulkCreate(_ context.Context, items []store.BulkItem) (int, []string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	added := 0
	var existed []string
	for _, it := range items {
		if _, ok := f.domains[it.Domain]; ok {
			existed = append(existed, it.Domain)
			continue
		}
		f.domains[it.Domain] = store.Domain{Domain: it.Domain, Note: it.Note, AddedBy: it.AddedBy, AddedAt: time.Now()}
		added++
	}
	return added, existed, nil
}

func (f *fakeStore) Match(_ context.Context, candidates []string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range candidates {
		if _, ok := f.domains[c]; ok {
			return c, true, nil
		}
	}
	return "", false, nil
}

func newTestServer(t *testing.T) (*api.Server, *fakeStore) {
	t.Helper()
	fs := newFakeStore()
	cfg := config.Config{ReadKeys: []string{readKey}, WriteKeys: []string{writeKey}}
	return api.New(api.Deps{Cfg: cfg, Store: fs}), fs
}

// do issues a request and returns the recorder. contentType defaults to JSON.
func do(t *testing.T, s *api.Server, method, path, key, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != "" {
		if contentType == "" {
			contentType = "application/json"
		}
		req.Header.Set("Content-Type", contentType)
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	return w
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &m))
	return m
}

func TestHealth(t *testing.T) {
	s, _ := newTestServer(t)
	w := do(t, s, http.MethodGet, "/health", "", "", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "ok", decodeBody(t, w)["status"])
}

func TestVetRequiresAuth(t *testing.T) {
	s, _ := newTestServer(t)
	w := do(t, s, http.MethodPost, "/v1/vet", "", "", `{"email":"user@example.com"}`)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestVetEmail(t *testing.T) {
	s, fs := newTestServer(t)
	_, err := fs.Create(context.Background(), "example.com", "", "")
	require.NoError(t, err)

	// Blocked, with subdomain matching.
	w := do(t, s, http.MethodPost, "/v1/vet", readKey, "", `{"email":"user@mx.example.com"}`)
	require.Equal(t, http.StatusOK, w.Code)
	body := decodeBody(t, w)
	require.Equal(t, "blocked", body["verdict"])
	require.Equal(t, "example.com", body["matched_domain"])

	// Allowed.
	w = do(t, s, http.MethodPost, "/v1/vet", readKey, "", `{"domain":"allowed.com"}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "allowed", decodeBody(t, w)["verdict"])

	// Invalid (no @).
	w = do(t, s, http.MethodPost, "/v1/vet", readKey, "", `{"email":"not-an-email"}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "invalid", decodeBody(t, w)["verdict"])

	// Missing both email and domain.
	w = do(t, s, http.MethodPost, "/v1/vet", readKey, "", `{}`)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestVetBatch(t *testing.T) {
	s, fs := newTestServer(t)
	_, err := fs.Create(context.Background(), "example.com", "", "")
	require.NoError(t, err)

	w := do(t, s, http.MethodPost, "/v1/vet/batch", readKey, "", `{"emails":["a@example.com","b@other.com"]}`)
	require.Equal(t, http.StatusOK, w.Code)
	results := decodeBody(t, w)["results"].([]any)
	require.Len(t, results, 2)
	require.Equal(t, "blocked", results[0].(map[string]any)["verdict"])
	require.Equal(t, "allowed", results[1].(map[string]any)["verdict"])
}

func TestWriteScopeRequired(t *testing.T) {
	s, _ := newTestServer(t)
	w := do(t, s, http.MethodPost, "/v1/domains", readKey, "", `{"domain":"example.com"}`)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestDomainCRUD(t *testing.T) {
	s, _ := newTestServer(t)

	// Create.
	w := do(t, s, http.MethodPost, "/v1/domains", writeKey, "", `{"domain":"Example.COM","note":"known disposable"}`)
	require.Equal(t, http.StatusCreated, w.Code)
	body := decodeBody(t, w)
	require.Equal(t, "example.com", body["domain"])
	require.Equal(t, "known disposable", body["note"])

	// Duplicate.
	w = do(t, s, http.MethodPost, "/v1/domains", writeKey, "", `{"domain":"example.com"}`)
	require.Equal(t, http.StatusConflict, w.Code)

	// Get.
	w = do(t, s, http.MethodGet, "/v1/domains/example.com", readKey, "", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "known disposable", decodeBody(t, w)["note"])

	// Update note.
	w = do(t, s, http.MethodPatch, "/v1/domains/example.com", writeKey, "", `{"note":"updated"}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "updated", decodeBody(t, w)["note"])

	// Delete.
	w = do(t, s, http.MethodDelete, "/v1/domains/example.com", writeKey, "", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, true, decodeBody(t, w)["deleted"])

	// Now missing.
	w = do(t, s, http.MethodGet, "/v1/domains/example.com", readKey, "", "")
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestListDomains(t *testing.T) {
	s, fs := newTestServer(t)
	for _, d := range []string{"beta.com", "alpha.com"} {
		_, err := fs.Create(context.Background(), d, "", "")
		require.NoError(t, err)
	}
	w := do(t, s, http.MethodGet, "/v1/domains?q=alp", readKey, "", "")
	require.Equal(t, http.StatusOK, w.Code)
	body := decodeBody(t, w)
	require.Equal(t, float64(1), body["total"])
	items := body["items"].([]any)
	require.Len(t, items, 1)
	require.Equal(t, "alpha.com", items[0].(map[string]any)["domain"])
}

func TestImportJSON(t *testing.T) {
	s, _ := newTestServer(t)
	w := do(t, s, http.MethodPost, "/v1/domains/import", writeKey, "",
		`{"domains":[{"domain":"new1.com"},{"domain":"BAD DOMAIN"},{"domain":"new1.com"}]}`)
	require.Equal(t, http.StatusOK, w.Code)
	body := decodeBody(t, w)
	require.Equal(t, float64(1), body["added"])
	skipped := body["skipped"].([]any)
	require.Len(t, skipped, 2)
}

func TestImportCSV(t *testing.T) {
	s, _ := newTestServer(t)
	csv := "domain,note\nnew1.com,first\nnew2.com,second\n"
	w := do(t, s, http.MethodPost, "/v1/domains/import", writeKey, "text/csv", csv)
	require.Equal(t, http.StatusOK, w.Code)
	body := decodeBody(t, w)
	require.Equal(t, float64(2), body["added"])
}

func TestImportFile(t *testing.T) {
	s, _ := newTestServer(t)
	upload := func(key, field, text string, copies int) *httptest.ResponseRecorder {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		for i := 0; i < copies; i++ {
			part, err := writer.CreateFormFile(field, "domains.txt")
			require.NoError(t, err)
			_, err = io.WriteString(part, text)
			require.NoError(t, err)
		}
		require.NoError(t, writer.Close())
		return do(t, s, http.MethodPost, "/v1/domains/import/file?added_by=list-import", key, writer.FormDataContentType(), body.String())
	}
	w := upload(writeKey, "file", "\xef\xbb\xbfExample.test\r\n\r\nsecond.test\nexample.test\nBAD DOMAIN\n", 1)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decodeBody(t, w)
	require.Equal(t, float64(2), body["added"])
	require.Equal(t, float64(4), body["total"])
	require.Len(t, body["skipped"], 2)
	w = do(t, s, http.MethodGet, "/v1/domains/example.test", readKey, "", "")
	require.Equal(t, "list-import", decodeBody(t, w)["added_by"])
	w = upload(writeKey, "file", "example.test\n", 1)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, float64(0), decodeBody(t, w)["added"])
	for _, tc := range []struct {
		name, key, field, text string
		copies, status         int
	}{
		{"unauthenticated", "", "file", "third.test", 1, http.StatusUnauthorized},
		{"read key", readKey, "file", "third.test", 1, http.StatusForbidden},
		{"missing file", writeKey, "wrong", "third.test", 1, http.StatusUnprocessableEntity},
		{"multiple files", writeKey, "file", "third.test", 2, http.StatusUnprocessableEntity},
		{"empty file", writeKey, "file", "\r\n", 1, http.StatusBadRequest},
		{"oversized", writeKey, "file", strings.Repeat("x", 8<<20), 1, http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := upload(tc.key, tc.field, tc.text, tc.copies)
			require.Equal(t, tc.status, w.Code, w.Body.String())
		})
	}
}
