package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/miguelpragier/emailvetter/internal/domain"
	"github.com/miguelpragier/emailvetter/internal/store"
)

// DomainBody is the JSON shape of one registry entry.
type DomainBody struct {
	Domain  string    `json:"domain"`
	Note    string    `json:"note"`
	AddedBy string    `json:"added_by"`
	AddedAt time.Time `json:"added_at"`
}

func domainBody(d store.Domain) DomainBody {
	return DomainBody{Domain: d.Domain, Note: d.Note, AddedBy: d.AddedBy, AddedAt: d.AddedAt}
}

// ListInput filters and pages the registry.
type ListInput struct {
	Query  string `query:"q" doc:"Case-insensitive substring filter on the domain"`
	Limit  int    `query:"limit" default:"50" minimum:"1" maximum:"200" doc:"Page size"`
	Offset int    `query:"offset" default:"0" minimum:"0" doc:"Number of rows to skip"`
}

// ListBody is the registry page.
type ListBody struct {
	Items  []DomainBody `json:"items"`
	Total  int          `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}

// ListOutput wraps the registry page.
type ListOutput struct {
	Body ListBody
}

// GetInput identifies one entry by domain.
type GetInput struct {
	Domain string `path:"domain" doc:"The domain, e.g. example.com"`
}

// GetOutput wraps one entry.
type GetOutput struct {
	Body DomainBody
}

// CreateInput adds one entry.
type CreateInput struct {
	Body struct {
		Domain  string `json:"domain" required:"true" doc:"The domain to block"`
		Note    string `json:"note,omitempty" doc:"Optional free-text reason"`
		AddedBy string `json:"added_by,omitempty" doc:"Who or what added it"`
	}
}

// CreateOutput wraps the created entry.
type CreateOutput struct {
	Body DomainBody
}

// UpdateNoteInput replaces an entry's note.
type UpdateNoteInput struct {
	Domain string `path:"domain" doc:"The domain, e.g. example.com"`
	Body   struct {
		Note string `json:"note" doc:"New free-text note"`
	}
}

// UpdateNoteOutput wraps the updated entry.
type UpdateNoteOutput struct {
	Body DomainBody
}

// DeleteInput identifies one entry to remove.
type DeleteInput struct {
	Domain string `path:"domain" doc:"The domain, e.g. example.com"`
}

// DeleteOutput confirms removal.
type DeleteOutput struct {
	Body struct {
		Domain  string `json:"domain"`
		Deleted bool   `json:"deleted"`
	}
}

func (s *Server) registerDomains() {
	huma.Register(s.api, huma.Operation{
		OperationID: "list-domains",
		Method:      http.MethodGet,
		Path:        "/v1/domains",
		Summary:     "List blocked domains",
		Security:    bearerSecurity(scopeVetRead),
		Tags:        []string{"domains"},
	}, func(ctx context.Context, in *ListInput) (*ListOutput, error) {
		if err := requireScope(ctx, scopeVetRead); err != nil {
			return nil, err
		}
		items, total, err := s.deps.Store.List(ctx, in.Query, in.Limit, in.Offset)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to list domains")
		}
		body := make([]DomainBody, 0, len(items))
		for _, d := range items {
			body = append(body, domainBody(d))
		}
		return &ListOutput{Body: ListBody{Items: body, Total: total, Limit: in.Limit, Offset: in.Offset}}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "get-domain",
		Method:      http.MethodGet,
		Path:        "/v1/domains/{domain}",
		Summary:     "Get a blocked domain",
		Security:    bearerSecurity(scopeVetRead),
		Tags:        []string{"domains"},
	}, func(ctx context.Context, in *GetInput) (*GetOutput, error) {
		if err := requireScope(ctx, scopeVetRead); err != nil {
			return nil, err
		}
		d, err := s.deps.Store.Get(ctx, in.Domain)
		if err != nil {
			return nil, storeError(err)
		}
		return &GetOutput{Body: domainBody(d)}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID:   "create-domain",
		Method:        http.MethodPost,
		Path:          "/v1/domains",
		Summary:       "Block a domain",
		Security:      bearerSecurity(scopeDomainsWrite),
		Tags:          []string{"domains"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *CreateInput) (*CreateOutput, error) {
		if err := requireScope(ctx, scopeDomainsWrite); err != nil {
			return nil, err
		}
		d, err := domain.NormalizeDomain(in.Body.Domain)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid domain")
		}
		created, err := s.deps.Store.Create(ctx, d, in.Body.Note, in.Body.AddedBy)
		if err != nil {
			return nil, storeError(err)
		}
		return &CreateOutput{Body: domainBody(created)}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "update-domain-note",
		Method:      http.MethodPatch,
		Path:        "/v1/domains/{domain}",
		Summary:     "Update a blocked domain's note",
		Security:    bearerSecurity(scopeDomainsWrite),
		Tags:        []string{"domains"},
	}, func(ctx context.Context, in *UpdateNoteInput) (*UpdateNoteOutput, error) {
		if err := requireScope(ctx, scopeDomainsWrite); err != nil {
			return nil, err
		}
		updated, err := s.deps.Store.UpdateNote(ctx, in.Domain, in.Body.Note)
		if err != nil {
			return nil, storeError(err)
		}
		return &UpdateNoteOutput{Body: domainBody(updated)}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "delete-domain",
		Method:      http.MethodDelete,
		Path:        "/v1/domains/{domain}",
		Summary:     "Unblock a domain",
		Security:    bearerSecurity(scopeDomainsWrite),
		Tags:        []string{"domains"},
	}, func(ctx context.Context, in *DeleteInput) (*DeleteOutput, error) {
		if err := requireScope(ctx, scopeDomainsWrite); err != nil {
			return nil, err
		}
		if err := s.deps.Store.Delete(ctx, in.Domain); err != nil {
			return nil, storeError(err)
		}
		return &DeleteOutput{Body: struct {
			Domain  string `json:"domain"`
			Deleted bool   `json:"deleted"`
		}{Domain: in.Domain, Deleted: true}}, nil
	})
}

// storeError maps store conditions to their HTTP equivalents.
func storeError(err error) error {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return huma.Error404NotFound("domain not found")
	case errors.Is(err, store.ErrExists):
		return huma.Error409Conflict("domain already exists")
	default:
		return err
	}
}
