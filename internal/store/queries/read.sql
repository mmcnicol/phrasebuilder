-- Reads used by the web pages. Retired rows are never shown.

-- name: ListUnits :many
SELECT u.id, u.language, u.position, u.title,
       count(p.id)::integer AS pattern_count
FROM units u
LEFT JOIN patterns p ON p.unit_id = u.id AND p.retired_at IS NULL
WHERE u.retired_at IS NULL
GROUP BY u.id
ORDER BY u.language, u.position;

-- name: GetUnit :one
SELECT id, language, position, title FROM units
WHERE id = $1 AND retired_at IS NULL;

-- name: ListUnitPatterns :many
SELECT id, frame, en, note, status FROM patterns
WHERE unit_id = $1 AND retired_at IS NULL
ORDER BY position;

-- name: ListUnitSentences :many
SELECT s.id, s.pattern_id, s.text, s.en, s.audio_path
FROM sentences s
JOIN patterns p ON p.id = s.pattern_id
WHERE p.unit_id = $1 AND p.retired_at IS NULL AND s.retired_at IS NULL
ORDER BY p.position, s.position;
