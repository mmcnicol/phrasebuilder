// Package web serves Phrasebuilder's pages, htmx partials and static files.
// Templates and static files are embedded, so the server binary is
// self-contained.
package web

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/mmcnicol/phrasebuilder/internal/content"
	"github.com/mmcnicol/phrasebuilder/internal/store"
)

//go:embed templates static
var files embed.FS

// ContentStore reads course content. *store.Store implements it.
type ContentStore interface {
	Units(ctx context.Context) ([]store.Unit, error)
	Unit(ctx context.Context, id string) (store.Unit, []store.Pattern, error)
}

// Config holds settings the handlers need.
type Config struct {
	// Revision identifies the running build, e.g. Cloud Run's K_REVISION.
	Revision string
	// Store holds the course content. If nil, content pages return 503.
	Store ContentStore
	// AudioBaseURL is where audio files are served from, e.g.
	// https://storage.googleapis.com/phrasebuilder-audio. If empty, pages
	// have no play buttons.
	AudioBaseURL string
}

type server struct {
	cfg   Config
	pages map[string]*template.Template
}

// New returns the app's HTTP handler.
func New(cfg Config) (http.Handler, error) {
	cfg.AudioBaseURL = strings.TrimRight(cfg.AudioBaseURL, "/")
	s := &server{cfg: cfg}
	pages, err := parsePages(files, s.funcs(), "home", "units", "unit", "error")
	if err != nil {
		return nil, err
	}
	s.pages = pages

	static, err := fs.Sub(files, "static")
	if err != nil {
		return nil, err
	}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer, securityHeaders(cfg.AudioBaseURL))
	r.Get("/", s.home)
	r.Get("/greeting", s.greeting)
	r.Get("/units", s.units)
	r.Get("/units/{lang}/{unit}", s.unit)
	// Not /healthz: Cloud Run reserves some paths ending in z.
	r.Get("/health", s.health)
	r.Handle("/static/*", http.StripPrefix("/static/", cacheStatic(http.FileServerFS(static))))
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		s.error(w, http.StatusNotFound, "Page not found.")
	})
	return r, nil
}

// parsePages builds one template set per page, each combining layout.html
// with templates/<page>.html, so every page can define its own "content".
func parsePages(fsys fs.FS, funcs template.FuncMap, names ...string) (map[string]*template.Template, error) {
	pages := make(map[string]*template.Template, len(names))
	for _, name := range names {
		t, err := template.New("").Funcs(funcs).ParseFS(fsys, "templates/layout.html", "templates/"+name+".html")
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		pages[name] = t
	}
	return pages, nil
}

var slotRE = regexp.MustCompile(`\{([a-z0-9-]+)\}`)

func (s *server) funcs() template.FuncMap {
	return template.FuncMap{
		// audioURL is the URL of an audio file, or "" if there is no audio.
		"audioURL": func(path string) string {
			if s.cfg.AudioBaseURL == "" || path == "" {
				return ""
			}
			return s.cfg.AudioBaseURL + "/" + path
		},
		// frame shows a pattern frame with its {slots} highlighted.
		"frame": func(f string) template.HTML {
			return template.HTML(slotRE.ReplaceAllString(html.EscapeString(f), `<span class="slot">…</span>`))
		},
		"languageName": func(code string) string { return content.Languages[code] },
	}
}

func (s *server) render(w http.ResponseWriter, status int, page string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := s.pages[page].ExecuteTemplate(w, "layout", data); err != nil {
		slog.Error("render", "page", page, "err", err)
	}
}

func (s *server) error(w http.ResponseWriter, status int, msg string) {
	s.render(w, status, "error", map[string]any{"Status": status, "Message": msg})
}

func (s *server) home(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusOK, "home", nil)
}

// greeting is an htmx partial: it returns an HTML fragment, not a page.
func (s *server) greeting(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<p lang="fr"><strong>Bonjour !</strong> <span lang="en">(Hello!)</span></p>`)
}

func (s *server) units(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Store == nil {
		s.error(w, http.StatusServiceUnavailable, "No database is configured.")
		return
	}
	units, err := s.cfg.Store.Units(r.Context())
	if err != nil {
		slog.Error("list units", "err", err)
		s.error(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}
	s.render(w, http.StatusOK, "units", map[string]any{"Units": units})
}

func (s *server) unit(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Store == nil {
		s.error(w, http.StatusServiceUnavailable, "No database is configured.")
		return
	}
	id := chi.URLParam(r, "lang") + "/" + chi.URLParam(r, "unit")
	u, patterns, err := s.cfg.Store.Unit(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		s.error(w, http.StatusNotFound, "There is no such unit.")
		return
	}
	if err != nil {
		slog.Error("get unit", "unit", id, "err", err)
		s.error(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}
	s.render(w, http.StatusOK, "unit", map[string]any{"Unit": u, "Patterns": patterns})
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintf(w, "ok %s\n", s.cfg.Revision)
}

// securityHeaders allows scripts, styles and media from this site only,
// plus audio from audioBaseURL's origin.
func securityHeaders(audioBaseURL string) func(http.Handler) http.Handler {
	media := "'self'"
	if u, err := url.Parse(audioBaseURL); err == nil && u.Scheme != "" && u.Host != "" {
		media += " " + u.Scheme + "://" + u.Host
	}
	csp := "default-src 'self'; media-src " + media + "; frame-ancestors 'none'"
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Content-Security-Policy", csp)
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			next.ServeHTTP(w, r)
		})
	}
}

func cacheStatic(next http.Handler) http.Handler {
	maxAge := fmt.Sprintf("public, max-age=%d", int((24 * time.Hour).Seconds()))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", maxAge)
		next.ServeHTTP(w, r)
	})
}
