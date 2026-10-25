# Phrasebuilder

A small Go web app for learning tourist-level French (later Italian and German)
by drilling whole sentences built from patterns, with spaced repetition.
See [docs/PROPOSAL.md](docs/PROPOSAL.md) for the plan.

Status: milestone 1 (skeleton) done — a server-rendered page with htmx, deployed to
Cloud Run.

## Running locally

```sh
go run ./cmd/server                 # http://localhost:8080
go test ./...
```

Templates and static files are embedded, so the binary is self-contained.
It is configured by environment variable:

| Variable | Purpose |
|---|---|
| `PORT` | Listen port (default 8080) |
| `K_REVISION` | Build identifier shown by `/health` (Cloud Run sets it) |

### URLs

| Path | What |
|---|---|
| `/` | Home page |
| `/greeting` | htmx partial used by the home page |
| `/health` | Health check: `ok <revision>` (not `/healthz`, which Cloud Run reserves) |
| `/static/*` | CSS and the vendored htmx (2.0.11) |

## Hosting

Google Cloud project `phrasebuilder-prod` (own billing account, £5 monthly
budget alert), region `europe-west1`. Cloud Run service `phrasebuilder` is
public at <https://phrasebuilder-532278562214.europe-west1.run.app> (no custom
domain yet): 256 MiB, scale 0–3, startup probe on `/health`.

Two service accounts, so the default compute account needs no roles:

- `phrasebuilder-build` builds the image (`roles/run.builder`).
- `phrasebuilder-run` runs the service (no roles yet; it will get Secret
  Accessor on specific secrets).

Deploy a new version from the repo (Cloud Build builds the Dockerfile):

```sh
gcloud run deploy phrasebuilder --source . --region europe-west1 --project phrasebuilder-prod \
  --build-service-account=projects/phrasebuilder-prod/serviceAccounts/phrasebuilder-build@phrasebuilder-prod.iam.gserviceaccount.com
```

GitHub Actions runs `gofmt`, `go vet` and `go test` on every push.

## Licence

- Code: MIT (see `LICENSE`).
- Course content (from milestone 2, in `content/`): CC BY-SA 4.0.
