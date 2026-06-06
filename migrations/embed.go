package migrations

import "embed"

// FS contains the SQL migrations shipped with the Cartolensia binary.
//
//go:embed *.sql
var FS embed.FS
