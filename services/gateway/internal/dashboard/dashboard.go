package dashboard

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed web/*
var files embed.FS

func Handler() http.Handler {
	web, err := fs.Sub(files, "web")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(web))
	index, err := fs.ReadFile(web, "index.html")
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		setHeaders(w)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(index)
	})
	mux.HandleFunc("GET /assets/{asset}", func(w http.ResponseWriter, r *http.Request) {
		asset := path.Clean(r.PathValue("asset"))
		if asset == "." || strings.Contains(asset, "/") {
			http.NotFound(w, r)
			return
		}
		serveFile(w, r, fileServer, asset)
	})
	return mux
}

func serveFile(w http.ResponseWriter, r *http.Request, files http.Handler, name string) {
	setHeaders(w)
	r.URL.Path = "/" + name
	files.ServeHTTP(w, r)
}

func setHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'self'; frame-ancestors 'none'; object-src 'none'")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
}
