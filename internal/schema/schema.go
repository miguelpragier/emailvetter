// Package schema embeds the SQL schema applied by the store on startup.
package schema

import _ "embed"

// SQL creates the tables idempotently. It contains two statements separated by
// ";"; no statement contains a semicolon inside a string literal.
//
//go:embed schema.sql
var SQL string
