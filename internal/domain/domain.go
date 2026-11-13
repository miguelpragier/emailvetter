// Package domain holds the pure, storage-agnostic logic for vetting email
// domains against a disposable-domain blocklist: normalization, subdomain
// expansion and email parsing. It performs no I/O.
package domain

import (
	"errors"
	"net/mail"
	"strings"
)

// Errors returned for unusable input.
var (
	// ErrInvalidDomain is returned when a domain is not a valid hostname-like
	// domain (dot-separated ASCII labels, at least two of them).
	ErrInvalidDomain = errors.New("invalid domain")
	// ErrInvalidEmail is returned when an address is not a plain mailbox.
	ErrInvalidEmail = errors.New("invalid email address")
)

// maxDomainLen is the longest a domain name may be (RFC 1035).
const maxDomainLen = 253

// maxEmailLen is the longest address RFC 5321 allows.
const maxEmailLen = 254

// NormalizeDomain trims, lower-cases and validates a domain for the blocklist.
// A valid domain is a hostname: dot-separated ASCII labels, at least two labels
// (so it is not a bare TLD), no @, scheme, port or path, and at most 253
// characters.
func NormalizeDomain(domain string) (string, error) {
	d := strings.ToLower(strings.TrimSpace(domain))
	if d == "" || len(d) > maxDomainLen {
		return "", ErrInvalidDomain
	}
	if strings.ContainsAny(d, "@/\\:") {
		return "", ErrInvalidDomain
	}
	labels := strings.Split(d, ".")
	if len(labels) < 2 {
		return "", ErrInvalidDomain
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 {
			return "", ErrInvalidDomain
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return "", ErrInvalidDomain
		}
		for _, r := range label {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
				continue
			}
			return "", ErrInvalidDomain
		}
	}
	return d, nil
}

// Candidates returns the domain and each of its parent suffixes that still
// contains a dot, so blocking example.com also blocks mx.example.com. A bare
// label (no dot) is returned as-is. The result is ordered most-specific first.
func Candidates(domain string) []string {
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return []string{domain}
	}
	out := make([]string, 0, len(labels)-1)
	for i := range len(labels) - 1 {
		out = append(out, strings.Join(labels[i:], "."))
	}
	return out
}

// DomainOfEmail returns the lower-cased, trimmed domain part of a bare mailbox
// ("local@domain"), or ErrInvalidEmail if the address is not a plain mailbox
// with a non-empty domain. It does not require the domain to be hostname-like:
// the caller decides how strict to be.
func DomainOfEmail(email string) (string, error) {
	e := strings.ToLower(strings.TrimSpace(email))
	if e == "" || len(e) > maxEmailLen {
		return "", ErrInvalidEmail
	}
	a, err := mail.ParseAddress(e)
	if err != nil || a.Address != e || a.Name != "" {
		return "", ErrInvalidEmail
	}
	at := strings.LastIndexByte(e, '@')
	if at < 1 || e[at+1:] == "" {
		return "", ErrInvalidEmail
	}
	return e[at+1:], nil
}

// Verdict is the outcome of vetting one address or domain.
type Verdict string

const (
	// VerdictAllowed means the domain is not in the blocklist.
	VerdictAllowed Verdict = "allowed"
	// VerdictBlocked means the domain (or a parent suffix) is in the blocklist.
	VerdictBlocked Verdict = "blocked"
	// VerdictInvalid means the input was not a usable email or domain.
	VerdictInvalid Verdict = "invalid"
)
