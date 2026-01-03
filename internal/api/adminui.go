package api

import (
	"io/fs"
	"net/http"
	"os"
	"strings"
)

func newAdminUIHandler(dir string) http.Handler {
	var webfs fs.FS
	if dir = strings.TrimSpace(dir); dir != "" {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			webfs = os.DirFS(dir)
		}
	} else if embedded, ok := embeddedAdminUI(); ok {
		webfs = embedded
	}

	if webfs == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		})
	}

	files := http.FileServer(http.FS(webfs))
	stripped := http.StripPrefix("/admin", files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		stripped.ServeHTTP(&locationPrefixWriter{ResponseWriter: w, prefix: "/admin"}, r)
	})
}

type locationPrefixWriter struct {
	http.ResponseWriter
	prefix string
	wrote  bool
}

func (w *locationPrefixWriter) WriteHeader(code int) {
	if !w.wrote {
		w.wrote = true
		loc := strings.TrimSpace(w.Header().Get("Location"))
		if strings.HasPrefix(loc, "/") {
			if !strings.HasPrefix(loc, w.prefix+"/") && loc != w.prefix {
				w.Header().Set("Location", w.prefix+loc)
			}
		} else if loc != "" && !strings.Contains(loc, "://") {
			w.Header().Set("Location", w.prefix+"/"+loc)
		}
	}
	w.ResponseWriter.WriteHeader(code)
}
