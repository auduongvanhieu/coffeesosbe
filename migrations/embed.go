// Package migrations embeds the SQL migration files so the API binary can
// migrate the database on its own (no external migrate CLI needed).
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
