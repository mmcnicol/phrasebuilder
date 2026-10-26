# Milestone 3 – Review loop

**Status: done (2026-10-26).** Live at
<https://phrasebuilder-532278562214.europe-west1.run.app/review>, revision
`phrasebuilder-00003-x45`. Changes from the plan:

- **2 new patterns a day**, not 1 (still at most 5 new sentences). Several
  café patterns have only one or two sentences, so 1 pattern a day gave a
  first session of 2 new sentences.
- A Produce card starts the day after it unlocks, not straight away, so the
  same sentence doesn't come up twice in a row.
- Undo restores the card from the state saved in `review_log`, rather than
  `fsrs.Rollback`, and only reaches back to today's answers.
- "Done for today" is shown in place on `/review`, not on a separate page.
- `users` has only `id` and `created_at`; display name and time zone wait
  until something uses them (days start at 4am Europe/London for now).
- Cross-site posts are blocked by Go's `http.CrossOriginProtection`, as well
  as the SameSite cookie.
- The passphrase is part of the cookie-signing key, so changing it logs every
  device out.

The plan as written:

Goal: a daily session you actually use. Press **Start**, review what's due,
meet a few new sentences, done in about 15 minutes, on laptop or phone.
Recognise and Produce exercises, scheduled by FSRS, for one user (you).
Login is milestone 4.

## Outcome

- `/review`: one card at a time.
  - Show the prompt, then reveal the answer.
  - Grade with Again / Hard / Good / Easy. Each button shows its next interval ("Good · 3d").
  - The next card swaps in via htmx.
- Session builder: due cards first, then today's new sentences, then "done for today" with a short summary.
- New patterns are introduced with their frame and grammar note before their first sentence.
- Undo the last answer, for mis-taps on the phone.
- "Report a problem" on any card, plus a `/reports` list.
- Protected by a passphrase, because the site is public. `/units` stays public.

## 1. Exercises

| Exercise | Prompt | Answer revealed |
|---|---|---|
| **Recognise** | French text, audio plays | English, pattern frame, grammar note |
| **Produce** | English: say the French **out loud** | French text, audio plays, note |

- Reveal happens in the browser (no request); grading is a POST.
- Keyboard shortcuts on the laptop: Space reveals, 1–4 grade, U undoes.
- The time from reveal to grade is recorded as `elapsed_ms` for future FSRS tuning.
- Audio: the play button and app.js already exist. Autoplay works after the first tap (Start), because htmx swaps keep the same page.

**Graduation:** a new sentence gets a Recognise card only. Its Produce card
is created once the Recognise card's interval reaches 3 days, meaning you've
recalled it on a couple of separate days. After that, the two have
independent schedules.

**Cards are per sentence**, not per word. Every word already lives inside
sentences, with audio. Word cards (your old paper cards) can be added later
with a migration if sentences alone don't stick.

## 2. Scheduling (`internal/srs`)

- go-fsrs **v4.0.0** with default parameters: 90% target retention, learning steps of 1 and 10 minutes, relearning step of 10 minutes, interval fuzz on.
- The card row maps 1:1 onto `fsrs.Card`.
- **Next card is computed from the database on every request**: no session state, so a reboot, a phone switch or a closed tab loses nothing. In order:
  1. A due card (`due ≤ now`), earliest first.
  2. Otherwise, if today's new allowance is left, the next new sentence in course order (unit, then pattern, then sentence).
  3. Otherwise, a learning card due within the next 20 minutes, shown early (as Anki does), so a session doesn't stall on 10-minute steps.
  4. Otherwise, done.
- **New allowance:** at most **5 new sentences a day**, from at most **1 new pattern a day**.
  - A partly introduced pattern is finished before the next one starts.
  - The proposal's "1 new pattern with 3–5 new words" maps to roughly this.
  - These are constants until the phase 2 settings page.
- **The day starts at 4am Europe/London**, so a late-night session counts as that day. DST is handled with embedded tzdata.
- Retired sentences' cards are never shown.
- **Answering is idempotent:** the form carries the card's `reps` as a version, so a double tap or a retry can't grade twice.
- **Undo:** `fsrs.Rollback` restores the card from the latest review log entry. If the card was brand new, it's deleted.

The decision logic is a pure function over a snapshot (due cards, today's
counts, next new sentences, now), so it can be tested without a database.

## 3. Database (migration 00002)

| Table | Columns |
|---|---|
| `users` | id, display_name, timezone, created_at. Row 1 is you. Milestone 4 adds email and Google sign-in to this same row, so your history carries over. |
| `cards` | id, user_id, sentence_id, exercise (recognise/produce), due, stability, difficulty, scheduled_days, reps, lapses, state, last_review, remaining_steps, created_at. Unique (user_id, sentence_id, exercise). |
| `review_log` | id, card_id, user_id, rating, reviewed_at, elapsed_ms, plus the card's state before the review (from `fsrs.ReviewLog`), enough for undo and future FSRS tuning |
| `problem_reports` | id, user_id, sentence_id, note, created_at, resolved_at |

- `cards.sentence_id` references `sentences`, whose rows are never deleted, so progress survives content edits.
- Streak and progress statistics (milestone 5) can be derived from `review_log`. The proposal's `daily_activity` table waits until it's needed.

## 4. Access (until milestone 4)

- `/login` asks for a passphrase. If it's right, the app sets a signed cookie (HMAC-SHA256, `HttpOnly`, `Secure`, `SameSite=Lax`, 1 year), so the laptop and phone each log in once.
- Two new secrets in Secret Manager, both readable by `phrasebuilder-run` only:
  - `app-passphrase`: you choose it; it goes in `.env`, and I create the secret from there without printing it.
  - `session-key`: 32 random bytes, generated and stored without anyone seeing it.
- `/review`, `/reports` and the POSTs need the cookie. `/units`, `/health` and static files don't.
- `SameSite=Lax` stops other sites posting with your cookie. Milestone 4 swaps the passphrase for Google sign-in and keeps the cookie.
- Login attempts are rate-limited in memory: the passphrase is the only lock, so it should be long.

## 5. Pages

- `/review`: card area plus "12 due · 3 new left" counter.
  - Large buttons at the bottom of the screen, for thumbs.
  - Answers swap in with `hx-post`; no full page reloads.
- `/review/done`: today's totals (reviews, new, minutes) and when the next card is due.
- Home page: a **Start** button and today's counts once logged in.
- `/reports`: open problem reports, each linking to its unit page, with a "resolved" button.

## 6. Tests

- **srs:** next-card choice (due order, new allowance and the one-pattern rule, learn-ahead window, retired sentences skipped); graduation to Produce; the 4am day boundary across a DST change; idempotent answers; undo of a new card and of a reviewed card.
- **store:** cards and review log round-trip through the temporary Neon database, `fsrs.Card` ↔ row conversion, unique constraint.
- **web:** login (wrong passphrase, cookie tampering, rate limit), review pages with a fake service, the double-submit guard, auth on every protected route.
- CI as now, with DB tests skipped there.

## Build order (each step committed)

1. Migration 00002, sqlc queries, and card ↔ `fsrs.Card` conversion.
2. `internal/srs`: session builder, answer, undo, and tests.
3. Login: passphrase, signed cookie, middleware, and tests.
4. Review pages, app.js shortcuts, done page, and reports.
5. Secrets, migrate production, deploy, then **your first real session** on laptop and phone.

## Needs you

- **Passphrase:** pick a long one (four or five random words) and add `APP_PASSPHRASE="…"` to `.env`.
- **Try it:** do a session on the laptop and on the phone and tell me what feels wrong. Expect to tune the new-sentence limit and graduation threshold after a week of use.

## Deferred

- Word cards.
- Settings page (new-per-day, session length): phase 2.
- Streak and progress page: milestone 5.
- Build, Swap and Type exercises: milestones 5–6.
- FSRS parameter tuning from your own review log: after a month or two of data.
- Daily reminders: phase 2.
- Passkeys: after Google sign-in and once a domain is chosen (a passkey is
  tied to its domain), added from a signed-in session with Google as the
  recovery route.
