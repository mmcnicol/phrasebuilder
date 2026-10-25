// Package web serves Phrasebuilder's pages, htmx partials and static files.
// Templates and static files are embedded, so the server binary is
// self-contained.
package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

//go:embed templates static
var files embed.FS

// Config holds settings the handlers need.
type Config struct {
	// Revision identifies the running build, e.g. Cloud Run's K_REVISION.
	Revision string
}

type server struct {
	cfg   Config
	pages map[string]*template.Template
}

// New returns the app's HTTP handler.
func New(cfg Config) (http.Handler, error) {
	pages, err := parsePages(files, "home")
	if err != nil {
		return nil, err
	}
	s := &server{cfg: cfg, pages: pages}

	static, err := fs.Sub(files, "static")
	if err != nil {
		return nil, err
	}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer, securityHeaders)
	r.Get("/", s.home)
	r.Get("/greeting", s.greeting)
	// Not /healthz: Cloud Run reserves some paths ending in z.
	r.Get("/health", s.health)
	r.Handle("/static/*", http.StripPrefix("/static/", cacheStatic(http.FileServerFS(static))))
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not found", http.StatusNotFound)
	})
	return r, nil
}

// parsePages builds one template set per page, each combining layout.html
// with templates/<page>.html, so every page can define its own "content".
func parsePages(fsys fs.FS, names ...string) (map[string]*template.Template, error) {
	pages := make(map[string]*template.Template, len(names))
	for _, name := range names {
		t, err := template.ParseFS(fsys, "templates/layout.html", "templates/"+name+".html")
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		pages[name] = t
	}
	return pages, nil
}

func (s *server) render(w http.ResponseWriter, page string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.pages[page].ExecuteTemplate(w, "layout", data); err != nil {
		slog.Error("render", "page", page, "err", err)
	}
}

func (s *server) home(w http.ResponseWriter, r *http.Request) {
	s.render(w, "home", nil)
}

// greeting is an htmx partial: it returns an HTML fragment, not a page.
func (s *server) greeting(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<p lang="fr"><strong>Bonjour !</strong> <span lang="en">(Hello!)</span></p>`)
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintf(w, "ok %s\n", s.cfg.Revision)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

func cacheStatic(next http.Handler) http.Handler {
	maxAge := fmt.Sprintf("public, max-age=%d", int((24 * time.Hour).Seconds()))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", maxAge)
		next.ServeHTTP(w, r)
	})
}
