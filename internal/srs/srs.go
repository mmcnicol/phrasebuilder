// Package srs decides what to review next and schedules answers with FSRS.
//
// It holds no state and does no I/O: callers pass in a Snapshot of the
// learner's cards and the current time, so the rules can be tested without
// a database.
package srs

import (
	"time"
	_ "time/tzdata" // Europe/London on hosts without zoneinfo (distroless)

	"github.com/open-spaced-repetition/go-fsrs/v4"
)

// Limits on new material. Constants until there is a settings page.
const (
	NewPerDay         = 5                // new sentences a day
	NewPatternsPerDay = 2                // patterns those sentences may start (some have only 1–2 sentences)
	LearnAhead        = 20 * time.Minute // show learning cards this early once nothing else is left
	ProduceAfterDays  = 3                // Recognise interval that unlocks a sentence's Produce card
	dayStartHour      = 4                // a late-night session counts as the day before
)

// Zone is where days start and end.
var Zone = mustLoadLocation("Europe/London")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// Exercise is a kind of card.
type Exercise string

const (
	Recognise Exercise = "recognise" // French shown, recall the English
	Produce   Exercise = "produce"   // English shown, say the French
)

// Card is a learner's card for one sentence and exercise. ID is 0 for a
// sentence not seen yet.
type Card struct {
	ID         int64
	SentenceID string
	Exercise   Exercise
	fsrs.Card
}

// NewSentence is a sentence the learner has not seen yet.
type NewSentence struct {
	SentenceID, PatternID string
	// PatternStarted means the learner has seen another sentence of this
	// pattern, so it needs no introduction.
	PatternStarted bool
}

// Snapshot is what Choose needs to know about one learner.
type Snapshot struct {
	Now time.Time
	// Due holds cards due by Now+LearnAhead, earliest first.
	Due []Card
	// NewToday counts sentences first answered today, and
	// PatternsStartedToday the patterns they started.
	NewToday, PatternsStartedToday int
	// New holds the next unseen sentences in course order, at least
	// NewPerDay of them if there are that many.
	New []NewSentence
}

// Kind says what Choose picked.
type Kind int

const (
	Done   Kind = iota // nothing left today
	Review             // a due card
	Learn              // a new sentence
	Ahead              // a learning card that is not quite due
)

// Choice is the next thing to show.
type Choice struct {
	Kind Kind
	Card Card        // for Review and Ahead
	New  NewSentence // for Learn
}

// Choose picks the next item: a due card, else a new sentence while
// today's allowance lasts, else a card due within LearnAhead, else Done.
func Choose(s Snapshot) Choice {
	if len(s.Due) > 0 && !s.Due[0].Due.After(s.Now) {
		return Choice{Kind: Review, Card: s.Due[0]}
	}
	if NewLeft(s) > 0 {
		return Choice{Kind: Learn, New: s.New[0]}
	}
	if len(s.Due) > 0 {
		return Choice{Kind: Ahead, Card: s.Due[0]}
	}
	return Choice{Kind: Done}
}

// DueNow counts the cards due now.
func DueNow(s Snapshot) int {
	n := 0
	for _, c := range s.Due {
		if !c.Due.After(s.Now) {
			n++
		}
	}
	return n
}

// NewLeft counts the new sentences still allowed today: at most NewPerDay,
// and none from a pattern beyond the NewPatternsPerDay started today. A
// partly seen pattern comes first in course order, so it is finished
// before the next one starts.
func NewLeft(s Snapshot) int {
	allowed := NewPerDay - s.NewToday
	patterns := s.PatternsStartedToday
	started := map[string]bool{}
	n := 0
	for _, ns := range s.New {
		if n >= allowed {
			break
		}
		if !ns.PatternStarted && !started[ns.PatternID] {
			if patterns >= NewPatternsPerDay {
				break
			}
			patterns++
			started[ns.PatternID] = true
		}
		n++
	}
	return n
}

// DayStart is when the day containing t began: 4am in Zone.
func DayStart(t time.Time) time.Time {
	lt := t.In(Zone)
	start := time.Date(lt.Year(), lt.Month(), lt.Day(), dayStartHour, 0, 0, 0, Zone)
	if lt.Before(start) {
		start = time.Date(lt.Year(), lt.Month(), lt.Day()-1, dayStartHour, 0, 0, 0, Zone)
	}
	return start
}

// NextDayStart is when the day after the one containing t begins.
func NextDayStart(t time.Time) time.Time {
	d := DayStart(t)
	return time.Date(d.Year(), d.Month(), d.Day()+1, dayStartHour, 0, 0, 0, Zone)
}

var scheduler = func() *fsrs.FSRS {
	p := fsrs.DefaultParam()
	p.EnableFuzz = true // spread out cards learned together
	return fsrs.NewFSRS(p)
}()

// Ratings are the four answers, from worst to best.
var Ratings = []fsrs.Rating{fsrs.Again, fsrs.Hard, fsrs.Good, fsrs.Easy}

// Next is the card after answering it with rating at now.
func Next(c fsrs.Card, now time.Time, rating fsrs.Rating) (fsrs.Card, error) {
	info, err := scheduler.Next(c, now, rating)
	return info.Card, err
}

// Intervals is how long until the card would next be due after each of
// Ratings. Fuzz depends on the exact time, so a long interval may differ
// by a day from what answering a moment later gives.
func Intervals(c fsrs.Card, now time.Time) ([]time.Duration, error) {
	log, err := scheduler.Repeat(c, now)
	if err != nil {
		return nil, err
	}
	d := make([]time.Duration, len(Ratings))
	for i, r := range Ratings {
		d[i] = log[r].Card.Due.Sub(now)
	}
	return d, nil
}

// UnlocksProduce reports whether a Recognise card, just answered, is known
// well enough to start practising the sentence the other way round.
func UnlocksProduce(ex Exercise, after fsrs.Card) bool {
	return ex == Recognise && after.State == fsrs.Review && after.ScheduledDays >= ProduceAfterDays
}
