// Command server serves the Phrasebuilder web app.
//
// Configuration is by environment variable, to suit Cloud Run:
//
//	PORT            listen port (default 8080)
//	K_REVISION      build identifier shown by /health (set by Cloud Run)
//	DATABASE_URL    Postgres connection string; without it, content pages return 503
//	AUDIO_BASE_URL  where audio files are served from; without it, no play buttons
//	APP_PASSPHRASE  unlocks the review pages; without it, they return 503
//	SESSION_KEY     base64, at least 32 random bytes, signs login cookies;
//	                without it, logins last until the server restarts
//
// Locally, these can be put in a .env file.
package main

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/mmcnicol/phrasebuilder/internal/store"
	"github.com/mmcnicol/phrasebuilder/internal/web"
)

func main() {
	if os.Getenv("K_SERVICE") != "" {
		// On Cloud Run, log JSON that Cloud Logging understands.
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{ReplaceAttr: cloudLogging})))
	}

	godotenv.Load() // optional; real environment variables take precedence

	port := cmpEnv("PORT", "8080")
	cfg := web.Config{
		Revision:     cmpEnv("K_REVISION", "dev"),
		AudioBaseURL: os.Getenv("AUDIO_BASE_URL"),
		Passphrase:   os.Getenv("APP_PASSPHRASE"),
	}
	if k := os.Getenv("SESSION_KEY"); k != "" {
		key, err := base64.StdEncoding.DecodeString(k)
		if err != nil || len(key) < 32 {
			fatal("SESSION_KEY", errors.New("must be base64 of at least 32 bytes"))
		}
		cfg.SessionKey = key
	}
	if cfg.Passphrase == "" {
		slog.Warn("APP_PASSPHRASE not set; review pages are unavailable")
	}
	if url := os.Getenv("DATABASE_URL"); url != "" {
		s, err := store.Open(context.Background(), url)
		if err != nil {
			fatal("open database", err)
		}
		defer s.Close()
		cfg.Store = s
		cfg.Reviews = s
	} else {
		slog.Warn("DATABASE_URL not set; content pages are unavailable")
	}
	h, err := web.New(cfg)
	if err != nil {
		fatal("build site", err)
	}

	hs := &http.Server{
		Addr:              ":" + port,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		hs.Shutdown(shutdown)
	}()
	slog.Info("listening", "port", port)
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fatal("serve", err)
	}
}

// cloudLogging renames slog's level and msg keys to Cloud Logging's
// severity and message.
func cloudLogging(groups []string, a slog.Attr) slog.Attr {
	if len(groups) > 0 {
		return a
	}
	switch a.Key {
	case slog.LevelKey:
		a.Key = "severity"
	case slog.MessageKey:
		a.Key = "message"
	}
	return a
}

func cmpEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func fatal(msg string, err error) {
	slog.Error(msg, "err", err)
	os.Exit(1)
}
