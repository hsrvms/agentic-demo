// Package web holds embedded static assets for the web application.
package web

import "embed"

// StaticFS contains the embedded static files from web/static/.
//
//go:embed all:static
var StaticFS embed.FS
