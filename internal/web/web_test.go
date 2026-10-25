package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func get(t *testing.T, path string) (*http.Response, string) {
	t.Helper()
	h, err := New(Config{Revision: "test-rev"})
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
		{"/nope", http.StatusNotFound, "text/plain", "Not found"},
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
	if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("Content-Security-Policy = %q", csp)
	}
	if got := res.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q", got)
	}
}
