// Package web embeds the schedule manager page.
package web

import "embed"

// FS holds the admin page assets.
//
//go:embed index.html app.js style.css
var FS embed.FS
