# Phrasebuilder

A small Go web app for learning tourist-level French (later Italian and German)
by drilling whole sentences built from patterns, with spaced repetition.
See [docs/PROPOSAL.md](docs/PROPOSAL.md) for the plan.

Status: milestone 3 (review loop) done — a daily review at `/review`
(Recognise and Produce cards scheduled by FSRS), behind a passphrase until
sign-in arrives. Unit 1 (at the café) can be browsed with audio at `/units`.
See `docs/milestones/`.

## Running locally

```sh
go run ./cmd/server                 # http://localhost:8080
go test ./...
```

Templates and static files are embedded, so the binary is self-contained.
It is configured by environment variable, or locally by a git-ignored `.env`
file (copy `.env.example`):

| Variable | Purpose |
|---|---|
| `PORT` | Listen port (default 8080) |
| `K_REVISION` | Build identifier shown by `/health` (Cloud Run sets it) |
| `DATABASE_URL` | Postgres connection string; without it, `/units` returns 503 |
| `AUDIO_BASE_URL` | Where audio is served from; without it, no play buttons |
| `APP_PASSPHRASE` | Unlocks the review pages; without it, they return 503 |
| `SESSION_KEY` | Base64 of 32+ random bytes; signs login cookies. Unset, a random key is used and logins last until restart |
| `TEST_DATABASE_URL` | Neon dev branch; the store tests create and drop a temporary database there. Unset, they are skipped |

### URLs

| Path | What |
|---|---|
| `/` | Home page |
| `/greeting` | htmx partial used by the home page |
| `/units` | Units |
| `/units/<lang>/<unit>` | A unit's patterns, grammar notes and sentences, with audio |
| `/login`, `/logout` | Passphrase login; sets a signed cookie for a year |
| `/review` | Today's review, one card at a time (htmx posts to `/review/answer` and `/review/undo`) |
| `/reports` | Open problem reports (posted from the review page) |
| `/health` | Health check: `ok <revision>` (not `/healthz`, which Cloud Run reserves) |
| `/static/*` | CSS, `app.js` (play buttons, review shortcuts), and the vendored htmx (2.0.11) |

`/review` and `/reports` need a login; everything else is public.

## Reviews

Scheduling is in `internal/srs` (go-fsrs v4, default parameters, fuzz on).
The next card is worked out from the database on every request: due cards,
then up to 5 new sentences a day from at most 2 new patterns, then learning
cards due within 20 minutes, then done. A day starts at 4am UK time. A
sentence starts with a Recognise card; its Produce card is added once the
Recognise interval reaches 3 days. Every answer is logged with the card's
previous state (`review_log`), so it can be undone.

## Content

Course content is YAML in `content/<lang>/unit-NN-<unit>.yaml` (schema in
`docs/milestones/02-content-pipeline.md`). Each pattern is a sentence frame
with `{slots}`; sentences are generated from the frame and its slot fillers.
IDs such as `fr/je-voudrais-noun/un-cafe` are stable, so learner progress
survives re-imports; rows removed from the YAML are retired, never deleted.

```sh
go run ./cmd/import validate    # check the YAML (CI runs this too)
go run ./cmd/import audio       # make missing MP3s in the bucket (Text-to-Speech)
go run ./cmd/import load        # migrate the database in DATABASE_URL and load content
go run ./cmd/import all         # validate, audio, load
go run ./cmd/import samples     # write voice samples to ./voice-samples
```

`audio` and `samples` use Application Default Credentials
(`gcloud auth application-default login`, quota project `phrasebuilder-prod`).
Audio files are named by a hash of the voice and text, so they never change
and are only generated once. The French voice is `fr-FR-Chirp3-HD-Kore`
(`internal/audio`); changing it regenerates everything.

To load into production, point `DATABASE_URL` at the Neon `main` branch.

Database queries are in `internal/store/queries`; after changing them or the
migrations, run `sqlc generate` (v1.31.1) and commit the output in
`internal/store/db`.

## Hosting

Google Cloud project `phrasebuilder-prod` (own billing account, £5 monthly
budget alert), region `europe-west1`. Cloud Run service `phrasebuilder` is
public at <https://phrasebuilder-532278562214.europe-west1.run.app> (no custom
domain yet): 256 MiB, scale 0–3, startup probe on `/health`.

- Database: Neon project `phrasebuilder` in AWS Frankfurt. Branch `main` is
  production (the app uses its pooled connection string, in Secret Manager as
  `database-url`); branch `dev` is for local work and tests.
- Bucket `phrasebuilder-audio` (europe-west1, public read): MP3s from
  `cmd/import audio`, cached for a year.
- Environment: `DATABASE_URL`, `APP_PASSPHRASE` and `SESSION_KEY` from
  secrets `database-url`, `app-passphrase` and `session-key`;
  `AUDIO_BASE_URL=https://storage.googleapis.com/phrasebuilder-audio`.
  Changing the passphrase (a new secret version, then a deploy) logs every
  device out.

Two service accounts, so the default compute account needs no roles:

- `phrasebuilder-build` builds the image (`roles/run.builder`).
- `phrasebuilder-run` runs the service; it can read those three secrets and
  nothing else.

Deploy a new version from the repo (Cloud Build builds the Dockerfile; the
service keeps its environment, secrets and runtime account):

```sh
gcloud run deploy phrasebuilder --source . --region europe-west1 --project phrasebuilder-prod \
  --build-service-account=projects/phrasebuilder-prod/serviceAccounts/phrasebuilder-build@phrasebuilder-prod.iam.gserviceaccount.com
```

GitHub Actions runs `gofmt`, `go vet` and `go test` on every push.

## Licence

- Code: MIT (see `LICENSE`).
- Course content (`content/`): CC BY-SA 4.0 (see `content/LICENSE.md`).
