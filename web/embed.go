// Package web embeds the browser frontend.
package web

import "embed"

//go:embed static
var Files embed.FS
