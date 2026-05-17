package adminapi

import (
	"embed"
	"io/fs"
	"net/http"
	"time"
)

//go:embed embedded_admin/*
var embeddedAdminFS embed.FS

func embeddedAdminFrontend() (*adminFrontend, error) {
	root, err := fs.Sub(embeddedAdminFS, "embedded_admin")
	if err != nil {
		return nil, err
	}
	indexBytes, err := fs.ReadFile(root, "index.html")
	if err != nil {
		return nil, err
	}
	return &adminFrontend{indexBytes: indexBytes, indexModAt: time.Unix(0, 0).UTC(), fileSystem: http.FS(root)}, nil
}
