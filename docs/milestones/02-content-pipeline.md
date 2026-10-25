# Milestone 2 – Content pipeline (plan)

Goal: write a French unit as YAML, check it, generate its audio, load it into
Postgres, and **browse it on the live site with audio**. Ends in something
usable: listening to and reading unit 1 on the phone.

## Outcome

- `content/fr/unit-01-cafe.yaml`: about 8–10 patterns, about 30 words, all `status: draft`.
- `go run ./cmd/import validate|audio|load|all`.
- Neon Postgres holding units, patterns, words and sentences.
- Bucket `phrasebuilder-audio` with an MP3 for every sentence and word.
- New pages `/units` and `/units/{id}`: patterns, grammar notes, sentences, ▶ play buttons.

## 1. Content schema (`content/<lang>/unit-NN-<slug>.yaml`)

```yaml
unit: cafe                       # id, unique per language
order: 1
title: At the café
patterns:
  - id: je-voudrais-noun         # unique per language; stored as fr/je-voudrais-noun
    status: draft                # draft | checked
    frame: "Je voudrais {item}, s'il vous plaît."
    en: "I would like {item}, please."
    note: "Je voudrais = I would like. Politer than je veux."
    slots:
      item:
        - { fr: "un café", en: "a coffee", g: m }
        - { fr: "une bière", en: "a beer", g: f }
        - { fr: "l'addition", en: "the bill", g: f }
  - id: ou-sont-les-toilettes    # no slots: a fixed phrase, one sentence
    status: draft
    frame: "Où sont les toilettes ?"
    en: "Where are the toilets?"
    note: "..."
```

**Sentences are generated**: frame × slot fillers (cartesian product for
several slots). Filler text is inserted as written, so the author writes
fillers that fit the frame (`de l'eau`, not `de` + `l'eau`). If a slot opens
the sentence, its first letter is capitalised.

**IDs are stable, so learner progress survives re-imports** (the cards in
milestone 3 point at these IDs):

- word: `fr/un-cafe` (language + slug of the French text; shared by every pattern that uses it)
- sentence: `fr/je-voudrais-noun/un-cafe` (pattern + filler slugs)

Editing a filler's text therefore creates a new item. Fixing a typo in a
frame or note does not.

**Validator rules** (each error reported with file and line):

- Strict decoding: unknown keys, such as a misspelled `nots:`, are errors.
- IDs are lowercase slugs and unique: units and patterns per language, unit `order` per language.
- Every `{slot}` in `frame` is defined and also appears in `en`; no unused slots; no duplicate fillers.
- Required fields are not empty; `status` is draft/checked; `g` is m/f/blank; the unit's language matches its directory.
- At most 20 sentences per pattern, to catch an accidental cartesian explosion.
- `note` is at most 300 characters ("2–3 lines").
- Two fillers that differ but produce the same word ID are an error.

YAML library: `go.yaml.in/yaml/v3` (the YAML org's maintained fork of `gopkg.in/yaml.v3`).

## 2. Database (Neon, via goose + sqlc + pgx)

Migrations are in `internal/store/migrations/*.sql`, embedded and run by
`import load` (the server never migrates). Tables for this milestone:

| Table | Columns |
|---|---|
| `languages` | code, name |
| `units` | id, language, position, title, retired_at |
| `patterns` | id, unit_id, position, frame, en, note, status, retired_at |
| `words` | id, language, text, en, gender, audio_path, retired_at |
| `sentences` | id, pattern_id, position, text, en, audio_path, retired_at |
| `sentence_words` | sentence_id, word_id, slot |

`users`, `cards`, `review_log` etc. come in milestones 3–4.

**Load** = one transaction per language: upsert everything in the YAML; rows
no longer in the YAML get `retired_at` set instead of being deleted, so
future cards and review history are never orphaned. Re-running is a no-op.

Queries are in `internal/store/queries/*.sql`. The code `sqlc` generates is
committed, so CI and Cloud Build don't need sqlc (sqlc is installed locally
from its release binary). The web handlers depend on a small `ContentStore`
interface, which is the "repository interface" from the proposal. Tests use a
fake.

Neon setup: a project in **AWS Frankfurt (eu-central-1)**, the region closest
to Cloud Run in Belgium.

- `main` branch: production. The app uses its pooled (`-pooler`) connection string.
- `dev` branch: local development, and integration tests when `TEST_DATABASE_URL` is set (skipped in CI).

## 3. Audio (Text-to-Speech → Cloud Storage)

- `import audio` synthesises an MP3 for every sentence and word that doesn't have one yet.
- **Content-addressed paths**: `fr/<sha256(voice + text)[:16]>.mp3`.
  - Re-running only fills gaps, and changed text gets a new file.
  - Files never change, so they are served with `Cache-Control: public, max-age=31536000, immutable`, which also suits Cloudflare later.
  - `load` can compute every `audio_path` itself, so `audio` and `load` are independent.
- One fixed fr-FR voice, chosen by listening to two or three samples (likely Chirp 3 HD or Neural2). Check the free tier for the chosen voice type before running. Unit 1 is only about 2,000 characters, so the cost is pennies at worst.
- Normal speed only; a "slow" button can use the browser's `playbackRate` later.
- Bucket `phrasebuilder-audio`, europe-west1, uniform access, public read (like kirkyards-photos).
- The CLI authenticates with `gcloud auth application-default login` (no key file). The quota project is `phrasebuilder-prod`.
- TTS and storage sit behind interfaces, so tests use fakes. The Google client libraries are imported only by `cmd/import`; the server binary doesn't include them.

## 4. Web pages

- `/units`: units with pattern counts.
- `/units/{id}`: each pattern's sentences, English, grammar note, and a ▶ button (`<audio>` with `preload="none"`).
- CSP gains `media-src https://storage.googleapis.com`.
- Config: `DATABASE_URL` (Secret Manager secret `database-url` → Cloud Run env var; runtime SA `phrasebuilder-run` gets Secret Accessor on that secret only) and `AUDIO_BASE_URL`.
- Locally `DATABASE_URL` is optional: without it, `/units` returns 503 and the rest still works. `.env` is loaded at startup (`joho/godotenv`).

## 5. Tests and CI

- Validator: table-driven tests over small broken YAML fixtures, one per rule.
- Expansion: sentence and word IDs, capitalisation, cartesian product.
- Audio: path hashing; "skip existing" with a fake store.
- Load: integration test against the Neon dev branch (upsert, idempotent re-run, retire) when `TEST_DATABASE_URL` is set.
- Web: `/units` pages with a fake `ContentStore`.
- CI also runs `go run ./cmd/import validate`, so every push checks the content.

## Build order (each step committed)

1. Schema types, expansion, validator, and tests: no cloud needed.
2. Draft unit 1 (café) YAML: needs **your review** of the French.
3. Migrations, sqlc queries, loader, and Neon setup.
4. Bucket, TTS voice choice, and `import audio`.
5. `/units` pages, secret wiring, deploy, and check on the phone.

## Needs you

- **Neon**: sign up (GitHub login, no card) and create project `phrasebuilder` in AWS Frankfurt with a `dev` branch.
  - Put the dev connection string in `.env` yourself; don't paste secrets into the chat.
  - Store the production string with: `! printf %s 'postgres://…' | gcloud secrets versions add database-url --data-file=- --project phrasebuilder-prod`
- **ADC**: `! gcloud auth application-default login`. This is lost if the live USB session resets, and so is `.env`.
- **Voice**: listen to the samples and pick one.
- **Content**: review the drafted unit 1 French. It stays `draft` either way.

## Deferred

- Swap/Type data such as accepted alternative answers (phase 2; strict decoding means adding fields later is easy).
- "Report a problem" button (milestone 3, when there are exercises).
- Tatoeba examples.
- Deploying from GitHub Actions.
