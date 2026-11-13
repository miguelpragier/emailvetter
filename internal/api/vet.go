package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/miguelpragier/emailvetter/internal/domain"
)

// VetInput carries exactly one of email or domain.
type VetInput struct {
	Body struct {
		Email  string `json:"email,omitempty" doc:"Email address to vet"`
		Domain string `json:"domain,omitempty" doc:"Domain to vet"`
	}
}

// VetResult is one vetting verdict.
type VetResult struct {
	Requested     string `json:"requested" doc:"The input as normalized (email as submitted, domain lower-cased)"`
	Verdict       string `json:"verdict" doc:"allowed, blocked or invalid"`
	MatchedDomain string `json:"matched_domain,omitempty" doc:"The blocked domain that matched, when verdict is blocked"`
}

// VetOutput wraps one vetting verdict.
type VetOutput struct {
	Body VetResult
}

// VetBatchInput vets several emails and/or domains at once.
type VetBatchInput struct {
	Body struct {
		Emails  []string `json:"emails,omitempty" doc:"Email addresses to vet"`
		Domains []string `json:"domains,omitempty" doc:"Domains to vet"`
	}
}

// VetBatchOutput wraps the batch verdicts.
type VetBatchOutput struct {
	Body struct {
		Results []VetResult `json:"results"`
	}
}

// maxVetBatch bounds a single batch request to keep it cheap.
const maxVetBatch = 1000

func (s *Server) registerVet() {
	huma.Register(s.api, huma.Operation{
		OperationID: "vet",
		Method:      http.MethodPost,
		Path:        "/v1/vet",
		Summary:     "Vet an email or domain",
		Description: "Reports whether an email address or domain is a disposable/blocked domain. " +
			"Blocking a domain also blocks its subdomains.",
		Security: bearerSecurity(scopeVetRead),
		Tags:     []string{"vet"},
	}, func(ctx context.Context, in *VetInput) (*VetOutput, error) {
		if err := requireScope(ctx, scopeVetRead); err != nil {
			return nil, err
		}
		if (in.Body.Email == "") == (in.Body.Domain == "") {
			return nil, huma.Error400BadRequest("provide exactly one of email or domain")
		}
		res, err := s.vet(ctx, in.Body.Email, in.Body.Domain)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to check the blocklist")
		}
		return &VetOutput{Body: res}, nil
	})
}

func (s *Server) registerVetBatch() {
	huma.Register(s.api, huma.Operation{
		OperationID: "vet-batch",
		Method:      http.MethodPost,
		Path:        "/v1/vet/batch",
		Summary:     "Vet several emails and/or domains",
		Security:    bearerSecurity(scopeVetRead),
		Tags:        []string{"vet"},
	}, func(ctx context.Context, in *VetBatchInput) (*VetBatchOutput, error) {
		if err := requireScope(ctx, scopeVetRead); err != nil {
			return nil, err
		}
		if len(in.Body.Emails)+len(in.Body.Domains) == 0 {
			return nil, huma.Error400BadRequest("provide at least one email or domain")
		}
		if len(in.Body.Emails)+len(in.Body.Domains) > maxVetBatch {
			return nil, huma.Error400BadRequest("batch exceeds the maximum size")
		}
		results := make([]VetResult, 0, len(in.Body.Emails)+len(in.Body.Domains))
		for _, email := range in.Body.Emails {
			res, err := s.vet(ctx, email, "")
			if err != nil {
				return nil, huma.Error500InternalServerError("failed to check the blocklist")
			}
			results = append(results, res)
		}
		for _, d := range in.Body.Domains {
			res, err := s.vet(ctx, "", d)
			if err != nil {
				return nil, huma.Error500InternalServerError("failed to check the blocklist")
			}
			results = append(results, res)
		}
		return &VetBatchOutput{Body: struct {
			Results []VetResult `json:"results"`
		}{Results: results}}, nil
	})
}

// vet resolves one verdict for an email or a domain (exactly one non-empty).
func (s *Server) vet(ctx context.Context, email, inputDomain string) (VetResult, error) {
	if email != "" {
		d, err := domain.DomainOfEmail(email)
		if err != nil {
			return VetResult{Requested: email, Verdict: string(domain.VerdictInvalid)}, nil
		}
		return s.check(ctx, email, domain.Candidates(d))
	}
	d, err := domain.NormalizeDomain(inputDomain)
	if err != nil {
		return VetResult{Requested: inputDomain, Verdict: string(domain.VerdictInvalid)}, nil
	}
	return s.check(ctx, d, domain.Candidates(d))
}

func (s *Server) check(ctx context.Context, requested string, candidates []string) (VetResult, error) {
	matched, blocked, err := s.deps.Store.Match(ctx, candidates)
	if err != nil {
		return VetResult{}, err
	}
	if !blocked {
		return VetResult{Requested: requested, Verdict: string(domain.VerdictAllowed)}, nil
	}
	return VetResult{Requested: requested, Verdict: string(domain.VerdictBlocked), MatchedDomain: matched}, nil
}
