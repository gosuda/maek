package server

import (
	"embed"
	"io/fs"
)

//go:embed static/*
var staticFS embed.FS

// GetStaticFS returns the sub-filesystem rooted at "static".
func GetStaticFS() (fs.FS, error) {
	return fs.Sub(staticFS, "static")
}

// GetStaticFile returns the raw bytes of a file inside static/.
func GetStaticFile(name string) ([]byte, error) {
	return staticFS.ReadFile("static/" + name)
}
