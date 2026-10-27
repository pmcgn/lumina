// Package web embeds the built frontend (frontend/ builds into ./dist).
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// BuildID identifies the embedded frontend build: a short hash of index.html,
// which references the content-hashed asset files. Empty if there is none.
func BuildID(fsys fs.FS) string {
	b, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:4])
}

// FS returns the frontend files with "dist" as root.
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
