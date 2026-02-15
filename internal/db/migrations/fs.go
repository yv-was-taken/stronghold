// Package migrations provides embedded SQL migration files for Stronghold.
package migrations

import (
	"embed"
	"io/fs"
)

//go:embed *.sql
var sqlFiles embed.FS

// FS returns the embedded SQL migration files.
func FS() fs.FS {
	return sqlFiles
}
