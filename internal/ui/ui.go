package ui

import (
	"embed"
	"io/fs"
)

//go:embed assets
var files embed.FS

// FS returns the embedded web assets rooted at the assets directory.
func FS() fs.FS {
	sub, err := fs.Sub(files, "assets")
	if err != nil {
		panic(err)
	}
	return sub
}
