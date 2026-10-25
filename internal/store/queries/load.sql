-- Writes used by `import load`. Each upsert also un-retires the row.

-- name: UpsertLanguage :batchexec
INSERT INTO languages (code, name) VALUES ($1, $2)
ON CONFLICT (code) DO UPDATE SET name = excluded.name;

-- name: UpsertUnit :batchexec
INSERT INTO units (id, language, position, title) VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO UPDATE SET
    language = excluded.language, position = excluded.position,
    title = excluded.title, retired_at = NULL;

-- name: UpsertPattern :batchexec
INSERT INTO patterns (id, unit_id, position, frame, en, note, status)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (id) DO UPDATE SET
    unit_id = excluded.unit_id, position = excluded.position,
    frame = excluded.frame, en = excluded.en, note = excluded.note,
    status = excluded.status, retired_at = NULL;

-- name: UpsertWord :batchexec
INSERT INTO words (id, language, text, en, gender, audio_path)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (id) DO UPDATE SET
    language = excluded.language, text = excluded.text, en = excluded.en,
    gender = excluded.gender, audio_path = excluded.audio_path, retired_at = NULL;

-- name: UpsertSentence :batchexec
INSERT INTO sentences (id, pattern_id, position, text, en, audio_path)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (id) DO UPDATE SET
    pattern_id = excluded.pattern_id, position = excluded.position,
    text = excluded.text, en = excluded.en,
    audio_path = excluded.audio_path, retired_at = NULL;

-- name: DeleteSentenceWords :exec
DELETE FROM sentence_words WHERE sentence_id = ANY(@sentence_ids::text[]);

-- name: InsertSentenceWord :batchexec
INSERT INTO sentence_words (sentence_id, slot, word_id) VALUES ($1, $2, $3);

-- name: RetireUnits :execrows
UPDATE units SET retired_at = now()
WHERE retired_at IS NULL AND NOT (id = ANY(@keep::text[]));

-- name: RetirePatterns :execrows
UPDATE patterns SET retired_at = now()
WHERE retired_at IS NULL AND NOT (id = ANY(@keep::text[]));

-- name: RetireWords :execrows
UPDATE words SET retired_at = now()
WHERE retired_at IS NULL AND NOT (id = ANY(@keep::text[]));

-- name: RetireSentences :execrows
UPDATE sentences SET retired_at = now()
WHERE retired_at IS NULL AND NOT (id = ANY(@keep::text[]));
