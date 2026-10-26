package srs

import (
	"testing"
	"time"

	"github.com/open-spaced-repetition/go-fsrs/v4"
)

var now = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

func card(id int64, due time.Duration) Card {
	return Card{ID: id, SentenceID: "s", Exercise: Recognise, Card: fsrs.Card{Due: now.Add(due), State: fsrs.Learning}}
}

func news(patterns ...string) []NewSentence {
	var ns []NewSentence
	for i, p := range patterns {
		started := p[0] == '+' // "+a": a sentence of an already started pattern a
		if started {
			p = p[1:]
		}
		ns = append(ns, NewSentence{SentenceID: p + string(rune('0'+i)), PatternID: p, PatternStarted: started})
	}
	return ns
}

func TestChoose(t *testing.T) {
	tests := []struct {
		name string
		s    Snapshot
		kind Kind
		id   string // card ID or sentence ID chosen
	}{
		{"nothing", Snapshot{}, Done, ""},
		{"due first", Snapshot{Due: []Card{card(1, -time.Hour), card(2, -time.Minute)}, New: news("a")}, Review, "1"},
		{"due now counts", Snapshot{Due: []Card{card(1, 0)}, New: news("a")}, Review, "1"},
		{"new before ahead", Snapshot{Due: []Card{card(1, 5*time.Minute)}, New: news("a")}, Learn, "a0"},
		{"ahead when no new left", Snapshot{Due: []Card{card(1, 5*time.Minute)}, NewToday: NewPerDay, New: news("a")}, Ahead, "1"},
		{"done when allowance used", Snapshot{NewToday: NewPerDay, New: news("a")}, Done, ""},
		{"pattern allowance used", Snapshot{NewToday: 2, PatternsStartedToday: NewPatternsPerDay, New: news("c")}, Done, ""},
		{"finish a started pattern", Snapshot{NewToday: 2, PatternsStartedToday: NewPatternsPerDay, New: news("+b", "c")}, Learn, "b0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.s.Now = now
			c := Choose(tt.s)
			if c.Kind != tt.kind {
				t.Fatalf("kind = %v, want %v", c.Kind, tt.kind)
			}
			var id string
			switch c.Kind {
			case Review, Ahead:
				id = string(rune('0' + c.Card.ID))
			case Learn:
				id = c.New.SentenceID
			}
			if id != tt.id {
				t.Errorf("chose %q, want %q", id, tt.id)
			}
		})
	}
}

func TestNewLeft(t *testing.T) {
	tests := []struct {
		name string
		s    Snapshot
		want int
	}{
		{"fresh day, two patterns", Snapshot{New: news("a", "b", "c", "c", "c")}, 2},
		{"started pattern then new ones", Snapshot{New: news("+a", "+a", "b", "b", "b", "b")}, NewPerDay},
		{"capped per day", Snapshot{New: news("a", "a", "a", "a", "a", "a", "a")}, NewPerDay},
		{"some used today", Snapshot{NewToday: 3, PatternsStartedToday: 1, New: news("+a", "+a", "+a")}, 2},
		{"pattern allowance used", Snapshot{NewToday: 2, PatternsStartedToday: NewPatternsPerDay, New: news("c", "c")}, 0},
		{"over the limit", Snapshot{NewToday: NewPerDay + 2, New: news("a")}, 0},
		{"course finished", Snapshot{}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewLeft(tt.s); got != tt.want {
				t.Errorf("NewLeft = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestDueNow(t *testing.T) {
	s := Snapshot{Now: now, Due: []Card{card(1, -time.Hour), card(2, 0), card(3, time.Minute)}}
	if got := DueNow(s); got != 2 {
		t.Errorf("DueNow = %d, want 2", got)
	}
}

func TestDayStart(t *testing.T) {
	london := func(y int, m time.Month, d, h, min int) time.Time { return time.Date(y, m, d, h, min, 0, 0, Zone) }
	tests := []struct {
		t, start, next time.Time
	}{
		{london(2026, 10, 20, 12, 0), london(2026, 10, 20, 4, 0), london(2026, 10, 21, 4, 0)},
		{london(2026, 10, 20, 3, 59), london(2026, 10, 19, 4, 0), london(2026, 10, 20, 4, 0)},
		{london(2026, 10, 20, 4, 0), london(2026, 10, 20, 4, 0), london(2026, 10, 21, 4, 0)},
		// Clocks go back at 2am on 25 October 2026: that day is 25 hours long.
		{london(2026, 10, 25, 1, 30), london(2026, 10, 24, 4, 0), london(2026, 10, 25, 4, 0)},
		{london(2026, 10, 25, 23, 0), london(2026, 10, 25, 4, 0), london(2026, 10, 26, 4, 0)},
		// Clocks go forward on 29 March 2026.
		{london(2026, 3, 29, 3, 30), london(2026, 3, 28, 4, 0), london(2026, 3, 29, 4, 0)},
	}
	for _, tt := range tests {
		if got := DayStart(tt.t.UTC()); !got.Equal(tt.start) {
			t.Errorf("DayStart(%v) = %v, want %v", tt.t, got, tt.start)
		}
		if got := NextDayStart(tt.t.UTC()); !got.Equal(tt.next) {
			t.Errorf("NextDayStart(%v) = %v, want %v", tt.t, got, tt.next)
		}
	}
	if got := london(2026, 10, 26, 4, 0).Sub(london(2026, 10, 25, 4, 0)); got != 24*time.Hour {
		t.Errorf("4am to 4am across the change = %v", got)
	}
}

func TestSchedulingANewCard(t *testing.T) {
	c := fsrs.NewCard(now)
	iv, err := Intervals(c, now)
	if err != nil {
		t.Fatal(err)
	}
	if iv[0] != time.Minute || iv[1] >= iv[3] || iv[2] >= iv[3] {
		t.Errorf("intervals for a new card = %v; want Again = 1m and Easy longest", iv)
	}

	// Good, Good, then Good a day later: the card graduates to Review.
	t1 := now
	for _, wait := range []time.Duration{0, 10 * time.Minute, 24 * time.Hour} {
		t1 = t1.Add(wait)
		if c, err = Next(c, t1, fsrs.Good); err != nil {
			t.Fatal(err)
		}
	}
	if c.State != fsrs.Review || c.Reps != 3 {
		t.Errorf("after three Goods: state %v, reps %d", c.State, c.Reps)
	}
}

func TestUnlocksProduce(t *testing.T) {
	review := func(days uint64) fsrs.Card { return fsrs.Card{State: fsrs.Review, ScheduledDays: days} }
	tests := []struct {
		ex    Exercise
		after fsrs.Card
		want  bool
	}{
		{Recognise, review(ProduceAfterDays), true},
		{Recognise, review(ProduceAfterDays - 1), false},
		{Recognise, fsrs.Card{State: fsrs.Learning}, false},
		{Recognise, fsrs.Card{State: fsrs.Relearning, ScheduledDays: 10}, false},
		{Produce, review(30), false},
	}
	for _, tt := range tests {
		if got := UnlocksProduce(tt.ex, tt.after); got != tt.want {
			t.Errorf("UnlocksProduce(%s, %+v) = %v", tt.ex, tt.after, got)
		}
	}
}
