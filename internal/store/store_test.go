package store

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/url"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"

	"github.com/mmcnicol/phrasebuilder/internal/content"
)

// testStore returns a store in a fresh, migrated schema that is dropped when
// the test ends. It needs TEST_DATABASE_URL (a direct, not pooled, Neon dev
// branch URL); without it the test is skipped.
func testStore(t *testing.T) *Store {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	schema := fmt.Sprintf("test_%d", rand.Uint32())

	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close(context.Background())
	})

	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()

	if _, err := Migrate(ctx, u.String()); err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

const unit = `unit: cafe
order: 1
title: At the café
patterns:
  - id: je-voudrais-noun
    status: draft
    frame: "Je voudrais {item}, s'il vous plaît."
    en: "I would like {item}, please."
    note: "Je voudrais = I would like."
    slots:
      item:
        - { fr: "un café", en: "a coffee", g: m }
        - { fr: "l'addition", en: "the bill", g: f }
  - id: ou-sont-les-toilettes
    status: draft
    frame: "Où sont les toilettes ?"
    en: "Where are the toilets?"
    note: "Où = where."
`

func course(t *testing.T, yaml string) *content.Course {
	t.Helper()
	units, problems := content.Load(fstest.MapFS{"fr/unit-01-cafe.yaml": {Data: []byte(yaml)}})
	c, more := content.Build(units)
	if problems = append(problems, more...); len(problems) > 0 {
		t.Fatal(problems)
	}
	return c
}

func audioPath(lang, text string) string { return lang + "/" + content.Slug(text) + ".mp3" }

func TestLoadCourse(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	st, err := s.LoadCourse(ctx, course(t, unit), audioPath)
	if err != nil {
		t.Fatal(err)
	}
	if want := (LoadStats{Units: 1, Patterns: 2, Words: 2, Sentences: 3}); st != want {
		t.Errorf("first load = %+v, want %+v", st, want)
	}

	// Loading again changes nothing.
	if st, err = s.LoadCourse(ctx, course(t, unit), audioPath); err != nil || st.Retired != 0 {
		t.Errorf("second load = %+v, %v; want nothing retired", st, err)
	}

	units, err := s.Units(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 || units[0] != (Unit{ID: "fr/cafe", Language: "fr", Position: 1, Title: "At the café", PatternCount: 2}) {
		t.Errorf("units = %+v", units)
	}

	u, patterns, err := s.Unit(ctx, "fr/cafe")
	if err != nil {
		t.Fatal(err)
	}
	if u.Title != "At the café" || len(patterns) != 2 || len(patterns[0].Sentences) != 2 {
		t.Fatalf("unit = %+v, patterns = %+v", u, patterns)
	}
	want := Sentence{ID: "fr/je-voudrais-noun/l-addition", Text: "Je voudrais l'addition, s'il vous plaît.", En: "I would like the bill, please.", AudioPath: "fr/je-voudrais-l-addition-s-il-vous-plait.mp3"}
	if got := patterns[0].Sentences[1]; got != want {
		t.Errorf("sentence = %+v, want %+v", got, want)
	}

	// Removing a pattern and a filler retires them (pattern, its sentence,
	// a sentence and a word) without deleting anything.
	smaller := strings.Split(unit, "  - id: ou-sont")[0]
	smaller = strings.Replace(smaller, "        - { fr: \"l'addition\", en: \"the bill\", g: f }\n", "", 1)
	if st, err = s.LoadCourse(ctx, course(t, smaller), audioPath); err != nil || st.Retired != 4 {
		t.Errorf("smaller load = %+v, %v; want 4 retired", st, err)
	}
	_, patterns, err = s.Unit(ctx, "fr/cafe")
	if err != nil || len(patterns) != 1 || len(patterns[0].Sentences) != 1 {
		t.Errorf("after retiring: patterns = %+v, %v", patterns, err)
	}

	// Putting them back un-retires them.
	if _, err = s.LoadCourse(ctx, course(t, unit), audioPath); err != nil {
		t.Fatal(err)
	}
	if _, patterns, _ = s.Unit(ctx, "fr/cafe"); len(patterns) != 2 {
		t.Errorf("after restoring: %d patterns, want 2", len(patterns))
	}

	if _, _, err := s.Unit(ctx, "fr/nope"); err != ErrNotFound {
		t.Errorf("missing unit: err = %v, want ErrNotFound", err)
	}
}
