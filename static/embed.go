// Package static embeds the front-end assets (CSS, vendored HTMX, the PWA
// manifest, the Service Worker, and icons) so the server binary serves them
// without the files present on disk.
package static

import "embed"

//go:embed css js icons sw.js manifest.webmanifest
var FS embed.FS
