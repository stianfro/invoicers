package web

import "embed"

// Templates contains all embedded HTML templates used by the app.
//
//go:embed templates/*.html
var Templates embed.FS
