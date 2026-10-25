-- Course content, loaded from content/*.yaml by `go run ./cmd/import load`.
-- IDs are stable slugs such as fr/cafe, so learner progress (later
-- migrations) survives re-imports. Rows removed from the YAML are retired,
-- never deleted.

-- +goose Up
CREATE TABLE languages (
    code text PRIMARY KEY,
    name text NOT NULL
);

CREATE TABLE units (
    id         text PRIMARY KEY,
    language   text NOT NULL REFERENCES languages (code),
    position   integer NOT NULL,
    title      text NOT NULL,
    retired_at timestamptz
);

CREATE TABLE patterns (
    id         text PRIMARY KEY,
    unit_id    text NOT NULL REFERENCES units (id),
    position   integer NOT NULL,
    frame      text NOT NULL,
    en         text NOT NULL,
    note       text NOT NULL,
    status     text NOT NULL CHECK (status IN ('draft', 'checked')),
    retired_at timestamptz
);
CREATE INDEX patterns_unit_id ON patterns (unit_id);

CREATE TABLE words (
    id         text PRIMARY KEY,
    language   text NOT NULL REFERENCES languages (code),
    text       text NOT NULL,
    en         text NOT NULL,
    gender     text NOT NULL CHECK (gender IN ('', 'm', 'f')),
    audio_path text NOT NULL,
    retired_at timestamptz
);

CREATE TABLE sentences (
    id         text PRIMARY KEY,
    pattern_id text NOT NULL REFERENCES patterns (id),
    position   integer NOT NULL,
    text       text NOT NULL,
    en         text NOT NULL,
    audio_path text NOT NULL,
    retired_at timestamptz
);
CREATE INDEX sentences_pattern_id ON sentences (pattern_id);

CREATE TABLE sentence_words (
    sentence_id text NOT NULL REFERENCES sentences (id),
    slot        text NOT NULL,
    word_id     text NOT NULL REFERENCES words (id),
    PRIMARY KEY (sentence_id, slot)
);
CREATE INDEX sentence_words_word_id ON sentence_words (word_id);

-- +goose Down
DROP TABLE sentence_words;
DROP TABLE sentences;
DROP TABLE words;
DROP TABLE patterns;
DROP TABLE units;
DROP TABLE languages;
