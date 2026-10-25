# Phrasebuilder

Go web app for learning languages through sentence patterns. Plan and milestones: `docs/PROPOSAL.md`.

## Stack

- Go (version in `go.mod`), `net/http` + chi router, `html/template` + htmx.
- **No JS build step.** htmx is vendored in `internal/web/static/`; no npm, no bundler, no CDN at runtime.
- Templates and static files are embedded with `//go:embed` in `internal/web`.
- Each page is `templates/layout.html` + `templates/<page>.html` defining `content`; register new pages in `parsePages` in `web.go`.
- Hosting: Cloud Run (`phrasebuilder-prod`, `europe-west1`), deployed with `gcloud run deploy --source`.
- Neon Postgres via pgx; queries in `internal/store/queries` → `sqlc generate` → committed `internal/store/db`. Web depends on the `ContentStore` interface, not the DB.
- goose migrations in `internal/store/migrations` (run by `cmd/import load`, never by the server; the server must not import goose or Google Cloud libraries).
- Audio: Text-to-Speech → bucket `phrasebuilder-audio`, content-addressed by voice + text (`internal/audio`).
- Planned: go-fsrs, Google OIDC.

## Commands

```sh
go run ./cmd/server     # http://localhost:8080
go test ./...
go vet ./...
gofmt -l .              # must print nothing (CI checks it)
go run ./cmd/import validate   # check content YAML
go run ./cmd/import all        # audio + load into DATABASE_URL (needs ADC and .env)
sqlc generate           # after changing queries or migrations (v1.31.1)
```

## Conventions

- Content lives in `/content` as YAML, not in the database or code. IDs must stay stable: learner progress points at them.
- No secrets in the repo (it is public). Production secrets go in Secret Manager; local ones in git-ignored `.env`.
- Never `source` `.env` in a shell or print its values: the Neon URLs contain `&` and a password.
- Always pass `--project phrasebuilder-prod` to gcloud; the machine's default project belongs to another app.
- Health check is `/health`, not `/healthz` (Cloud Run reserves some paths ending in `z`).
