# Phrasebuilder

Go web app for learning languages through sentence patterns. Plan and milestones: `docs/PROPOSAL.md`.

## Stack

- Go (version in `go.mod`), `net/http` + chi router, `html/template` + htmx.
- **No JS build step.** htmx is vendored in `internal/web/static/`; no npm, no bundler, no CDN at runtime.
- Templates and static files are embedded with `//go:embed` in `internal/web`.
- Each page is `templates/layout.html` + `templates/<page>.html` defining `content`; register new pages in `parsePages` in `web.go`.
- Hosting: Cloud Run (`phrasebuilder-prod`, `europe-west1`), deployed with `gcloud run deploy --source`.
- Planned: Neon Postgres via sqlc + pgx behind a repository interface, goose migrations, go-fsrs, Google OIDC.

## Commands

```sh
go run ./cmd/server     # http://localhost:8080
go test ./...
go vet ./...
gofmt -l .              # must print nothing (CI checks it)
```

## Conventions

- Content lives in `/content` as YAML (from milestone 2), not in the database or code.
- No secrets in the repo (it is public). Production secrets go in Secret Manager; local ones in git-ignored `.env`.
- Always pass `--project phrasebuilder-prod` to gcloud; the machine's default project belongs to another app.
- Health check is `/health`, not `/healthz` (Cloud Run reserves some paths ending in `z`).
