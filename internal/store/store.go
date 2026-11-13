// Package store persists blocked email domains in rqlite.
package store

import (
	"context"
	"errors"
	"time"
)

// Errors the store returns for domain-not-found / duplicate conditions.
var (
	// ErrNotFound is returned when the requested domain is not in the registry.
	ErrNotFound = errors.New("domain not found")
	// ErrExists is returned when adding a domain already present.
	ErrExists = errors.New("domain already exists")
)

// Domain is one blocked-domain registry entry.
type Domain struct {
	Domain  string
	Note    string
	AddedBy string
	AddedAt time.Time
}

// BulkItem is one entry of a bulk import, already normalized and de-duplicated
// by the caller.
type BulkItem struct {
	Domain  string
	Note    string
	AddedBy string
}

// Store is the storage contract the API depends on. The rqlite-backed
// implementation is built by New.
type Store interface {
	// Ping verifies the store is reachable.
	Ping(ctx context.Context) error

	// EnsureSchema applies the schema and seeds the curated domain list once.
	EnsureSchema(ctx context.Context) error

	// List returns the page matching the case-insensitive substring filter
	// (empty matches everything) ordered by domain, plus the total number of
	// matching rows.
	List(ctx context.Context, filter string, limit, offset int) ([]Domain, int, error)

	// Get returns one entry, or ErrNotFound.
	Get(ctx context.Context, domain string) (Domain, error)

	// Create inserts one entry, returning ErrExists on a duplicate.
	Create(ctx context.Context, domain, note, addedBy string) (Domain, error)

	// UpdateNote replaces an entry's note, returning ErrNotFound if absent.
	UpdateNote(ctx context.Context, domain, note string) (Domain, error)

	// Delete removes one entry, returning ErrNotFound if absent.
	Delete(ctx context.Context, domain string) error

	// BulkCreate inserts items atomically, returning how many were added and the
	// domains that already existed.
	BulkCreate(ctx context.Context, items []BulkItem) (added int, existed []string, err error)

	// Match reports whether any of candidates (most-specific first) is blocked
	// and, if so, the most specific matching domain.
	Match(ctx context.Context, candidates []string) (matched string, blocked bool, err error)
}
