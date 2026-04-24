package migrations

import "embed"

// FS contains embedded schema migrations.
//
//go:embed *.sql
var FS embed.FS
