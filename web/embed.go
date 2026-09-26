package web

import "embed"

// FS holds the embedded static web assets (favicon.svg, plus any UI files
// you add later). Served by internal/api.
//
//go:embed favicon.svg
var FS embed.FS
