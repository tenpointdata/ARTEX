//go:build embedui

package server

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// webuiDist holds the statically-exported frontend (built with
// `cd web && npm run build:static`, then copied into server/webui/dist). Compiled
// into the binary only when building with `-tags embedui`. The `all:` prefix is
// required so Next's `_next/` asset dir (leading underscore) is included.
//
//go:embed all:webui/dist
var webuiDist embed.FS

// webuiHandler serves the embedded SPA. Public (no JWT) — auth is enforced
// client-side and on /api. Serves the exported per-route index.html files and
// falls back to index.html so client-side routing still resolves unknown paths.
//
// Responses carry an ETag and, for the content-hashed Next chunks, immutable
// caching; compressible bodies are gzipped when the client accepts it. See
// webui_assets.go for why (embed.FS has no mtime, so the previous
// http.ServeFileFS path emitted no validator at all and re-sent ~1.5 MB of
// assets on every load).
func (s *Server) webuiHandler() http.Handler {
	root, err := fs.Sub(webuiDist, "webui/dist")
	if err != nil {
		panic(err)
	}
	// serve sets the per-document CSP (inline-script hashes) for HTML before
	// handing the file to serveWebAsset.
	serve := func(w http.ResponseWriter, r *http.Request, name string, immutable bool) bool {
		if e, err := loadAsset(root, name); err == nil && strings.HasPrefix(e.ctype, "text/html") {
			w.Header().Set("Content-Security-Policy", documentCSP(name, e.raw))
		}
		return serveWebAsset(w, r, root, name, immutable)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		// 1) exact file (assets: _next/*, favicon.ico, ...)
		// 2) route dir → <p>/index.html (trailingSlash export)
		// 3) <p>.html
		// 4) SPA fallback → index.html (hand it to client-side routing)
		for _, cand := range []string{p, p + "/index.html", p + ".html"} {
			if serve(w, r, cand, webuiImmutable(cand)) {
				return
			}
		}
		if !serve(w, r, "index.html", false) {
			http.NotFound(w, r)
		}
	})
}
