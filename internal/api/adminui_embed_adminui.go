//go:build adminui

package api

import (
	"embed"
	"io/fs"
)

//go:embed adminui_dist/**
var embeddedAdminUIFS embed.FS

func embeddedAdminUI() (fs.FS, bool) {
	sub, err := fs.Sub(embeddedAdminUIFS, "adminui_dist")
	if err != nil {
		return nil, false
	}
	return sub, true
}

