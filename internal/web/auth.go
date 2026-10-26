package web

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// Until sign-in (milestone 4), a passphrase unlocks the review pages for
// the only user. Logging in sets a signed cookie, so each device logs in
// once a year.
const (
	sessionCookie = "pb_session"
	sessionLength = 365 * 24 * time.Hour
	theUser       = int64(1)
)

type auth struct {
	passphrase [sha256.Size]byte // hashed, so comparing takes constant time
	key        []byte            // signs cookies
	limiter    *rate.Limiter     // login attempts, across all clients
	now        func() time.Time
}

// newAuth returns nil if there is no passphrase. Without a session key, a
// random one is used, so logins last until the server restarts. The cookie
// key is derived from the passphrase too: changing it logs everyone out.
func newAuth(passphrase string, sessionKey []byte, now func() time.Time) *auth {
	if passphrase == "" {
		return nil
	}
	if len(sessionKey) == 0 {
		slog.Warn("SESSION_KEY not set; logins last until the server restarts")
		sessionKey = make([]byte, 32)
		rand.Read(sessionKey)
	}
	mac := hmac.New(sha256.New, sessionKey)
	mac.Write([]byte("phrasebuilder session\x00" + passphrase))
	return &auth{
		passphrase: sha256.Sum256([]byte(passphrase)),
		key:        mac.Sum(nil),
		// A burst of 5, then one every 6 seconds: slow enough that only a
		// short passphrase could be guessed.
		limiter: rate.NewLimiter(rate.Every(6*time.Second), 5),
		now:     now,
	}
}

func (a *auth) allowAttempt() bool { return a.limiter.AllowN(a.now(), 1) }

func (a *auth) checkPassphrase(p string) bool {
	h := sha256.Sum256([]byte(p))
	return subtle.ConstantTimeCompare(h[:], a.passphrase[:]) == 1
}

func (a *auth) sign(payload string) string {
	mac := hmac.New(sha256.New, a.key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// cookie is a session cookie for user: "<user>.<expiry>.<signature>".
func (a *auth) cookie(user int64) *http.Cookie {
	exp := a.now().Add(sessionLength)
	payload := fmt.Sprintf("%d.%d", user, exp.Unix())
	return &http.Cookie{
		Name: sessionCookie, Value: payload + "." + a.sign(payload), Path: "/",
		Expires: exp, MaxAge: int(sessionLength.Seconds()),
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	}
}

// user returns the logged-in user, if the request has a valid cookie.
func (a *auth) user(r *http.Request) (int64, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return 0, false
	}
	i := strings.LastIndexByte(c.Value, '.')
	if i < 0 || !hmac.Equal([]byte(c.Value[i+1:]), []byte(a.sign(c.Value[:i]))) {
		return 0, false
	}
	userStr, expStr, ok := strings.Cut(c.Value[:i], ".")
	if !ok {
		return 0, false
	}
	user, err1 := strconv.ParseInt(userStr, 10, 64)
	exp, err2 := strconv.ParseInt(expStr, 10, 64)
	if err1 != nil || err2 != nil || !a.now().Before(time.Unix(exp, 0)) {
		return 0, false
	}
	return user, true
}

type userKey struct{}

func userFrom(ctx context.Context) int64 { return ctx.Value(userKey{}).(int64) }

// requireUser lets logged-in users through; others are sent to /login.
// Pages behind it are personal, so they are never cached.
func (s *server) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil || s.cfg.Reviews == nil {
			s.error(w, http.StatusServiceUnavailable, "Reviews are not set up on this server.")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		user, ok := s.auth.user(r)
		if !ok {
			if r.Method == http.MethodGet {
				http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
				return
			}
			if isHTMX(r) {
				w.Header().Set("HX-Redirect", "/login")
			}
			s.error(w, http.StatusUnauthorized, "Please log in.")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, user)))
	})
}

func (s *server) loginForm(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		s.error(w, http.StatusServiceUnavailable, "Logging in is not set up on this server.")
		return
	}
	next := safeNext(r.URL.Query().Get("next"))
	if _, ok := s.auth.user(r); ok {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}
	s.render(w, http.StatusOK, "login", map[string]any{"Next": next})
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		s.error(w, http.StatusServiceUnavailable, "Logging in is not set up on this server.")
		return
	}
	next := safeNext(r.PostFormValue("next"))
	if !s.auth.allowAttempt() {
		s.render(w, http.StatusTooManyRequests, "login", map[string]any{"Next": next, "Error": "Too many attempts. Wait a minute and try again."})
		return
	}
	if !s.auth.checkPassphrase(r.PostFormValue("passphrase")) {
		slog.Warn("wrong passphrase")
		s.render(w, http.StatusUnauthorized, "login", map[string]any{"Next": next, "Error": "That passphrase is not right."})
		return
	}
	http.SetCookie(w, s.auth.cookie(theUser))
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// safeNext keeps redirects after login on this site.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/review"
	}
	return next
}

func isHTMX(r *http.Request) bool { return r.Header.Get("HX-Request") == "true" }
