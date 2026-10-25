// Command server serves the Phrasebuilder web app.
//
// Configuration is by environment variable, to suit Cloud Run:
//
//	PORT        listen port (default 8080)
//	K_REVISION  build identifier shown by /health (set by Cloud Run)
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mmcnicol/phrasebuilder/internal/web"
)

func main() {
	if os.Getenv("K_SERVICE") != "" {
		// On Cloud Run, log JSON that Cloud Logging understands.
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{ReplaceAttr: cloudLogging})))
	}

	port := cmpEnv("PORT", "8080")
	h, err := web.New(web.Config{Revision: cmpEnv("K_REVISION", "dev")})
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
