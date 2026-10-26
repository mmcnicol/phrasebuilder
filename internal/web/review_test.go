package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/open-spaced-repetition/go-fsrs/v4"

	"github.com/mmcnicol/phrasebuilder/internal/srs"
	"github.com/mmcnicol/phrasebuilder/internal/store"
)

const passphrase = "correct horse battery staple"

var clock = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

// fakeReviews has one new sentence, or nothing left once answered.
type fakeReviews struct {
	answers []store.Answer
	undos   int
	reports []string
}

func (f *fakeReviews) Snapshot(_ context.Context, user int64, now time.Time) (srs.Snapshot, error) {
	if len(f.answers) > 0 {
		return srs.Snapshot{Now: now, NewToday: 1}, nil
	}
	return srs.Snapshot{Now: now, New: []srs.NewSentence{{SentenceID: "fr/je-voudrais-noun/un-cafe", PatternID: "fr/je-voudrais-noun"}}}, nil
}

func (f *fakeReviews) Item(_ context.Context, user int64, id string, ex srs.Exercise, now time.Time) (store.ReviewItem, error) {
	return store.ReviewItem{
		Card: srs.Card{SentenceID: id, Exercise: ex, Card: fsrs.NewCard(now)},
		Text: "Je voudrais un café, s'il vous plaît.", En: "I would like a coffee, please.",
		AudioPath: "fr/0123456789abcdef.mp3", Language: "fr", UnitID: "fr/cafe",
		PatternID: "fr/je-voudrais-noun", Frame: "Je voudrais {item}, s'il vous plaît.",
		PatternEn: "I would like {item}, please.", Note: "Je voudrais = I would like.", Status: "draft",
		NewPattern: true,
	}, nil
}

func (f *fakeReviews) Answer(_ context.Context, a store.Answer) (bool, error) {
	f.answers = append(f.answers, a)
	return true, nil
}

func (f *fakeReviews) Undo(context.Context, int64, time.Time) (store.Undone, bool, error) {
	f.undos++
	f.answers = nil
	return store.Undone{SentenceID: "fr/je-voudrais-noun/un-cafe", Exercise: srs.Recognise}, true, nil
}

func (f *fakeReviews) Today(context.Context, int64, time.Time) (store.Today, error) {
	return store.Today{Reviews: 7, NewSentences: 1, Elapsed: 90 * time.Second, NextDue: clock.Add(10 * time.Minute)}, nil
}

func (f *fakeReviews) AddReport(_ context.Context, _ int64, id, note string) error {
	f.reports = append(f.reports, id+": "+note)
	return nil
}

func (f *fakeReviews) OpenReports(context.Context) ([]store.Report, error) {
	return []store.Report{{ID: 3, Note: "Typo", Created: clock, SentenceID: "fr/x", Text: "Bonjour.", En: "Hello.", Unit: "fr/cafe"}}, nil
}

func (f *fakeReviews) ResolveReport(context.Context, int64) error { return nil }

type app struct {
	t       *testing.T
	h       http.Handler
	reviews *fakeReviews
	now     time.Time
}

func newApp(t *testing.T) *app {
	t.Helper()
	a := &app{t: t, reviews: &fakeReviews{}, now: clock}
	cfg := testConfig
	cfg.Reviews, cfg.Passphrase, cfg.SessionKey = a.reviews, passphrase, []byte("0123456789abcdef0123456789abcdef")
	cfg.Now = func() time.Time { return a.now }
	h, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a.h = h
	return a
}

// do sends a request with optional form values and cookie; htmx marks it
// as an htmx request.
func (a *app) do(method, path string, form url.Values, cookie *http.Cookie, htmx bool) (*http.Response, string) {
	a.t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	a.h.ServeHTTP(rec, req)
	res := rec.Result()
	b, _ := io.ReadAll(res.Body)
	return res, string(b)
}

func (a *app) login() *http.Cookie {
	a.t.Helper()
	res, _ := a.do("POST", "/login", url.Values{"passphrase": {passphrase}, "next": {"/review"}}, nil, false)
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/review" {
		a.t.Fatalf("login: status %d, Location %q", res.StatusCode, res.Header.Get("Location"))
	}
	for _, c := range res.Cookies() {
		if c.Name == sessionCookie {
			if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.MaxAge < 364*24*3600 {
				a.t.Errorf("session cookie %+v", c)
			}
			return c
		}
	}
	a.t.Fatal("login set no session cookie")
	return nil
}

func TestProtectedRoutes(t *testing.T) {
	a := newApp(t)
	for _, path := range []string{"/review", "/reports"} {
		res, _ := a.do("GET", path, nil, nil, false)
		if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/login?next="+url.QueryEscape(path) {
			t.Errorf("GET %s logged out: %d → %q", path, res.StatusCode, res.Header.Get("Location"))
		}
	}
	for _, path := range []string{"/review/answer", "/review/undo", "/reports", "/reports/3/resolve"} {
		res, _ := a.do("POST", path, url.Values{}, nil, true)
		if res.StatusCode != http.StatusUnauthorized || res.Header.Get("HX-Redirect") != "/login" {
			t.Errorf("POST %s logged out: %d, HX-Redirect %q", path, res.StatusCode, res.Header.Get("HX-Redirect"))
		}
	}
	if len(a.reviews.answers)+a.reviews.undos+len(a.reviews.reports) > 0 {
		t.Error("a logged-out request reached the store")
	}
}

func TestLogin(t *testing.T) {
	a := newApp(t)
	if res, body := a.do("GET", "/login?next=/reports", nil, nil, false); res.StatusCode != 200 || !strings.Contains(body, `name="next" value="/reports"`) {
		t.Errorf("login form: %d", res.StatusCode)
	}
	res, body := a.do("POST", "/login", url.Values{"passphrase": {"wrong"}}, nil, false)
	if res.StatusCode != http.StatusUnauthorized || !strings.Contains(body, "not right") || len(res.Cookies()) != 0 {
		t.Errorf("wrong passphrase: %d", res.StatusCode)
	}

	c := a.login()
	if res, _ := a.do("GET", "/review", nil, c, false); res.StatusCode != 200 || res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("review with cookie: %d, Cache-Control %q", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	if res, _ := a.do("GET", "/login", nil, c, false); res.StatusCode != http.StatusSeeOther {
		t.Errorf("login page when logged in: %d, want a redirect", res.StatusCode)
	}

	for name, value := range map[string]string{
		"tampered user": "2" + c.Value[1:],
		"no signature":  c.Value[:strings.LastIndexByte(c.Value, '.')],
		"garbage":       "nope",
	} {
		bad := &http.Cookie{Name: sessionCookie, Value: value}
		if res, _ := a.do("GET", "/review", nil, bad, false); res.StatusCode != http.StatusSeeOther {
			t.Errorf("%s cookie: %d, want redirect to login", name, res.StatusCode)
		}
	}

	a.now = clock.Add(sessionLength + time.Minute)
	if res, _ := a.do("GET", "/review", nil, c, false); res.StatusCode != http.StatusSeeOther {
		t.Errorf("expired cookie: %d, want redirect to login", res.StatusCode)
	}

	res, _ = a.do("POST", "/logout", url.Values{}, c, false)
	if len(res.Cookies()) != 1 || res.Cookies()[0].MaxAge >= 0 {
		t.Errorf("logout cookies = %+v", res.Cookies())
	}
}

func TestLoginRateLimit(t *testing.T) {
	a := newApp(t)
	for i := 0; i < 5; i++ {
		a.do("POST", "/login", url.Values{"passphrase": {"guess"}}, nil, false)
	}
	res, _ := a.do("POST", "/login", url.Values{"passphrase": {passphrase}}, nil, false)
	if res.StatusCode != http.StatusTooManyRequests {
		t.Errorf("sixth attempt: %d, want 429", res.StatusCode)
	}
	a.now = a.now.Add(time.Minute)
	a.login()
}

func TestSafeNext(t *testing.T) {
	for in, want := range map[string]string{
		"/reports": "/reports", "": "/review", "https://evil.example": "/review",
		"//evil.example": "/review", "/\\evil.example": "/review",
	} {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCrossSitePost(t *testing.T) {
	a := newApp(t)
	req := httptest.NewRequest("POST", "/login", strings.NewReader("passphrase=x"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	a.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("cross-site POST: %d, want 403", rec.Code)
	}
}

func TestReviewNotConfigured(t *testing.T) {
	if res, _ := getWith(t, testConfig, "/review"); res.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("/review without passphrase or store: %d, want 503", res.StatusCode)
	}
	if res, _ := getWith(t, testConfig, "/login"); res.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("/login without passphrase: %d, want 503", res.StatusCode)
	}
}

func TestReviewFlow(t *testing.T) {
	a := newApp(t)
	c := a.login()

	_, body := a.do("GET", "/review", nil, c, false)
	for _, want := range []string{
		"0 due", "1 new left", "New pattern", "What does this mean?",
		"Je voudrais un café, s&#39;il vous plaît.", `data-autoplay`,
		`name="sentence_id" value="fr/je-voudrais-noun/un-cafe"`, `name="card_id" value="0"`, `name="reps" value="0"`,
		`value="1" class="grade grade-1">Again<small>1m</small>`, `value="4" class="grade grade-4">Easy<small>`,
		"Report a problem", `<script src="/static/app.js" defer></script>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("review page missing %q", want)
		}
	}
	if strings.Contains(body, "data-undo") {
		t.Error("undo offered before any answer")
	}

	form := url.Values{"card_id": {"0"}, "sentence_id": {"fr/je-voudrais-noun/un-cafe"}, "reps": {"0"}, "rating": {"3"}, "elapsed_ms": {"4200"}}
	res, body := a.do("POST", "/review/answer", form, c, true)
	if res.StatusCode != 200 || strings.Contains(body, "<html") {
		t.Fatalf("answer: %d; want a fragment", res.StatusCode)
	}
	if got := a.reviews.answers; len(got) != 1 || got[0] != (store.Answer{UserID: theUser, SentenceID: "fr/je-voudrais-noun/un-cafe", Rating: fsrs.Good, ElapsedMS: 4200, Now: clock}) {
		t.Errorf("answers = %+v", got)
	}
	for _, want := range []string{"Done for today", "7 reviews, 1 new sentence, about 2 min.", "Next card due in 10m.", "data-undo"} {
		if !strings.Contains(body, want) {
			t.Errorf("done fragment missing %q", want)
		}
	}

	// Without htmx, answering reloads the page.
	if res, _ := a.do("POST", "/review/answer", form, c, false); res.StatusCode != http.StatusSeeOther {
		t.Errorf("plain answer: %d, want 303", res.StatusCode)
	}

	res, body = a.do("POST", "/review/undo", url.Values{}, c, true)
	if res.StatusCode != 200 || a.reviews.undos != 1 || !strings.Contains(body, "What does this mean?") {
		t.Errorf("undo: %d, undos %d", res.StatusCode, a.reviews.undos)
	}

	for name, bad := range map[string]url.Values{
		"rating 0":       {"card_id": {"5"}, "reps": {"1"}, "rating": {"0"}},
		"rating 5":       {"card_id": {"5"}, "reps": {"1"}, "rating": {"5"}},
		"no card":        {"card_id": {"0"}, "reps": {"0"}, "rating": {"3"}},
		"missing fields": {"rating": {"3"}},
	} {
		if res, _ := a.do("POST", "/review/answer", bad, c, true); res.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, res.StatusCode)
		}
	}
}

func TestReports(t *testing.T) {
	a := newApp(t)
	c := a.login()
	res, body := a.do("POST", "/reports", url.Values{"sentence_id": {"fr/x"}, "note": {"  Typo  "}}, c, true)
	if res.StatusCode != 200 || !strings.Contains(body, "Thanks") || len(a.reviews.reports) != 1 || a.reviews.reports[0] != "fr/x: Typo" {
		t.Errorf("report: %d %q, reports %v", res.StatusCode, body, a.reviews.reports)
	}
	if res, _ := a.do("POST", "/reports", url.Values{"sentence_id": {"fr/x"}, "note": {" "}}, c, true); res.StatusCode != http.StatusBadRequest {
		t.Errorf("empty report: %d, want 400", res.StatusCode)
	}
	if _, body := a.do("GET", "/reports", nil, c, false); !strings.Contains(body, "Typo") || !strings.Contains(body, `action="/reports/3/resolve"`) {
		t.Error("reports page missing the report")
	}
	if res, _ := a.do("POST", "/reports/3/resolve", url.Values{}, c, false); res.StatusCode != http.StatusSeeOther {
		t.Errorf("resolve: %d, want 303", res.StatusCode)
	}
}

func TestShort(t *testing.T) {
	for d, want := range map[time.Duration]string{
		10 * time.Second: "1m", 10 * time.Minute: "10m", 3 * time.Hour: "3h",
		4 * 24 * time.Hour: "4d", 60 * 24 * time.Hour: "2mo", 400 * 24 * time.Hour: "1y",
	} {
		if got := short(d); got != want {
			t.Errorf("short(%v) = %q, want %q", d, got, want)
		}
	}
}
