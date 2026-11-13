// Package api exposes the emailvetter HTTP API using Huma v2 (type-first
// OpenAPI 3.1 with built-in request validation and docs). Vetting reads and
// registry writes are separated by API-key scope.
package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/miguelpragier/emailvetter/internal/config"
	"github.com/miguelpragier/emailvetter/internal/store"
)

const version = "0.1.0"

// Deps are the services the API depends on.
type Deps struct {
	Cfg   config.Config
	Store store.Store
}

// Server is the emailvetter HTTP API server.
type Server struct {
	deps   Deps
	api    huma.API
	server *http.Server
	// read and write hold SHA-256 hashes of the configured API keys, so the raw
	// keys never live in memory after startup.
	read, write  map[string]struct{}
	authDisabled bool
}

// New builds the server with all routes registered.
func New(deps Deps) *Server {
	mux := http.NewServeMux()

	cfg := huma.DefaultConfig("Email Vetter API", version)
	cfg.Info.Description = "Vet email addresses and domains against an operator-managed " +
		"disposable-email blocklist. Authenticate with Authorization: Bearer <api-key>; " +
		"read keys may vet, write keys may also edit the registry."
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearer": {Type: "http", Scheme: "bearer", Description: "A static API key (read or write scope)"},
	}
	api := humago.New(mux, cfg)

	s := &Server{deps: deps, api: api}
	s.read, s.write = keySets(deps.Cfg)
	s.authDisabled = !deps.Cfg.AuthEnabled()
	api.UseMiddleware(s.authMiddleware)

	s.registerSystem()
	s.registerVet()
	s.registerVetBatch()
	s.registerDomains()
	s.registerImport()

	s.server = &http.Server{
		Addr:              deps.Cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s
}

// Handler returns the fully wired HTTP handler (useful for tests).
func (s *Server) Handler() http.Handler { return s.server.Handler }

// Start listens and blocks until ctx is cancelled, then shuts down gracefully.
func (s *Server) Start(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.server.Shutdown(shutdownCtx)
	}
}

// scope names what an operation may do.
type scope string

const (
	// scopeVetRead allows vetting and reading the registry.
	scopeVetRead scope = "vet:read"
	// scopeDomainsWrite allows editing the registry (and, implicitly, reading).
	scopeDomainsWrite scope = "domains:write"
)

type principal struct{ write bool }

type principalCtxKey struct{}

// authMiddleware resolves the Bearer API key, if the operation requires
// authentication, and stores the granted scopes in the request context. When
// authentication is disabled (no keys configured, non-production only) or the
// operation is public (health), it passes through.
func (s *Server) authMiddleware(ctx huma.Context, next func(huma.Context)) {
	if s.authDisabled || !requiresAuth(ctx.Operation()) {
		next(ctx)
		return
	}
	key, ok := bearerKey(ctx.Header("Authorization"))
	h := hashKey(key)
	if !ok || !s.known(h) {
		ctx.SetHeader("WWW-Authenticate", `Bearer realm="emailvetter"`)
		_ = huma.WriteErr(s.api, ctx, http.StatusUnauthorized, "missing or invalid API key")
		return
	}
	_, write := s.write[h]
	ctx = huma.WithValue(ctx, principalCtxKey{}, principal{write: write})
	next(ctx)
}

func (s *Server) known(hash string) bool {
	_, r := s.read[hash]
	_, w := s.write[hash]
	return r || w
}

// requiresAuth reports whether the operation declares a security requirement.
func requiresAuth(op *huma.Operation) bool {
	return op != nil && len(op.Security) > 0
}

// bearerKey extracts the token from an "Authorization: Bearer <token>" header.
func bearerKey(authz string) (string, bool) {
	scheme, token, ok := strings.Cut(authz, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

func hashKey(k string) string {
	sum := sha256.Sum256([]byte(k))
	return hex.EncodeToString(sum[:])
}

// keySets hashes the configured keys into lookup sets.
func keySets(cfg config.Config) (read, write map[string]struct{}) {
	read = make(map[string]struct{}, len(cfg.ReadKeys))
	for _, k := range cfg.ReadKeys {
		if k = strings.TrimSpace(k); k != "" {
			read[hashKey(k)] = struct{}{}
		}
	}
	write = make(map[string]struct{}, len(cfg.WriteKeys))
	for _, k := range cfg.WriteKeys {
		if k = strings.TrimSpace(k); k != "" {
			write[hashKey(k)] = struct{}{}
		}
	}
	return read, write
}

// requireScope enforces that the authenticated caller may exercise scope. It is
// called by every protected handler (the middleware only authenticates; this
// authorizes).
func requireScope(ctx context.Context, want scope) error {
	p, ok := ctx.Value(principalCtxKey{}).(principal)
	if !ok {
		return huma.Error401Unauthorized("authentication required")
	}
	if want == scopeDomainsWrite && !p.write {
		return huma.Error403Forbidden("this API key lacks the domains:write scope")
	}
	return nil
}

// bearerSecurity documents an operation's required scope in the OpenAPI file.
func bearerSecurity(s scope) []map[string][]string {
	return []map[string][]string{{"bearer": {string(s)}}}
}
