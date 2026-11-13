package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// HealthBody is the health payload.
type HealthBody struct {
	Status  string `json:"status" example:"ok"`
	Version string `json:"version" example:"0.1.0"`
}

// HealthOutput wraps the health payload.
type HealthOutput struct {
	Body HealthBody
}

func (s *Server) registerSystem() {
	huma.Register(s.api, huma.Operation{
		OperationID: "health",
		Method:      http.MethodGet,
		Path:        "/health",
		Summary:     "Health check",
		Description: "Reports service health; fails with 503 when rqlite is unreachable.",
		Tags:        []string{"system"},
	}, func(ctx context.Context, _ *struct{}) (*HealthOutput, error) {
		if err := s.deps.Store.Ping(ctx); err != nil {
			return nil, huma.Error503ServiceUnavailable("database unavailable")
		}
		return &HealthOutput{Body: HealthBody{Status: "ok", Version: version}}, nil
	})
}
