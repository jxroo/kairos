package dashboard

import "embed"

// Assets contains the built dashboard static files.
//
//go:embed all:dist
var Assets embed.FS
