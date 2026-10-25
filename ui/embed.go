// Package ui carries the built console assets. The Vite build writes dist
// (run `make ui` to regenerate), dist is committed so `go build ./...` stays
// sufficient without Node, and the serve command embeds it from here.
package ui

import "embed"

// Dist is the built console: index.html and the hashed assets under dist.
//
//go:embed all:dist
var Dist embed.FS
