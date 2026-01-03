//go:build !adminui

package api

import "io/fs"

func embeddedAdminUI() (fs.FS, bool) {
	return nil, false
}
