package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/open-spaced-repetition/go-fsrs/v4"

	"github.com/mmcnicol/phrasebuilder/internal/srs"
)

const (
	coffee = "fr/je-voudrais-noun/un-cafe"
	bill   = "fr/je-voudrais-noun/l-addition"
	toilet = "fr/ou-sont-les-toilettes"
)

func TestReviews(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if _, err := s.LoadCourse(ctx, course(t, unit), audioPath); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

	snap := mustSnapshot(t, s, now)
	if len(snap.Due) != 0 || len(snap.New) != 3 || snap.New[0].SentenceID != coffee || snap.New[0].PatternStarted {
		t.Fatalf("first snapshot = %+v", snap)
	}
	if n := srs.NewLeft(snap); n != 3 {
		t.Errorf("new left on day one = %d, want 3 (both patterns)", n)
	}

	it := mustItem(t, s, coffee, srs.Recognise, now)
	if it.ID != 0 || !it.NewPattern || it.Text != "Je voudrais un café, s'il vous plaît." || it.Note == "" || it.Language != "fr" {
		t.Errorf("new item = %+v", it)
	}

	// The first answer creates the card; repeating it changes nothing.
	first := Answer{UserID: 1, SentenceID: coffee, Rating: fsrs.Good, ElapsedMS: 3000, Now: now}
	mustAnswer(t, s, first, true)
	mustAnswer(t, s, first, false)

	snap = mustSnapshot(t, s, now)
	if snap.NewToday != 1 || snap.PatternsStartedToday != 1 || len(snap.Due) != 1 || len(snap.New) != 2 {
		t.Fatalf("after one answer: %+v", snap)
	}
	if !snap.New[0].PatternStarted {
		t.Error("pattern not started after its first sentence")
	}
	if mustItem(t, s, bill, srs.Recognise, now).NewPattern {
		t.Error("second sentence of a pattern introduces it again")
	}
	c := snap.Due[0]
	if c.SentenceID != coffee || c.Reps != 1 || c.State != fsrs.Learning || !c.Due.After(now) {
		t.Errorf("card after Good = %+v", c)
	}

	// A stale answer (shown before the card moved on) is ignored.
	mustAnswer(t, s, Answer{UserID: 1, CardID: c.ID, Reps: 0, Rating: fsrs.Again, Now: now.Add(time.Minute)}, false)
	// Another user's card is not found.
	mustAnswer(t, s, Answer{UserID: 2, CardID: c.ID, Reps: 1, Rating: fsrs.Again, Now: now.Add(time.Minute)}, false)

	// Keep answering Good when due until the Produce card unlocks.
	when := now
	var unlockedAt time.Time
	for i := 0; i < 10 && unlockedAt.IsZero(); i++ {
		c = mustItem(t, s, coffee, srs.Recognise, when).Card
		when = c.Due
		mustAnswer(t, s, Answer{UserID: 1, CardID: c.ID, Reps: int(c.Reps), Rating: fsrs.Good, Now: when}, true)
		if p := mustItem(t, s, coffee, srs.Produce, when); p.ID != 0 {
			unlockedAt = when
			if !p.Due.Equal(srs.NextDayStart(when)) || p.Reps != 0 {
				t.Errorf("produce card = %+v, want new and due %v", p.Card, srs.NextDayStart(when))
			}
		}
	}
	if unlockedAt.IsZero() {
		t.Fatal("produce card never unlocked")
	}
	if c := mustItem(t, s, coffee, srs.Recognise, when); c.ScheduledDays < srs.ProduceAfterDays {
		t.Errorf("unlocked at a %d-day interval", c.ScheduledDays)
	}

	// Undo removes the unlocked Produce card and restores the Recognise card.
	reps := mustItem(t, s, coffee, srs.Recognise, when).Reps
	u, ok, err := s.Undo(ctx, 1, when)
	if err != nil || !ok || u != (Undone{SentenceID: coffee, Exercise: srs.Recognise}) {
		t.Fatalf("undo = %+v, %v, %v", u, ok, err)
	}
	if p := mustItem(t, s, coffee, srs.Produce, when); p.ID != 0 {
		t.Error("undo left the produce card")
	}
	if got := mustItem(t, s, coffee, srs.Recognise, when).Reps; got != reps-1 {
		t.Errorf("reps after undo = %d, want %d", got, reps-1)
	}

	// Undoing a new sentence's first answer deletes its card.
	mustAnswer(t, s, Answer{UserID: 1, SentenceID: bill, Rating: fsrs.Hard, Now: when}, true)
	if _, ok, err := s.Undo(ctx, 1, when); !ok || err != nil {
		t.Fatalf("undo = %v, %v", ok, err)
	}
	if it := mustItem(t, s, bill, srs.Recognise, when); it.ID != 0 {
		t.Error("undo kept the new card")
	}

	// Only today's answers can be undone; the last earlier one was yesterday.
	if _, ok, _ := s.Undo(ctx, 1, srs.NextDayStart(when).Add(time.Hour)); ok {
		t.Error("undid an answer from another day")
	}

	// Both of today's answers were undone.
	today, err := s.Today(ctx, 1, when)
	if err != nil || today.Reviews != 0 || today.NextDue.IsZero() {
		t.Errorf("today = %+v, %v", today, err)
	}
	if first, err := s.Today(ctx, 1, now); err != nil || first.NewSentences != 1 || first.Elapsed < 3*time.Second {
		t.Errorf("day one = %+v, %v", first, err)
	}

	// Retiring a sentence hides its card.
	mustAnswer(t, s, Answer{UserID: 1, SentenceID: toilet, Rating: fsrs.Again, Now: when}, true)
	smaller := strings.Split(unit, "  - id: ou-sont")[0]
	if _, err := s.LoadCourse(ctx, course(t, smaller), audioPath); err != nil {
		t.Fatal(err)
	}
	for _, c := range mustSnapshot(t, s, when.Add(time.Hour)).Due {
		if c.SentenceID == toilet {
			t.Error("retired sentence's card is due")
		}
	}
}

func TestReports(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if _, err := s.LoadCourse(ctx, course(t, unit), audioPath); err != nil {
		t.Fatal(err)
	}
	if err := s.AddReport(ctx, 1, coffee, "Should be « un café »?"); err != nil {
		t.Fatal(err)
	}
	reports, err := s.OpenReports(ctx)
	if err != nil || len(reports) != 1 || reports[0].Unit != "fr/cafe" || reports[0].Text == "" {
		t.Fatalf("reports = %+v, %v", reports, err)
	}
	if err := s.ResolveReport(ctx, reports[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ResolveReport(ctx, reports[0].ID); err != ErrNotFound {
		t.Errorf("resolving twice: err = %v, want ErrNotFound", err)
	}
	if reports, _ := s.OpenReports(ctx); len(reports) != 0 {
		t.Errorf("open reports after resolving = %+v", reports)
	}
}

func mustSnapshot(t *testing.T, s *Store, now time.Time) srs.Snapshot {
	t.Helper()
	snap, err := s.Snapshot(context.Background(), 1, now)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func mustItem(t *testing.T, s *Store, sentence string, ex srs.Exercise, now time.Time) ReviewItem {
	t.Helper()
	it, err := s.Item(context.Background(), 1, sentence, ex, now)
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func mustAnswer(t *testing.T, s *Store, a Answer, want bool) {
	t.Helper()
	applied, err := s.Answer(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	if applied != want {
		t.Fatalf("answer %+v applied = %v, want %v", a, applied, want)
	}
}
