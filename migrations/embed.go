// Package migrations embeds the SQL migration files so the server ships as a
// single binary while keeping the SQL as plain, reviewable files on disk.
package migrations

import "embed"

// FS contains the .up.sql and .down.sql migration files.
//
//go:embed *.sql
var FS embed.FS
