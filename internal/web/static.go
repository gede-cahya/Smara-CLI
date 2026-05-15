package web

import (
	"bytes"
	"compress/gzip"
	"embed"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// distPath returns the absolute path to web/dist, resolving relative to the binary.
func distPath() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "web/dist"
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "web", "dist")
}

//go:embed all:dist
var distFS embed.FS

// staticFiles holds the SPA static files (index.html, JS, CSS) in memory.
// During development they are served from disk; production builds embed them.
var staticFiles = map[string][]byte{}
var staticGzip = map[string][]byte{}

func init() {
	// Pre-load embedded dist/ files into memory map for fast serving
	entries, err := distFS.ReadDir("dist")
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			subEntries, _ := distFS.ReadDir("dist/" + entry.Name())
			for _, sub := range subEntries {
				if sub.IsDir() {
					continue
				}
				name := entry.Name() + "/" + sub.Name()
				data, _ := distFS.ReadFile("dist/" + name)
				if len(data) > 0 {
					staticFiles[name] = data
				}
			}
			continue
		}
		data, _ := distFS.ReadFile("dist/" + entry.Name())
		if len(data) > 0 {
			staticFiles[entry.Name()] = data
		}
	}
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	// Never serve SPA fallback for API or WebSocket paths
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws") {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("Not found"))
		return
	}

	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "" {
		p = "index.html"
	}

	// Try serving from disk first (development) so frontend rebuilds are picked up immediately
	dp := distPath()
	diskPath := filepath.Join(dp, p)
	if data, err := os.ReadFile(diskPath); err == nil {
		serveData(w, r, p, data, false)
		return
	}

	// SPA client-side routing — try index.html on disk for unknown paths
	if p != "index.html" {
		if data, err := os.ReadFile(filepath.Join(dp, "index.html")); err == nil {
			serveData(w, r, "index.html", data, false)
			return
		}
	}

	// Fallback: embedded files (production / release binary)
	if data, ok := staticFiles[p]; ok {
		serveData(w, r, p, data, false)
		return
	}
	if data, ok := staticGzip[p]; ok {
		serveData(w, r, p, data, true)
		return
	}

	// Fallback: SPA client-side routing from embedded
	if data, ok := staticFiles["index.html"]; ok {
		serveData(w, r, "index.html", data, false)
		return
	}
	if data, ok := staticGzip["index.html"]; ok {
		serveData(w, r, "index.html", data, true)
		return
	}

	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte("Smara Web Interface is not built. Run 'make web' to build the frontend."))
}

func serveData(w http.ResponseWriter, r *http.Request, filename string, data []byte, gzipped bool) {
	ct := mime.TypeByExtension(path.Ext(filename))
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	if gzipped {
		w.Header().Set("Content-Encoding", "gzip")
		gr, err := gzip.NewReader(bytes.NewReader(data))
		if err == nil {
			plain, _ := io.ReadAll(gr)
			data = plain
			w.Header().Del("Content-Encoding")
		}
	}
	// HTML must NEVER be cached — it carries the script tags pointing at
	// hashed asset filenames. If the browser caches HTML, it keeps loading
	// the old asset hashes after a rebuild and never picks up new code.
	// Hashed assets themselves are immutable, so cache them aggressively.
	if strings.HasSuffix(filename, ".html") || filename == "index.html" {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// RegisterStatic registers a static file for in-memory serving.
func RegisterStatic(filename string, data []byte) {
	staticFiles[filename] = data
}
