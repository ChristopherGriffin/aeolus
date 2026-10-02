// Package ui serves the web UI (0042): plain HTML, CSS and JavaScript
// modules embedded in the binary, with no build step. The UI reads and
// changes the manager only through the API, with the person's own token.
package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed static
var static embed.FS

// csp lets the page load scripts, styles and data only from the manager
// itself, and keeps it out of frames (0042).
const csp = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

// types are the only kinds of file the UI ships. A fixed table, because
// mime.TypeByExtension reads the host's settings, and some map .js to
// text/plain, which browsers refuse to run as a module.
var types = map[string]string{
	".html": "text/html; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".js":   "text/javascript; charset=utf-8",
	".svg":  "image/svg+xml",
}

// Handler serves the UI: the page at / to browsers, its files under /ui/.
// Every other request, and / asked for as JSON, goes to next.
func Handler(next http.Handler) http.Handler {
	files, err := fs.Sub(static, "static")
	if err != nil {
		panic(err) // the embedded directory is part of the build
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/" && wantsHTML(r):
			serve(w, r, files, "index.html")
		case strings.HasPrefix(r.URL.Path, "/ui/"):
			serve(w, r, files, strings.TrimPrefix(r.URL.Path, "/ui/"))
		default:
			next.ServeHTTP(w, r)
		}
	})
}

func wantsHTML(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

// serve writes one embedded file. There are no directory listings.
func serve(w http.ResponseWriter, r *http.Request, files fs.FS, name string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name = path.Clean("/" + name)[1:]
	body, err := fs.ReadFile(files, name)
	if err != nil || name == "" {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Security-Policy", csp)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Cache-Control", "no-cache")
	if t, ok := types[path.Ext(name)]; ok {
		h.Set("Content-Type", t)
	}
	if r.Method == http.MethodGet {
		w.Write(body)
	}
}
