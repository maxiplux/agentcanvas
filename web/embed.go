// Package web embeds the studio UI (index.html) and its vendored assets.
package web

import "embed"

// FS holds index.html and vendor/.
//
//go:embed index.html vendor
var FS embed.FS
