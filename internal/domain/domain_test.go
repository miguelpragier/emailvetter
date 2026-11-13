package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeDomain(t *testing.T) {
	valid := []struct{ in, want string }{
		{"Example.COM", "example.com"},
		{"  example.com ", "example.com"},
		{"mx.example.com", "mx.example.com"},
		{"a-b.example.co.uk", "a-b.example.co.uk"},
	}
	for _, tt := range valid {
		got, err := NormalizeDomain(tt.in)
		require.NoError(t, err, tt.in)
		require.Equal(t, tt.want, got)
	}

	invalid := []string{
		"",
		"localhost",
		"example",
		"@example.com",
		"example.com/path",
		"https://example.com",
		"exa mple.com",
		"-example.com",
		"example-.com",
		"a..com",
		strings.Repeat("a", 64) + ".com",
		strings.Repeat("a.", 200) + "com",
	}
	for _, in := range invalid {
		_, err := NormalizeDomain(in)
		require.ErrorIs(t, err, ErrInvalidDomain, in)
	}
}

func TestCandidates(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"example.com", []string{"example.com"}},
		{"mx.example.com", []string{"mx.example.com", "example.com"}},
		{"a.b.example.com", []string{"a.b.example.com", "b.example.com", "example.com"}},
		{"localhost", []string{"localhost"}},
	}
	for _, tt := range cases {
		require.Equal(t, tt.want, Candidates(tt.in))
	}
}

func TestDomainOfEmail(t *testing.T) {
	valid := []struct{ in, want string }{
		{"user@example.com", "example.com"},
		{"User@Example.COM", "example.com"},
		{"a+b@sub.example.com", "sub.example.com"},
	}
	for _, tt := range valid {
		got, err := DomainOfEmail(tt.in)
		require.NoError(t, err)
		require.Equal(t, tt.want, got)
	}

	invalid := []string{"", "no-at", "user@", "@example.com", "example.com"}
	for _, in := range invalid {
		_, err := DomainOfEmail(in)
		require.ErrorIs(t, err, ErrInvalidEmail)
	}
}
