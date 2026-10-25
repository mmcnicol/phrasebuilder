-- Reviews: what is due, answering, undo, and problem reports. Cards of
-- retired sentences are never shown.

-- name: ListDueCards :many
SELECT c.* FROM cards c
JOIN sentences s ON s.id = c.sentence_id
JOIN patterns p ON p.id = s.pattern_id
WHERE c.user_id = @user_id AND c.due <= @until
  AND s.retired_at IS NULL AND p.retired_at IS NULL
ORDER BY c.due, c.id;

-- name: CountNewToday :one
-- Sentences first answered since @since, and the patterns they started.
SELECT
    (SELECT count(*) FROM cards n
     WHERE n.user_id = @user_id AND n.exercise = 'recognise' AND n.created_at >= @since)::integer AS sentences,
    (SELECT count(*) FROM (
        SELECT s.pattern_id FROM cards c
        JOIN sentences s ON s.id = c.sentence_id
        WHERE c.user_id = @user_id AND c.exercise = 'recognise'
        GROUP BY s.pattern_id
        HAVING min(c.created_at) >= @since
    ) started)::integer AS patterns;

-- name: ListNewSentences :many
-- The next sentences the user has not seen, in course order.
SELECT s.id, s.pattern_id,
       EXISTS (
           SELECT 1 FROM cards c
           JOIN sentences s2 ON s2.id = c.sentence_id
           WHERE c.user_id = @user_id AND s2.pattern_id = s.pattern_id
       ) AS pattern_started
FROM sentences s
JOIN patterns p ON p.id = s.pattern_id
JOIN units u ON u.id = p.unit_id
WHERE s.retired_at IS NULL AND p.retired_at IS NULL AND u.retired_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM cards c
      WHERE c.user_id = @user_id AND c.sentence_id = s.id AND c.exercise = 'recognise'
  )
ORDER BY u.language, u.position, p.position, s.position
LIMIT @max;

-- name: GetReviewSentence :one
SELECT s.id, s.text, s.en, s.audio_path,
       p.id AS pattern_id, p.frame, p.en AS pattern_en, p.note, p.status,
       u.id AS unit_id, u.language
FROM sentences s
JOIN patterns p ON p.id = s.pattern_id
JOIN units u ON u.id = p.unit_id
WHERE s.id = $1;

-- name: PatternStarted :one
SELECT EXISTS (
    SELECT 1 FROM cards c
    JOIN sentences s ON s.id = c.sentence_id
    WHERE c.user_id = @user_id AND s.pattern_id = @pattern_id
);

-- name: GetCardBySentence :one
SELECT * FROM cards
WHERE user_id = @user_id AND sentence_id = @sentence_id AND exercise = @exercise;

-- name: LockCard :one
SELECT * FROM cards WHERE id = @id AND user_id = @user_id FOR UPDATE;

-- name: InsertCard :one
-- Returns no row if the card already exists (a repeated answer).
INSERT INTO cards (user_id, sentence_id, exercise, due, stability, difficulty,
                   scheduled_days, reps, lapses, state, last_review, remaining_steps, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT (user_id, sentence_id, exercise) DO NOTHING
RETURNING id;

-- name: UpdateCard :exec
UPDATE cards SET due = $2, stability = $3, difficulty = $4, scheduled_days = $5,
    reps = $6, lapses = $7, state = $8, last_review = $9, remaining_steps = $10
WHERE id = $1;

-- name: DeleteCard :exec
DELETE FROM cards WHERE id = $1;

-- name: InsertReview :exec
INSERT INTO review_log (user_id, card_id, rating, reviewed_at, elapsed_ms, created_card, unlocked_card_id,
    before_due, before_stability, before_difficulty, before_scheduled_days, before_reps,
    before_lapses, before_state, before_last_review, before_remaining_steps)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16);

-- name: LockLastReview :one
SELECT r.*, c.sentence_id, c.exercise FROM review_log r
JOIN cards c ON c.id = r.card_id
WHERE r.user_id = @user_id AND r.reviewed_at >= @since
ORDER BY r.id DESC
LIMIT 1
FOR UPDATE OF r;

-- name: DeleteReview :exec
DELETE FROM review_log WHERE id = $1;

-- name: DeleteUnreviewedCard :exec
DELETE FROM cards WHERE id = $1 AND reps = 0;

-- name: SummariseReviews :one
SELECT count(*)::integer AS reviews,
       count(*) FILTER (WHERE created_card)::integer AS new_sentences,
       coalesce(sum(elapsed_ms), 0)::bigint AS elapsed_ms
FROM review_log
WHERE user_id = @user_id AND reviewed_at >= @since;

-- name: NextDue :one
SELECT min(c.due)::timestamptz FROM cards c
JOIN sentences s ON s.id = c.sentence_id
JOIN patterns p ON p.id = s.pattern_id
WHERE c.user_id = @user_id AND s.retired_at IS NULL AND p.retired_at IS NULL;

-- name: InsertReport :exec
INSERT INTO problem_reports (user_id, sentence_id, note) VALUES ($1, $2, $3);

-- name: ListOpenReports :many
SELECT r.id, r.note, r.created_at, s.id AS sentence_id, s.text, s.en, p.unit_id
FROM problem_reports r
JOIN sentences s ON s.id = r.sentence_id
JOIN patterns p ON p.id = s.pattern_id
WHERE r.resolved_at IS NULL
ORDER BY r.created_at;

-- name: ResolveReport :execrows
UPDATE problem_reports SET resolved_at = now() WHERE id = $1 AND resolved_at IS NULL;
