package server

import (
	"io"
	"io/fs"
	"net/http"
	"strings"
)

func (s *Server) handleDashboard() http.Handler {
	var fileSystem http.FileSystem

	if s.dashboardCfg.DevMode && s.dashboardCfg.StaticDir != "" {
		fileSystem = http.Dir(s.dashboardCfg.StaticDir)
	} else {
		sub, err := fs.Sub(s.dashboardFS, ".")
		if err != nil {
			s.logger.Error("failed to create sub FS for dashboard")
			return http.NotFoundHandler()
		}
		fileSystem = http.FS(sub)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip /dashboard prefix.
		path := strings.TrimPrefix(r.URL.Path, "/dashboard")
		if path == "" || path == "/" {
			path = "/index.html"
		}

		// Try to open the requested file.
		f, err := fileSystem.Open(path)
		if err != nil {
			// SPA fallback: serve index.html for unknown paths.
			path = "/index.html"
			f, err = fileSystem.Open(path)
			if err != nil {
				http.NotFound(w, r)
				return
			}
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil {
			http.NotFound(w, r)
			return
		}

		// If it's a directory, try index.html inside it.
		if stat.IsDir() {
			f.Close()
			f, err = fileSystem.Open(path + "/index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			defer f.Close()
			stat, _ = f.Stat()
		}

		// Detect content type from extension.
		if strings.HasSuffix(path, ".html") {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		} else if strings.HasSuffix(path, ".js") {
			w.Header().Set("Content-Type", "application/javascript")
		} else if strings.HasSuffix(path, ".css") {
			w.Header().Set("Content-Type", "text/css")
		} else if strings.HasSuffix(path, ".woff2") {
			w.Header().Set("Content-Type", "font/woff2")
		} else if strings.HasSuffix(path, ".woff") {
			w.Header().Set("Content-Type", "font/woff")
		}

		http.ServeContent(w, r, stat.Name(), stat.ModTime(), f.(io.ReadSeeker))
	})
}
