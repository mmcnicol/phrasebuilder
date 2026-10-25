package web

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mmcnicol/phrasebuilder/internal/store"
)

type fakeStore struct{ err error }

func (f fakeStore) Units(context.Context) ([]store.Unit, error) {
	return []store.Unit{{ID: "fr/cafe", Language: "fr", Position: 1, Title: "At the café", PatternCount: 1}}, f.err
}

func (f fakeStore) Unit(_ context.Context, id string) (store.Unit, []store.Pattern, error) {
	if f.err != nil {
		return store.Unit{}, nil, f.err
	}
	if id != "fr/cafe" {
		return store.Unit{}, nil, store.ErrNotFound
	}
	return store.Unit{ID: "fr/cafe", Language: "fr", Position: 1, Title: "At the café", PatternCount: 1},
		[]store.Pattern{{
			ID: "fr/je-voudrais-noun", Frame: "Je voudrais {item}, s'il vous plaît.", En: "I would like {item}, please.",
			Note: "Je voudrais = I would like.", Status: "draft",
			Sentences: []store.Sentence{{ID: "fr/je-voudrais-noun/un-cafe", Text: "Je voudrais un café, s'il vous plaît.", En: "I would like a coffee, please.", AudioPath: "fr/0123456789abcdef.mp3"}},
		}}, nil
}

var testConfig = Config{Revision: "test-rev", Store: fakeStore{}, AudioBaseURL: "https://storage.googleapis.com/phrasebuilder-audio/"}

func get(t *testing.T, path string) (*http.Response, string) {
	t.Helper()
	return getWith(t, testConfig, path)
}

func getWith(t *testing.T, cfg Config, path string) (*http.Response, string) {
	t.Helper()
	h, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	res := rec.Result()
	body, _ := io.ReadAll(res.Body)
	return res, string(body)
}

func TestRoutes(t *testing.T) {
	tests := []struct {
		path        string
		status      int
		contentType string
		contains    string
	}{
		{"/", http.StatusOK, "text/html", `hx-get="/greeting"`},
		{"/greeting", http.StatusOK, "text/html", "Bonjour"},
		{"/health", http.StatusOK, "text/plain", "ok test-rev"},
		{"/static/htmx.min.js", http.StatusOK, "javascript", "htmx"},
		{"/static/style.css", http.StatusOK, "text/css", "--accent"},
		{"/static/app.js", http.StatusOK, "javascript", "data-audio"},
		{"/nope", http.StatusNotFound, "text/html", "Page not found."},
		{"/units", http.StatusOK, "text/html", `<a href="/units/fr/cafe">At the café</a>`},
		{"/units/fr/cafe", http.StatusOK, "text/html", "Je voudrais un café, s&#39;il vous plaît."},
		{"/units/fr/nope", http.StatusNotFound, "text/html", "There is no such unit."},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			res, body := get(t, tt.path)
			if res.StatusCode != tt.status {
				t.Errorf("status = %d, want %d", res.StatusCode, tt.status)
			}
			if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, tt.contentType) {
				t.Errorf("Content-Type = %q, want %q", ct, tt.contentType)
			}
			if !strings.Contains(body, tt.contains) {
				t.Errorf("body does not contain %q", tt.contains)
			}
		})
	}
}

func TestHomeUsesLayout(t *testing.T) {
	_, body := get(t, "/")
	for _, want := range []string{"<!doctype html>", "<title>Phrasebuilder</title>", "/static/htmx.min.js", "Learn French"} {
		if !strings.Contains(body, want) {
			t.Errorf("home page missing %q", want)
		}
	}
}

func TestSecurityHeaders(t *testing.T) {
	res, _ := get(t, "/")
	if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") || !strings.Contains(csp, "media-src 'self' https://storage.googleapis.com;") {
		t.Errorf("Content-Security-Policy = %q", csp)
	}
	if got := res.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q", got)
	}
}

func TestUnitPage(t *testing.T) {
	_, body := get(t, "/units/fr/cafe")
	for _, want := range []string{
		"<title>At the café · Phrasebuilder</title>",
		`Je voudrais <span class="slot">…</span>, s&#39;il vous plaît.`,
		`data-audio="https://storage.googleapis.com/phrasebuilder-audio/fr/0123456789abcdef.mp3"`,
		`<span class="badge"`,
		"Je voudrais = I would like.",
		`<script src="/static/app.js" defer></script>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("unit page missing %q", want)
		}
	}
}

func TestNoAudioBaseURL(t *testing.T) {
	cfg := testConfig
	cfg.AudioBaseURL = ""
	res, body := getWith(t, cfg, "/units/fr/cafe")
	if strings.Contains(body, "data-audio") {
		t.Error("play button shown without AUDIO_BASE_URL")
	}
	if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "media-src 'self';") {
		t.Errorf("Content-Security-Policy = %q", csp)
	}
}

func TestStoreUnavailable(t *testing.T) {
	cfg := testConfig
	cfg.Store = nil
	for _, path := range []string{"/units", "/units/fr/cafe"} {
		if res, _ := getWith(t, cfg, path); res.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("%s without a store: status %d, want 503", path, res.StatusCode)
		}
	}
	cfg.Store = fakeStore{err: errors.New("connection refused")}
	for _, path := range []string{"/units", "/units/fr/cafe"} {
		if res, body := getWith(t, cfg, path); res.StatusCode != http.StatusInternalServerError || strings.Contains(body, "connection refused") {
			t.Errorf("%s with a failing store: status %d; body must not leak the error", path, res.StatusCode)
		}
	}
}
