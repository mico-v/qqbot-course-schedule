// Package web embeds the built admin SPA.
package web

import "embed"

// FS holds the Vite build output (web/dist): index.html plus hashed assets.
//
//go:embed all:dist
var FS embed.FS
