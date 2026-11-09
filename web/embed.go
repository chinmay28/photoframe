// Package web embeds the progressive web app served by the frame.
package web

import (
	"embed"
	"io/fs"
)

//go:embed index.html app.css app.js sw.js manifest.webmanifest icons
var files embed.FS

// FS returns the app's static files.
func FS() fs.FS { return files }
