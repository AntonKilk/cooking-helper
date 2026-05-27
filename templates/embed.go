// Package templates embeds the html/template files so the server binary can
// render pages without the template files present on disk.
package templates

import "embed"

//go:embed *.gohtml
var FS embed.FS
