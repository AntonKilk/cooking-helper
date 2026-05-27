// Package migrations embeds the golang-migrate SQL files so the server binary
// can apply the schema on startup without the files present on disk.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
