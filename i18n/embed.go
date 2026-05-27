// Package i18n embeds the RU/FI/EN JSON dictionaries so the server binary can
// load translations on startup without the files present on disk.
package i18n

import "embed"

//go:embed *.json
var FS embed.FS
