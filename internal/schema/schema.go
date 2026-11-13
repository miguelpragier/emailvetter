// Package schema embeds the SQL schema and the curated disposable-domain seed
// list applied by the store on first run.
package schema

import _ "embed"

// SQL creates the tables idempotently. It contains two statements separated by
// ";"; no statement contains a semicolon inside a string literal.
//
//go:embed schema.sql
var SQL string

// SeedDomains is the curated disposable-domain list, one domain per line. The
// store applies it exactly once (gated by an app_meta marker), so a domain an
// operator deletes stays deleted.
//
//go:embed seed_domains.txt
var SeedDomains string
