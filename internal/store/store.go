// Package store keeps course content (and later, learner state) in Postgres.
//
// Queries are in queries/*.sql and the Go code for them in db/ is generated
// by sqlc (run `sqlc generate` after changing either). Migrations are in
// migrations/*.sql and run by goose.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver "pgx", for goose
	"github.com/pressly/goose/v3"

	"github.com/mmcnicol/phrasebuilder/internal/content"
	"github.com/mmcnicol/phrasebuilder/internal/store/db"
)

//go:embed migrations/*.sql
var migrations embed.FS

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("not found")

// Store is a Postgres-backed store.
type Store struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// Open connects lazily: it does not contact the database, so a server can
// start while a serverless database (Neon) is still asleep.
func Open(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool, q: db.New(pool)}, nil
}

// Close closes the store's connections.
func (s *Store) Close() { s.pool.Close() }

// Migrate brings the database schema up to date.
func Migrate(ctx context.Context, url string) ([]string, error) {
	sqlDB, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
	defer sqlDB.Close()
	fsys, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return nil, err
	}
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, fsys)
	if err != nil {
		return nil, err
	}
	results, err := p.Up(ctx)
	var applied []string
	for _, r := range results {
		applied = append(applied, r.Source.Path)
	}
	return applied, err
}

// LoadStats counts what LoadCourse changed.
type LoadStats struct {
	Units, Patterns, Words, Sentences int   // upserted
	Retired                           int64 // rows no longer in the content
}

// LoadCourse upserts all of c in one transaction and retires rows that are no
// longer in it. audioPath gives the audio file for a text in a language.
// Loading the same course twice changes nothing.
func (s *Store) LoadCourse(ctx context.Context, c *content.Course, audioPath func(language, text string) string) (LoadStats, error) {
	var st LoadStats
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)

		langs := make([]db.UpsertLanguageParams, 0, len(content.Languages))
		for _, code := range sortedKeys(content.Languages) {
			langs = append(langs, db.UpsertLanguageParams{Code: code, Name: content.Languages[code]})
		}
		if err := batch(q.UpsertLanguage(ctx, langs)); err != nil {
			return fmt.Errorf("languages: %w", err)
		}

		var units []db.UpsertUnitParams
		var unitIDs []string
		for _, u := range c.Units {
			units = append(units, db.UpsertUnitParams{ID: u.ID, Language: u.Language, Position: int32(u.Position), Title: u.Title})
			unitIDs = append(unitIDs, u.ID)
		}
		if err := batch(q.UpsertUnit(ctx, units)); err != nil {
			return fmt.Errorf("units: %w", err)
		}

		var patterns []db.UpsertPatternParams
		var patternIDs []string
		for _, p := range c.Patterns {
			patterns = append(patterns, db.UpsertPatternParams{
				ID: p.ID, UnitID: p.UnitID, Position: int32(p.Position),
				Frame: p.Frame, En: p.En, Note: p.Note, Status: p.Status,
			})
			patternIDs = append(patternIDs, p.ID)
		}
		if err := batch(q.UpsertPattern(ctx, patterns)); err != nil {
			return fmt.Errorf("patterns: %w", err)
		}

		var words []db.UpsertWordParams
		var wordIDs []string
		for _, w := range c.Words {
			words = append(words, db.UpsertWordParams{
				ID: w.ID, Language: w.Language, Text: w.Text, En: w.En,
				Gender: w.Gender, AudioPath: audioPath(w.Language, w.Text),
			})
			wordIDs = append(wordIDs, w.ID)
		}
		if err := batch(q.UpsertWord(ctx, words)); err != nil {
			return fmt.Errorf("words: %w", err)
		}

		unitLang := map[string]string{}
		for _, u := range c.Units {
			unitLang[u.ID] = u.Language
		}
		lang := map[string]string{} // pattern ID -> language
		for _, p := range c.Patterns {
			lang[p.ID] = unitLang[p.UnitID]
		}
		var sentences []db.UpsertSentenceParams
		var sentenceIDs []string
		var sentenceWords []db.InsertSentenceWordParams
		for _, sn := range c.Sentences {
			sentences = append(sentences, db.UpsertSentenceParams{
				ID: sn.ID, PatternID: sn.PatternID, Position: int32(sn.Position),
				Text: sn.Text, En: sn.En, AudioPath: audioPath(lang[sn.PatternID], sn.Text),
			})
			sentenceIDs = append(sentenceIDs, sn.ID)
			for _, sw := range sn.Words {
				sentenceWords = append(sentenceWords, db.InsertSentenceWordParams{SentenceID: sn.ID, Slot: sw.Slot, WordID: sw.WordID})
			}
		}
		if err := batch(q.UpsertSentence(ctx, sentences)); err != nil {
			return fmt.Errorf("sentences: %w", err)
		}
		if err := q.DeleteSentenceWords(ctx, sentenceIDs); err != nil {
			return fmt.Errorf("sentence words: %w", err)
		}
		if err := batch(q.InsertSentenceWord(ctx, sentenceWords)); err != nil {
			return fmt.Errorf("sentence words: %w", err)
		}

		for _, r := range []struct {
			name   string
			retire func(context.Context, []string) (int64, error)
			keep   []string
		}{
			{"sentences", q.RetireSentences, sentenceIDs},
			{"patterns", q.RetirePatterns, patternIDs},
			{"units", q.RetireUnits, unitIDs},
			{"words", q.RetireWords, wordIDs},
		} {
			n, err := r.retire(ctx, nonNil(r.keep))
			if err != nil {
				return fmt.Errorf("retire %s: %w", r.name, err)
			}
			st.Retired += n
		}
		st = LoadStats{Units: len(units), Patterns: len(patterns), Words: len(words), Sentences: len(sentences), Retired: st.Retired}
		return nil
	})
	return st, err
}

// batchResults is implemented by every sqlc :batchexec result.
type batchResults interface {
	Exec(func(int, error))
	Close() error
}

// batch runs a sqlc batch and returns its first error.
func batch(b batchResults) error {
	var first error
	b.Exec(func(i int, err error) {
		if err != nil && first == nil {
			first = fmt.Errorf("row %d: %w", i, err)
		}
	})
	if err := b.Close(); err != nil && first == nil {
		first = err
	}
	return first
}

// Unit is a unit as listed on the site.
type Unit struct {
	ID           string // e.g. fr/cafe
	Language     string
	Position     int
	Title        string
	PatternCount int
}

// Pattern is a pattern with its sentences, for a unit page.
type Pattern struct {
	ID, Frame, En, Note, Status string
	Sentences                   []Sentence
}

// Sentence is one generated sentence.
type Sentence struct {
	ID, Text, En, AudioPath string
}

// Units lists the current units, by language and position.
func (s *Store) Units(ctx context.Context) ([]Unit, error) {
	rows, err := s.q.ListUnits(ctx)
	if err != nil {
		return nil, err
	}
	units := make([]Unit, len(rows))
	for i, r := range rows {
		units[i] = Unit{ID: r.ID, Language: r.Language, Position: int(r.Position), Title: r.Title, PatternCount: int(r.PatternCount)}
	}
	return units, nil
}

// Unit returns a unit with its patterns and sentences, or ErrNotFound.
func (s *Store) Unit(ctx context.Context, id string) (Unit, []Pattern, error) {
	u, err := s.q.GetUnit(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Unit{}, nil, ErrNotFound
	}
	if err != nil {
		return Unit{}, nil, err
	}
	prows, err := s.q.ListUnitPatterns(ctx, id)
	if err != nil {
		return Unit{}, nil, err
	}
	srows, err := s.q.ListUnitSentences(ctx, id)
	if err != nil {
		return Unit{}, nil, err
	}
	patterns := make([]Pattern, len(prows))
	index := make(map[string]int, len(prows))
	for i, p := range prows {
		patterns[i] = Pattern{ID: p.ID, Frame: p.Frame, En: p.En, Note: p.Note, Status: p.Status}
		index[p.ID] = i
	}
	for _, r := range srows {
		if i, ok := index[r.PatternID]; ok {
			patterns[i].Sentences = append(patterns[i].Sentences, Sentence{ID: r.ID, Text: r.Text, En: r.En, AudioPath: r.AudioPath})
		}
	}
	unit := Unit{ID: u.ID, Language: u.Language, Position: int(u.Position), Title: u.Title, PatternCount: len(patterns)}
	return unit, patterns, nil
}

// nonNil makes sure an empty list is sent as an empty array, not NULL, so
// "NOT (id = ANY(keep))" retires everything rather than nothing.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
