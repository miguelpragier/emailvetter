package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/miguelpragier/emailvetter/internal/schema"
)

// rqliteStore implements Store against rqlite's HTTP API.
type rqliteStore struct {
	base string // e.g. "http://127.0.0.1:4001"
	user string
	pass string
	hc   *http.Client
}

// New builds a Store backed by rqlite at addr (an http/https URL, trailing
// slash trimmed). user/pass enable HTTP Basic Auth when both are set.
func New(addr, user, pass string, timeout time.Duration) Store {
	return &rqliteStore{
		base: strings.TrimRight(addr, "/"),
		user: user,
		pass: pass,
		hc:   &http.Client{Timeout: timeout},
	}
}

// statement builds one parameterized statement for the rqlite JSON request
// body: [sql, arg1, arg2, ...].
func statement(sql string, args []any) []any {
	out := make([]any, 0, len(args)+1)
	out = append(out, sql)
	out = append(out, args...)
	return out
}

// result is the per-statement result rqlite returns, for both queries and
// writes. Only the fields emailvetter reads are populated.
type result struct {
	Types        map[string]string `json:"types"`
	Rows         []map[string]any  `json:"rows"`
	LastInsertID int64             `json:"last_insert_id"`
	RowsAffected int64             `json:"rows_affected"`
	Error        string            `json:"error"`
	ErrorV2      *struct {
		Message string `json:"message"`
	} `json:"error_v2"`
}

type response struct {
	Results []result `json:"results"`
}

// do posts statements to path and returns the parsed response. It treats an
// HTTP error status and an in-band statement error identically.
func (s *rqliteStore) do(ctx context.Context, path string, statements []any) (*response, error) {
	body, err := json.Marshal(statements)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.base+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.user != "" {
		req.SetBasicAuth(s.user, s.pass)
	}

	resp, err := s.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rqlite: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("rqlite: read response: %w", err)
	}

	var r response
	if err := json.Unmarshal(data, &r); err != nil {
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("rqlite: http %s: %s", resp.Status, strings.TrimSpace(string(data)))
		}
		return nil, fmt.Errorf("rqlite: decode response: %w", err)
	}
	if resp.StatusCode >= 400 {
		if msg := firstError(&r); msg != "" {
			return nil, errors.New("rqlite: " + msg)
		}
		return nil, fmt.Errorf("rqlite: http %s", resp.Status)
	}
	// Even on HTTP 200 a statement may have failed (rqlite reports these
	// in-band; see error_v2 in the API docs).
	if msg := firstError(&r); msg != "" {
		return nil, errors.New("rqlite: " + msg)
	}
	return &r, nil
}

// firstError returns the first error message in the response, if any.
func firstError(r *response) string {
	for _, res := range r.Results {
		if res.ErrorV2 != nil && res.ErrorV2.Message != "" {
			return res.ErrorV2.Message
		}
		if res.Error != "" {
			return res.Error
		}
	}
	return ""
}

// query runs one read statement and returns its rows in associative form.
func (s *rqliteStore) query(ctx context.Context, sql string, args ...any) ([]map[string]any, error) {
	r, err := s.do(ctx, "/db/query?associative&level=strong", []any{statement(sql, args)})
	if err != nil {
		return nil, err
	}
	if len(r.Results) == 0 {
		return nil, nil
	}
	return r.Results[0].Rows, nil
}

// execute runs one write statement and returns rows_affected.
func (s *rqliteStore) execute(ctx context.Context, sql string, args ...any) (int64, error) {
	r, err := s.do(ctx, "/db/execute", []any{statement(sql, args)})
	if err != nil {
		return 0, err
	}
	if len(r.Results) == 0 {
		return 0, nil
	}
	return r.Results[0].RowsAffected, nil
}

// executeBatch runs statements atomically and returns the per-statement results.
func (s *rqliteStore) executeBatch(ctx context.Context, statements []any) ([]result, error) {
	r, err := s.do(ctx, "/db/execute?transaction", statements)
	if err != nil {
		return nil, err
	}
	return r.Results, nil
}

// Ping verifies the store is reachable and queryable.
func (s *rqliteStore) Ping(ctx context.Context) error {
	_, err := s.query(ctx, `SELECT 1 AS ok`)
	return err
}

// EnsureSchema creates missing tables without modifying existing domain entries.
func (s *rqliteStore) EnsureSchema(ctx context.Context) error {
	for _, stmt := range splitSQL(schema.SQL) {
		if _, err := s.execute(ctx, stmt); err != nil {
			return fmt.Errorf("apply schema: %w", err)
		}
	}
	return nil
}

// splitSQL splits a SQL script on ";" into trimmed, non-empty statements. The
// embedded schema contains no semicolon inside a string literal, so this is
// safe.
func splitSQL(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ";") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (s *rqliteStore) List(ctx context.Context, filter string, limit, offset int) ([]Domain, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	f := strings.ToLower(strings.TrimSpace(filter))

	countRows, err := s.query(ctx,
		`SELECT count(*) AS n FROM blocked_domains WHERE ? = '' OR instr(domain, ?) > 0`, f, f)
	if err != nil {
		return nil, 0, err
	}
	total := 0
	if len(countRows) > 0 {
		total = asInt(countRows[0]["n"])
	}

	rows, err := s.query(ctx,
		`SELECT domain, note, added_by, added_at FROM blocked_domains
		 WHERE ? = '' OR instr(domain, ?) > 0
		 ORDER BY domain LIMIT ? OFFSET ?`, f, f, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	out := make([]Domain, 0, len(rows))
	for _, r := range rows {
		out = append(out, domainFromRow(r))
	}
	return out, total, nil
}

func (s *rqliteStore) Get(ctx context.Context, domain string) (Domain, error) {
	rows, err := s.query(ctx,
		`SELECT domain, note, added_by, added_at FROM blocked_domains WHERE domain = ?`, domain)
	if err != nil {
		return Domain{}, err
	}
	if len(rows) == 0 {
		return Domain{}, ErrNotFound
	}
	return domainFromRow(rows[0]), nil
}

func (s *rqliteStore) Create(ctx context.Context, domain, note, addedBy string) (Domain, error) {
	now := time.Now().UTC()
	added, err := s.execute(ctx,
		`INSERT OR IGNORE INTO blocked_domains (domain, note, added_by, added_at) VALUES (?, ?, ?, ?)`,
		domain, note, addedBy, now.Format(time.RFC3339))
	if err != nil {
		return Domain{}, err
	}
	if added == 0 {
		return Domain{}, ErrExists
	}
	return Domain{Domain: domain, Note: note, AddedBy: addedBy, AddedAt: now}, nil
}

func (s *rqliteStore) UpdateNote(ctx context.Context, domain, note string) (Domain, error) {
	updated, err := s.execute(ctx,
		`UPDATE blocked_domains SET note = ? WHERE domain = ?`, note, domain)
	if err != nil {
		return Domain{}, err
	}
	if updated == 0 {
		return Domain{}, ErrNotFound
	}
	return s.Get(ctx, domain)
}

func (s *rqliteStore) Delete(ctx context.Context, domain string) error {
	deleted, err := s.execute(ctx, `DELETE FROM blocked_domains WHERE domain = ?`, domain)
	if err != nil {
		return err
	}
	if deleted == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *rqliteStore) BulkCreate(ctx context.Context, items []BulkItem) (int, []string, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	stmts := make([]any, 0, len(items))
	for _, it := range items {
		stmts = append(stmts, statement(
			`INSERT OR IGNORE INTO blocked_domains (domain, note, added_by, added_at) VALUES (?, ?, ?, ?)`,
			[]any{it.Domain, it.Note, it.AddedBy, now}))
	}
	results, err := s.executeBatch(ctx, stmts)
	if err != nil {
		return 0, nil, err
	}
	added := 0
	var existed []string
	for i, res := range results {
		if i >= len(items) {
			break
		}
		if res.RowsAffected > 0 {
			added++
		} else {
			existed = append(existed, items[i].Domain)
		}
	}
	return added, existed, nil
}

func (s *rqliteStore) Match(ctx context.Context, candidates []string) (string, bool, error) {
	if len(candidates) == 0 {
		return "", false, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(candidates)), ",")
	args := make([]any, len(candidates))
	for i, c := range candidates {
		args[i] = c
	}
	rows, err := s.query(ctx,
		`SELECT domain FROM blocked_domains WHERE domain IN (`+placeholders+`)`, args...)
	if err != nil {
		return "", false, err
	}
	found := make(map[string]bool, len(rows))
	for _, r := range rows {
		found[asString(r["domain"])] = true
	}
	// Candidates are most-specific first, so the first present one is the
	// most specific match (blocking example.com blocks mx.example.com, and the
	// verdict reports example.com as the matched domain).
	for _, c := range candidates {
		if found[c] {
			return c, true, nil
		}
	}
	return "", false, nil
}

func domainFromRow(r map[string]any) Domain {
	return Domain{
		Domain:  asString(r["domain"]),
		Note:    asString(r["note"]),
		AddedBy: asString(r["added_by"]),
		AddedAt: asTime(r["added_at"]),
	}
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	}
	return 0
}

func asTime(v any) time.Time {
	s, _ := v.(string)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}
