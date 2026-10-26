// Package web embeds the built SPA into the binary.
package web

import "embed"

// Assets holds web/dist (index.html + assets).
//
//go:embed all:dist
var Assets embed.FS
