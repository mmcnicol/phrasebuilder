# Language Learning Web App – Project Proposal

Oct 25, 2026 · @Matthew McNicol

## Summary

Build a small Go web app that teaches tourist-level French (then Italian and German) by drilling **whole sentences built from patterns**, not isolated words, with spaced repetition and progress tracking per user.

Working name: **Phrasebuilder** (placeholder; pick a domain later). It deliberately keeps the one thing that has worked for you, flashcards, and extends it to the thing that hasn't: constructing sentences. It runs on the same Cloud Run + Cloud Storage + Cloudflare setup as kirkyards.scot, so the operational side is already known.

## Background and motivation

The learner (me) has plenty of exposure but little speaking ability. That gap is the problem this app targets.

| Experience | When | What it gave |
| --- | --- | --- |
| Mandatory French at school | Age 11–13 | Basic vocabulary, little retained |
| French electives at university | Age 18–20 | Grammar rules, studied but not used |
| French evening classes | Age 25 | Classroom conversation |
| Pimsleur French audio courses | Self-study | Listening, some pronunciation |
| Paper word cards (FR ↔ EN) | Self-study | **Real vocabulary gains** |
| Trips: France ×3–4, Germany ×2–3, Italy ×1 | Holidays, judo | Motivation, real situations |

**What worked:** short, repeated, self-paced recall of individual words.

**What didn't:** turning words into sentences. Grammar was learned as rules to read, never as patterns to produce under light pressure.

Existing apps (Duolingo, Babbel, Anki, Pimsleur) each cover part of this. The point of building my own is to (a) get exactly the method that suits me, (b) learn by building, and (c) have a portfolio project in Go. It does not need to beat them commercially.

## Goals, non-goals and success criteria

The primary goal is spoken confidence in tourist situations in French within 6 months of daily 15-minute use.

**Goals**

- Learner can sign up, log in and pick a target language (FR first; IT and DE later).
- Structured path of short units built around real situations: café, hotel, train, directions, shop, small talk, emergencies.
- Every unit teaches a few **sentence patterns** and the words to slot into them.
- Spaced-repetition review of both words and patterns, so the daily session is chosen for you.
- Audio for every sentence, and a mode where you answer out loud before revealing.
- Progress tracking: streak, items due, items mastered, units completed.

**Non-goals (for now)**

- Native mobile apps (a responsive web app / PWA is enough).
- Social features, leaderboards, paid subscriptions.
- Automated speech scoring or AI conversation partner (possible stretch).
- Covering grammar exhaustively; only what tourist sentences need.

**Success criteria (personal)**

- [ ] Order food, book a room, buy a ticket and ask directions in French without notes.
- [ ] Produce \~150 core sentence patterns from an English prompt, out loud, in under 5 seconds each.
- [ ] Used on 80% of days for 3 months (the app is only useful if I keep coming back).
- [ ] Stretch: read a short French news article (e.g. from a learner-level news source) with a dictionary only for occasional words.

## Learning approach

The core idea: **treat a sentence pattern like a flashcard**, and grammar becomes something you recall and produce, not read about.

A *pattern* is a sentence frame with slots, e.g. `Je voudrais [un café / une chambre / deux billets], s'il vous plaît.` Each pattern carries one small grammar point (here: *je voudrais* + noun with article). Words are taught as slot fillers, so every new word immediately lives in a sentence.

**Five exercise types, from easy to hard**

1. **Recognise** – hear/see the French sentence, recall the English meaning (your old paper cards).
2. **Build** – tap word tiles into the right order to make the sentence; teaches word order and agreement without typing.
3. **Swap** – given a pattern you know, change one slot ("a coffee" → "two tickets") and adjust articles/agreement.
4. **Produce** – see English, say the French **out loud**, then reveal and self-grade (Again / Hard / Good / Easy). This is the Pimsleur-style recall step, but on demand.
5. **Type** – type the French; checked with accent-tolerant matching and accepted alternatives.

**Scheduling.** Each (item, exercise type) gets its own spaced-repetition schedule. Use the open-source FSRS algorithm (a Go implementation exists: `go-fsrs`) rather than inventing one. An item graduates from Recognise to Produce as it becomes stable.

**Daily session.** \~15 minutes: due reviews first, then 1 new pattern with 3–5 new words. The app decides; the learner just presses Start.

**Grammar notes.** Each pattern has a 2–3 line note ("*voudrais* is the polite form of *vouloir*; use it to ask for things"). Short, attached to the sentence, never a standalone lecture.

**Reading (stretch).** Graded short texts built only from patterns and words already learned, with tap-to-translate. Later: import a news article and highlight which words you already know.

The multi-language design is identical: a course is just patterns + words + audio for one language pair (EN→FR, EN→IT, EN→DE).

## Feature scope by phase

The MVP is single-language (EN→FR), single-user-friendly, and usable daily by me; everything else waits until I've used it for a month.

| Feature | MVP | Phase 2 | Stretch |
| --- | --- | --- | --- |
| Sign up / log in (Sign in with Google) | ✓ |  |  |
| Email magic-link login |  | ✓ |  |
| French course: 10 situation units, \~100 patterns, \~500 words | ✓ |  |  |
| Exercises: Recognise, Build, Produce | ✓ |  |  |
| Exercises: Swap, Type |  | ✓ |  |
| FSRS scheduling + daily session | ✓ |  |  |
| Pre-generated audio for every sentence | ✓ |  |  |
| Progress page: streak, due today, mastered, unit completion | ✓ |  |  |
| Grammar notes per pattern | ✓ |  |  |
| Admin: import course content from YAML files in the repo | ✓ |  |  |
| Daily reminder: phone notification (Web Push) |  | ✓ |  |
| Daily reminder: email |  | ✓ |  |
| Italian and German courses |  | ✓ |  |
| Installable PWA, offline review |  | ✓ |  |
| Settings: daily new-item limit, session length, reminder time |  | ✓ |  |
| Graded reading texts with tap-to-translate |  |  | ✓ |
| Paste a news article → highlight known words |  |  | ✓ |
| Pronunciation check (speech recognition) |  |  | ✓ |
| AI role-play conversation (e.g. "order at a café") |  |  | ✓ |

One rule keeps scope honest: **no new feature until the previous phase has been used for 30 days.**

## Content strategy

Content is the hard part, not code. Write it as plain YAML files in the repo, drafted with AI help and reviewed by hand, so it is versioned, diffable and owned outright.

**Example content file** (`content/fr/unit-02-cafe.yaml`):

```yaml
unit: cafe
title: At the café
patterns:
  - id: fr-je-voudrais-noun
    frame: "Je voudrais {item}, s'il vous plaît."
    en: "I would like {item}, please."
    note: "Je voudrais = I would like. Politer than je veux."
    slots:
      item:
        - { fr: "un café", en: "a coffee" }
        - { fr: "une bière", en: "a beer" }
        - { fr: "l'addition", en: "the bill" }
```

**Sources and licensing**

- Write my own sentences (with AI drafting), so there are no copyright issues.
- Word frequency lists to choose vocabulary: check licences before use (several open frequency lists exist under CC licences).
- Tatoeba (CC BY 2.0 FR) sentences as optional extra examples, with attribution.
- Do **not** copy Pimsleur, Duolingo or textbook material.

**Quality control.** Content is not native-checked in the MVP, so review never blocks progress. Patterns stay marked `draft` in the YAML, and a "report a problem" button flags doubtful sentences. If the app takes off, pay a translation agency to review each language and flip patterns to `checked`.

**Language variety.** Neutral (standard metropolitan) French, Standard German (Hochdeutsch), and standard Italian.

**Audio.** Generate MP3s with Google Cloud Text-to-Speech at content-import time, store in Cloud Storage, and never generate on the fly. One fixed voice per language keeps it consistent.

## Technical architecture

Reuse the kirkyards.scot shape: one Go container on Cloud Run, files in Cloud Storage, Cloudflare in front once a domain is bought (until then, the plain run.app URL). The only genuinely new parts are a database, login and the content import tool.

&#91;embedded content: architecture · runtime and build-time parts\]

At runtime only the Go app, the database and cached audio are involved; Text-to-Speech is called only when content is imported.

**Stack choices**

| Concern | Choice | Why |
| --- | --- | --- |
| Language | Go (latest stable) | Personal preference; single static binary |
| HTTP | `net/http` + chi router | Small, idiomatic, familiar from kirkyards-style apps |
| UI | `html/template` + htmx, minimal CSS | No JS build step; server-rendered; works on phone |
| Auth | Sign in with Google via OIDC (`coreos/go-oidc`), cookie sessions | No password storage or reset emails |
| Scheduling | [`go-fsrs`](https://github.com/open-spaced-repetition/go-fsrs) | Maintained FSRS implementation in Go |
| DB access | `sqlc` + `pgx` (Postgres) or `database/sql` (SQLite) | Typed queries without an ORM; close to JDBC habits |
| Migrations | `goose` | Plain SQL files |
| Audio | Google Cloud Text-to-Speech → Cloud Storage | Free tier covers this volume |
| Hosting | Cloud Run, scale to zero, Docker image | As kirkyards |
| CI/CD | GitHub Actions → Artifact Registry → Cloud Run | Test on every push, deploy from main |

**Database choice** (the one real cost decision)

| Option | Free tier | Pros | Cons |
| --- | --- | --- | --- |
| [Neon](https://neon.com/pricing) (serverless Postgres) | 0.5 GB, 100 compute-hours a month, no card needed | Sleeps after 5 min idle and wakes on the next query automatically; never pauses for good | Cold start of a second or so after idle; compute stops if monthly limits are hit |
| [Supabase](https://supabase.com/pricing) (Postgres + extras) | 500 MB, 2 projects | Well known; dashboard, auth and storage bundled | Paused after 7 days without activity; needs a manual restore in the dashboard |
| [MongoDB Atlas](https://www.mongodb.com/docs/atlas/reference/free-shared-limitations/) free cluster | 512 MB, 100 ops/sec | Account already exists | Document model fits users → cards → reviews less naturally than SQL; idle free clusters may be deactivated |
| SQLite + Litestream to Cloud Storage | £0 | Simplest, no extra service | Cloud Run capped at 1 instance; more care with backups |
| Cloud SQL for Postgres (smallest) | none, \~£8–10 / month | All inside Google Cloud | Costs money even when idle |

Decision: **Neon (free serverless Postgres)**, behind a repository interface, so moving to Cloud SQL later is a config change.

**Repo layout**

```text
cmd/server/      main web app
cmd/import/      content validator + loader + TTS
internal/web/    handlers, templates, htmx partials
internal/srs/    go-fsrs wrapper, session builder
internal/store/  sqlc queries, migrations
content/fr/      YAML units (later it/, de/)
docs/PROPOSAL.md this document
```

**Secrets**

No secret ever lives in the repo; production secrets live in Google Secret Manager.

| Secret | Where it lives | How the app gets it |
| --- | --- | --- |
| Google OAuth client secret | Secret Manager | Cloud Run exposes it as an env var |
| Session cookie signing key | Secret Manager | Cloud Run env var |
| Database connection string (password) | Secret Manager | Cloud Run env var |
| Local dev values | `.env` file, git-ignored | Loaded at startup; `.env.example` with dummy values is committed |

- The OAuth client ID, project ID and bucket name are not secrets and can sit in config.
- The Cloud Run service account gets the Secret Accessor role on those secrets only.
- GitHub Actions deploys using Workload Identity Federation, so there is no service-account JSON key to store or leak.
- The import CLI runs locally with `gcloud auth application-default login`, again no key file.
- Turn on GitHub secret scanning with push protection; optionally a `gitleaks` pre-commit hook.
- If a secret is ever committed, rotate it; deleting the commit is not enough.

**Public repository**

The repo can be public, and that is a reasonable default for a portfolio project.

- Benefits: shows your Go work; free GitHub Actions minutes and secret scanning on public repos.
- Code licence: MIT is the simple choice.
- Content licence: decide separately. CC BY-SA keeps it open; "all rights reserved" keeps commercial options open. Tatoeba sentences, if used, need CC BY attribution either way.
- Anyone can copy the code and content; that is acceptable for a side project.
- Check any employer policy on outside work and IP before publishing (see Risks).

**Daily reminders**

Each learner can opt in to a reminder at a time they choose (default 9pm, their local time) if they have not done today's session. Running cost: £0.

- **Trigger:** one [Cloud Scheduler](https://cloud.google.com/scheduler/pricing) job calls a protected `/tasks/reminders` endpoint every 15 minutes. The first 3 jobs per billing account are free; check how many kirkyards already uses. The app picks users whose reminder time has passed, with no activity today and no reminder sent today.
- **Push notifications (Web Push):** free and needs no domain, because the run.app URL is already HTTPS. Each device subscribes once (open the site, allow notifications); after that, reminders arrive without the site being open. Android Chrome shows them like any app notification; on the laptop they appear while the browser is running. Needs a service worker. Go library: `webpush-go`; its VAPID private key goes in Secret Manager.
- **Email while it's just me:** send through my own Gmail account over SMTP with an app password (requires 2-Step Verification), from me to me. No domain needed. Check that Google still offers app passwords when building this.
- **Email for other users:** needs a sending domain for SPF/DKIM. Use [Resend](https://resend.com/pricing) (free: 3,000 a month, 100 a day) or [Brevo](https://www.brevo.com/pricing/) (free: 300 a day) once the app has its own domain. Not kirkyards.scot.
- **SMS (Twilio):** skip. Per-message charges add up, and push does the same job for free.
- **Consent:** off by default, opt-in per channel, a one-click unsubscribe link in every email (UK GDPR / PECR).

## Data model sketch

Two halves: **course content** (read-only, loaded from YAML) and **learner state** (written on every review). Keeping them separate means content can be re-imported without touching progress.

| Table | Key columns | Notes |
| --- | --- | --- |
| `languages` | code (fr, it, de), name |  |
| `units` | id, language, order, title | One per situation |
| `patterns` | id, unit\_id, frame, en, note, status | status = draft / checked |
| `words` | id, language, text, en, gender, audio\_path | Slot fillers |
| `sentences` | id, pattern\_id, fr, en, audio\_path | Generated from frame × slots |
| `users` | id, email, display\_name, auth\_provider, timezone, reminder\_time, reminder\_channel, last\_reminded\_on, created\_at |  |
| `enrolments` | user\_id, language, started\_at, daily\_new\_limit |  |
| `cards` | user\_id, item\_type, item\_id, exercise, FSRS fields (due, stability, difficulty, reps, lapses, state) | One row per item × exercise type |
| `review_log` | user\_id, card\_id, rating, reviewed\_at, elapsed\_ms | Drives stats and future FSRS tuning |
| `daily_activity` | user\_id, date, reviews, new\_items, minutes | Streaks and progress charts |

The `cards` table maps directly onto go-fsrs's `Card` struct, so the scheduler is a thin wrapper.

## Costs, privacy and risks

Running cost should stay under about £2 a month plus the domain, provided the database is not Cloud SQL.

**Estimated running costs** (approximate; verify before launch)

| Item | Estimate | Notes |
| --- | --- | --- |
| Domain (OVHcloud) | £0 to start (\~£10–15 / year later) | Start on the run.app URL; buy a domain only before opening public sign-up |
| Cloudflare DNS + CDN | £0 | Free plan; caches audio files |
| Cloud Run | \~£0 | Free tier covers a low-traffic app scaled to zero |
| Cloud Storage (audio) | < £1 / month | A few thousand small MP3s |
| Text-to-Speech | \~£0 | One-off generation; \~100k characters per language vs 4M free Standard / 1M free WaveNet per month ([pricing](https://cloud.google.com/text-to-speech/pricing)) |
| Database | £0 | Neon free plan (see Architecture) |

Set a Google Cloud **budget alert at £5** on day one.

**Google Cloud separation from kirkyards.scot**

phrasebuilder gets its own Google Cloud project **and** its own billing account, so nothing is shared with kirkyards.

- **New project** (e.g. `phrasebuilder-prod`): its own Cloud Run service, Cloud Storage bucket, Secret Manager secrets, service accounts and IAM. Nothing in one project can touch the other.
- **New billing account**, linked only to that project: costs appear on their own invoice, and the budget alert covers only this app.
- Free tiers such as Cloud Run's and Cloud Scheduler's 3 free jobs are counted per billing account, so a separate account also stops kirkyards and phrasebuilder eating into each other's allowance.
- Creating a billing account needs a payment method. Google can limit how many billing accounts a personal account may have; if it refuses, fall back to the existing billing account with a separate project and a project-scoped budget alert.
- Separate GitHub Actions deploy identity (Workload Identity Federation pool and service account) per project.

**Privacy (UK GDPR)**

- Store only email, display name and learning data; no passwords if using Google sign-in.
- Privacy notice page, "delete my account" button that removes all rows, and data export.
- Check whether the ICO data protection fee applies once other people sign up.
- No analytics cookies; Cloudflare Web Analytics is cookieless.

**Risks**

| Risk | Mitigation |
| --- | --- |
| Content effort stalls the project | MVP = 10 units only; AI-assisted drafting; ship unit 1 before writing unit 2 |
| Incorrect French in content | draft status on all MVP content; "report a problem" button; paid translation-agency review if the app takes off |
| I stop using it (the real failure mode) | 15-minute sessions, a streak, phone-friendly UI, push to use it before building more |
| Scope creep into a Duolingo clone | 30-day rule; non-goals list above |
| Employer policy on side projects | Check employer policy on outside work and IP; keep it on personal accounts and hardware |
| SQLite on Cloud Run losing data | If chosen: Litestream replication to Cloud Storage, max 1 instance, nightly backup |

## Milestones, handoff and open questions

Build in six small milestones, each ending in something I can use, and hand each one to Claude Code as a separate task.

1. **Skeleton** – new Google Cloud project and billing account, Go module, chi router, html/template + htmx, Dockerfile, health check, deploy to Cloud Run on its run.app URL (no custom domain yet). Same pattern as kirkyards.
2. **Content pipeline** – YAML schema, validator CLI (`go run ./cmd/import`), load into DB, TTS audio generation to Cloud Storage. One French unit.
3. **Review loop (no login)** – Recognise + Produce exercises, go-fsrs scheduling, single hard-coded user. *Start using it daily here.*
4. **Accounts** – Sign in with Google (OIDC), sessions, per-user cards, delete-my-account.
5. **Progress + Build exercise** – progress page, streak, word-tile Build exercise, 10 French units.
6. **Phase 2** – Swap and Type exercises, Italian and German content, PWA.

**Handoff to Claude Code**

- [ ] Create the public GitHub repo mmcnicol/phrasebuilder; export this doc to Markdown as `docs/PROPOSAL.md`.
- [ ] Write a short `CLAUDE.md`: stack choices, "no JS build step", test commands, "content lives in /content".
- [ ] Give Claude Code one milestone per session, asking for a plan first and tests for the scheduler and content validator.
- [ ] Reuse kirkyards' Dockerfile, Cloud Build / deploy setup and Cloudflare config.

**Decisions**

- Users: just me at first. No sign-up for anyone else until a domain is bought. Google sign-in stays in OAuth "Testing" mode with me as the only test user.
- Devices: personal laptop for daily sessions; Android phone for reminders (and the odd session).
- Domain: start on the Cloud Run run.app URL; buy a domain only when opening sign-up. kirkyards.scot is not used for this app, even temporarily.
- Language variety: neutral French, Standard German, standard Italian.
- Content review: none for the MVP; consider a paid translation agency if the app takes off.
- Database: Neon free plan (serverless Postgres), behind a repository interface so moving later is a config change.
- Secrets: Google Secret Manager plus Workload Identity Federation; nothing secret in the repo.
- Repo: public on GitHub from day one as mmcnicol/phrasebuilder (Go module github.com/mmcnicol/phrasebuilder); MIT for code, CC BY-SA 4.0 for course content.
- Reminders: Web Push to phone and laptop first; email to myself via Gmail SMTP; provider email once there is a domain; no SMS.

**Open questions**

- [ ] None blocking milestone 1. Revisit the domain and sign-up once the MVP has been used daily for a month.
