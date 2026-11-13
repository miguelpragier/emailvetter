package api

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/miguelpragier/emailvetter/internal/domain"
	"github.com/miguelpragier/emailvetter/internal/store"
)

// maxImportBytes bounds a single import body, allowing bulk domain imports
// while limiting memory use.
const maxImportBytes = 8 << 20

// ImportInput carries the raw import body plus an optional actor.
type ImportInput struct {
	AddedBy     string `query:"added_by" doc:"Actor recorded for every imported row"`
	ContentType string `header:"Content-Type"`
	RawBody     []byte `contentType:"application/octet-stream"`
}

// ImportSkipped records one row that was not imported and why.
type ImportSkipped struct {
	Input  string `json:"input"`
	Reason string `json:"reason" doc:"invalid, duplicate_input or already_exists"`
}

// ImportBody is the import summary.
type ImportBody struct {
	Added   int             `json:"added"`
	Skipped []ImportSkipped `json:"skipped"`
	Total   int             `json:"total"`
}

// ImportOutput wraps the import summary.
type ImportOutput struct {
	Body ImportBody
}

func (s *Server) registerImport() {
	huma.Register(s.api, huma.Operation{
		OperationID: "import-domains",
		Method:      http.MethodPost,
		Path:        "/v1/domains/import",
		Summary:     "Import domains in bulk",
		Description: "Bulk-imports domains from a JSON body " +
			`({"domains":[{"domain":"x","note":"y"},...]} or ["x.com","y.com"]), a CSV body ` +
			`(optional header "domain,note", then one row per line) or a text body ` +
			`(one domain per line).`,
		Security:     bearerSecurity(scopeDomainsWrite),
		Tags:         []string{"domains"},
		MaxBodyBytes: maxImportBytes,
	}, func(ctx context.Context, in *ImportInput) (*ImportOutput, error) {
		if err := requireScope(ctx, scopeDomainsWrite); err != nil {
			return nil, err
		}
		raws, err := parseImport(in.RawBody, in.ContentType)
		if err != nil {
			return nil, huma.Error400BadRequest("malformed import: " + err.Error())
		}
		items, skipped := normalizeImport(raws, in.AddedBy)
		added, existed, err := s.deps.Store.BulkCreate(ctx, items)
		if err != nil {
			return nil, huma.Error500InternalServerError("import failed")
		}
		for _, d := range existed {
			skipped = append(skipped, ImportSkipped{Input: d, Reason: "already_exists"})
		}
		return &ImportOutput{Body: ImportBody{Added: added, Skipped: skipped, Total: added + len(skipped)}}, nil
	})
}

// importRaw is one parsed entry before normalization.
type importRaw struct {
	domain string
	note   string
}

// parseImport dispatches on the request Content-Type. A missing or unparsable
// Content-Type falls back to JSON, then to one-domain-per-line text.
func parseImport(body []byte, contentType string) ([]importRaw, error) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = ""
	}
	switch mediaType {
	case "text/csv":
		return parseCSV(body)
	case "text/plain":
		return parseText(body)
	default:
		if raws, err := parseJSON(body); err == nil {
			return raws, nil
		}
		return parseText(body)
	}
}

func parseJSON(body []byte) ([]importRaw, error) {
	// Structured form: {"domains":[{"domain":"x","note":"y"}, ...]}.
	var structured struct {
		Domains []struct {
			Domain string `json:"domain"`
			Note   string `json:"note"`
		} `json:"domains"`
	}
	if err := json.Unmarshal(body, &structured); err == nil && len(structured.Domains) > 0 {
		out := make([]importRaw, 0, len(structured.Domains))
		for _, d := range structured.Domains {
			out = append(out, importRaw{domain: d.Domain, note: d.Note})
		}
		return out, nil
	}
	// Bare array of domain strings: ["x.com", "y.com"].
	var bare []string
	if err := json.Unmarshal(body, &bare); err == nil {
		out := make([]importRaw, 0, len(bare))
		for _, d := range bare {
			out = append(out, importRaw{domain: d})
		}
		return out, nil
	}
	return nil, errors.New(`body is not valid JSON (expected {"domains":[...]} or a string array)`)
}

func parseCSV(body []byte) ([]importRaw, error) {
	r := csv.NewReader(bytes.NewReader(body))
	r.FieldsPerRecord = -1 // accept 1 (domain) or 2 (domain, note) columns
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	out := make([]importRaw, 0, len(records))
	for i, rec := range records {
		for j := range rec {
			rec[j] = strings.TrimSpace(rec[j])
		}
		// Skip an optional header row.
		if i == 0 && len(rec) > 0 && strings.EqualFold(rec[0], "domain") {
			continue
		}
		if len(rec) == 0 || rec[0] == "" {
			continue
		}
		item := importRaw{domain: rec[0]}
		if len(rec) >= 2 {
			item.note = rec[1]
		}
		out = append(out, item)
	}
	return out, nil
}

func parseText(body []byte) ([]importRaw, error) {
	var out []importRaw
	for _, line := range strings.Split(string(body), "\n") {
		if d := strings.TrimSpace(line); d != "" {
			out = append(out, importRaw{domain: d})
		}
	}
	return out, nil
}

// normalizeImport lower-cases and validates each raw entry, dropping malformed
// and in-batch duplicates, and reports every drop with a reason.
func normalizeImport(raws []importRaw, addedBy string) ([]store.BulkItem, []ImportSkipped) {
	seen := make(map[string]struct{}, len(raws))
	items := make([]store.BulkItem, 0, len(raws))
	var skipped []ImportSkipped
	for _, r := range raws {
		d, err := domain.NormalizeDomain(r.domain)
		if err != nil {
			skipped = append(skipped, ImportSkipped{Input: r.domain, Reason: "invalid"})
			continue
		}
		if _, dup := seen[d]; dup {
			skipped = append(skipped, ImportSkipped{Input: r.domain, Reason: "duplicate_input"})
			continue
		}
		seen[d] = struct{}{}
		items = append(items, store.BulkItem{Domain: d, Note: strings.TrimSpace(r.note), AddedBy: addedBy})
	}
	return items, skipped
}
