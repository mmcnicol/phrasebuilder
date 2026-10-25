package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/open-spaced-repetition/go-fsrs/v4"

	"github.com/mmcnicol/phrasebuilder/internal/srs"
	"github.com/mmcnicol/phrasebuilder/internal/store/db"
)

// Snapshot loads what srs.Choose needs to pick a user's next item.
func (s *Store) Snapshot(ctx context.Context, userID int64, now time.Time) (srs.Snapshot, error) {
	snap := srs.Snapshot{Now: now}
	due, err := s.q.ListDueCards(ctx, db.ListDueCardsParams{UserID: userID, Until: ts(now.Add(srs.LearnAhead))})
	if err != nil {
		return snap, fmt.Errorf("due cards: %w", err)
	}
	for _, c := range due {
		snap.Due = append(snap.Due, fromRow(c))
	}
	n, err := s.q.CountNewToday(ctx, db.CountNewTodayParams{UserID: userID, Since: ts(srs.DayStart(now))})
	if err != nil {
		return snap, fmt.Errorf("new today: %w", err)
	}
	snap.NewToday, snap.PatternsStartedToday = int(n.Sentences), int(n.Patterns)
	news, err := s.q.ListNewSentences(ctx, db.ListNewSentencesParams{UserID: userID, Max: srs.NewPerDay})
	if err != nil {
		return snap, fmt.Errorf("new sentences: %w", err)
	}
	for _, r := range news {
		snap.New = append(snap.New, srs.NewSentence{SentenceID: r.ID, PatternID: r.PatternID, PatternStarted: r.PatternStarted})
	}
	return snap, nil
}

// ReviewItem is a card with the sentence and pattern it shows.
type ReviewItem struct {
	srs.Card                            // ID is 0 for a sentence not seen yet
	Text, En, AudioPath, Language       string
	UnitID, PatternID, Frame, PatternEn string
	Note, Status                        string
	NewPattern                          bool // the user has seen no sentence of this pattern
}

// Item loads the user's card for a sentence and exercise, or a new card at
// now if there is none, with what it shows. It returns ErrNotFound for an
// unknown sentence.
func (s *Store) Item(ctx context.Context, userID int64, sentenceID string, ex srs.Exercise, now time.Time) (ReviewItem, error) {
	r, err := s.q.GetReviewSentence(ctx, sentenceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReviewItem{}, ErrNotFound
	}
	if err != nil {
		return ReviewItem{}, err
	}
	it := ReviewItem{
		Card: srs.Card{SentenceID: sentenceID, Exercise: ex, Card: fsrs.NewCard(now)},
		Text: r.Text, En: r.En, AudioPath: r.AudioPath, Language: r.Language,
		UnitID: r.UnitID, PatternID: r.PatternID, Frame: r.Frame, PatternEn: r.PatternEn,
		Note: r.Note, Status: r.Status,
	}
	c, err := s.q.GetCardBySentence(ctx, db.GetCardBySentenceParams{UserID: userID, SentenceID: sentenceID, Exercise: string(ex)})
	switch {
	case err == nil:
		it.Card = fromRow(c)
	case errors.Is(err, pgx.ErrNoRows):
		started, err := s.q.PatternStarted(ctx, db.PatternStartedParams{UserID: userID, PatternID: r.PatternID})
		if err != nil {
			return ReviewItem{}, err
		}
		it.NewPattern = !started
	default:
		return ReviewItem{}, err
	}
	return it, nil
}

// Answer is a user's grade for a card.
type Answer struct {
	UserID int64
	// CardID is the card answered, or 0 for a new sentence's first
	// (Recognise) answer, which creates the card.
	CardID     int64
	SentenceID string // for a new sentence
	// Reps is the card's review count when it was shown. If the card has
	// moved on since, the answer is a repeat and is ignored.
	Reps      int
	Rating    fsrs.Rating
	ElapsedMS int // from showing the card to the answer
	Now       time.Time
}

// Answer schedules a card and logs the review. It reports false, and
// changes nothing, if the answer was already recorded (a double tap or a
// retried request).
func (s *Store) Answer(ctx context.Context, a Answer) (bool, error) {
	a.Now = a.Now.Truncate(time.Microsecond) // Postgres's precision
	applied := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		var before srs.Card
		if a.CardID == 0 {
			before = srs.Card{SentenceID: a.SentenceID, Exercise: srs.Recognise, Card: fsrs.NewCard(a.Now)}
		} else {
			c, err := q.LockCard(ctx, db.LockCardParams{ID: a.CardID, UserID: a.UserID})
			if errors.Is(err, pgx.ErrNoRows) {
				return nil // undone in the meantime
			}
			if err != nil {
				return err
			}
			before = fromRow(c)
			if int(before.Reps) != a.Reps {
				return nil
			}
		}
		after, err := srs.Next(before.Card, a.Now, a.Rating)
		if err != nil {
			return err
		}

		cardID := a.CardID
		if cardID == 0 {
			cardID, err = q.InsertCard(ctx, insertParams(a.UserID, a.SentenceID, srs.Recognise, after, a.Now))
			if errors.Is(err, pgx.ErrNoRows) {
				return nil // already answered
			}
			if err != nil {
				return err
			}
		} else if err := q.UpdateCard(ctx, updateParams(cardID, after)); err != nil {
			return err
		}

		var unlocked pgtype.Int8
		if srs.UnlocksProduce(before.Exercise, after) {
			// A Produce card starts tomorrow, not straight after its Recognise card.
			produce := fsrs.NewCard(srs.NextDayStart(a.Now))
			id, err := q.InsertCard(ctx, insertParams(a.UserID, before.SentenceID, srs.Produce, produce, a.Now))
			switch {
			case err == nil:
				unlocked = pgtype.Int8{Int64: id, Valid: true}
			case !errors.Is(err, pgx.ErrNoRows): // no row: it already exists
				return err
			}
		}

		b := before.Card
		if err := q.InsertReview(ctx, db.InsertReviewParams{
			UserID: a.UserID, CardID: cardID, Rating: int16(a.Rating), ReviewedAt: ts(a.Now),
			ElapsedMs: int32(min(max(a.ElapsedMS, 0), 24*60*60*1000)), CreatedCard: a.CardID == 0, UnlockedCardID: unlocked,
			BeforeDue: ts(b.Due), BeforeStability: b.Stability, BeforeDifficulty: b.Difficulty,
			BeforeScheduledDays: int32(b.ScheduledDays), BeforeReps: int32(b.Reps), BeforeLapses: int32(b.Lapses),
			BeforeState: int16(b.State), BeforeLastReview: ts(b.LastReview), BeforeRemainingSteps: int32(b.RemainingSteps),
		}); err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied, err
}

// Undone is the card whose answer was undone, to show again.
type Undone struct {
	SentenceID string
	Exercise   srs.Exercise
}

// Undo reverts the user's latest answer today: the card goes back to how it
// was, or is deleted if the answer created it. It reports false if there
// is nothing to undo.
func (s *Store) Undo(ctx context.Context, userID int64, now time.Time) (Undone, bool, error) {
	var u Undone
	ok := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		r, err := q.LockLastReview(ctx, db.LockLastReviewParams{UserID: userID, Since: ts(srs.DayStart(now))})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := q.DeleteReview(ctx, r.ID); err != nil {
			return err
		}
		if r.UnlockedCardID.Valid {
			if err := q.DeleteUnreviewedCard(ctx, r.UnlockedCardID.Int64); err != nil {
				return err
			}
		}
		if r.CreatedCard {
			err = q.DeleteCard(ctx, r.CardID)
		} else {
			err = q.UpdateCard(ctx, db.UpdateCardParams{
				ID: r.CardID, Due: r.BeforeDue, Stability: r.BeforeStability, Difficulty: r.BeforeDifficulty,
				ScheduledDays: r.BeforeScheduledDays, Reps: r.BeforeReps, Lapses: r.BeforeLapses,
				State: r.BeforeState, LastReview: r.BeforeLastReview, RemainingSteps: r.BeforeRemainingSteps,
			})
		}
		if err != nil {
			return err
		}
		u, ok = Undone{SentenceID: r.SentenceID, Exercise: srs.Exercise(r.Exercise)}, true
		return nil
	})
	return u, ok, err
}

// Today summarises a user's reviews since the day started.
type Today struct {
	Reviews, NewSentences int
	Elapsed               time.Duration
	// NextDue is when the user's next card is due; zero if they have none.
	NextDue time.Time
}

// Today summarises the user's day so far.
func (s *Store) Today(ctx context.Context, userID int64, now time.Time) (Today, error) {
	r, err := s.q.SummariseReviews(ctx, db.SummariseReviewsParams{UserID: userID, Since: ts(srs.DayStart(now))})
	if err != nil {
		return Today{}, err
	}
	next, err := s.q.NextDue(ctx, userID)
	if err != nil {
		return Today{}, err
	}
	return Today{
		Reviews: int(r.Reviews), NewSentences: int(r.NewSentences),
		Elapsed: time.Duration(r.ElapsedMs) * time.Millisecond, NextDue: next.Time,
	}, nil
}

// Report is a problem reported with a sentence.
type Report struct {
	ID                         int64
	Note                       string
	Created                    time.Time
	SentenceID, Text, En, Unit string
}

// AddReport records a problem with a sentence.
func (s *Store) AddReport(ctx context.Context, userID int64, sentenceID, note string) error {
	return s.q.InsertReport(ctx, db.InsertReportParams{UserID: userID, SentenceID: sentenceID, Note: note})
}

// OpenReports lists unresolved reports, oldest first.
func (s *Store) OpenReports(ctx context.Context) ([]Report, error) {
	rows, err := s.q.ListOpenReports(ctx)
	if err != nil {
		return nil, err
	}
	reports := make([]Report, len(rows))
	for i, r := range rows {
		reports[i] = Report{ID: r.ID, Note: r.Note, Created: r.CreatedAt.Time, SentenceID: r.SentenceID, Text: r.Text, En: r.En, Unit: r.UnitID}
	}
	return reports, nil
}

// ResolveReport marks a report resolved, or returns ErrNotFound.
func (s *Store) ResolveReport(ctx context.Context, id int64) error {
	n, err := s.q.ResolveReport(ctx, id)
	if err == nil && n == 0 {
		err = ErrNotFound
	}
	return err
}

func fromRow(c db.Card) srs.Card {
	return srs.Card{
		ID: c.ID, SentenceID: c.SentenceID, Exercise: srs.Exercise(c.Exercise),
		Card: fsrs.Card{
			Due: c.Due.Time, Stability: c.Stability, Difficulty: c.Difficulty,
			ScheduledDays: uint64(c.ScheduledDays), Reps: uint64(c.Reps), Lapses: uint64(c.Lapses),
			State: fsrs.State(c.State), LastReview: c.LastReview.Time, RemainingSteps: int(c.RemainingSteps),
		},
	}
}

func insertParams(userID int64, sentenceID string, ex srs.Exercise, c fsrs.Card, now time.Time) db.InsertCardParams {
	return db.InsertCardParams{
		UserID: userID, SentenceID: sentenceID, Exercise: string(ex),
		Due: ts(c.Due), Stability: c.Stability, Difficulty: c.Difficulty,
		ScheduledDays: int32(c.ScheduledDays), Reps: int32(c.Reps), Lapses: int32(c.Lapses),
		State: int16(c.State), LastReview: ts(c.LastReview), RemainingSteps: int32(c.RemainingSteps),
		CreatedAt: ts(now),
	}
}

func updateParams(id int64, c fsrs.Card) db.UpdateCardParams {
	return db.UpdateCardParams{
		ID: id, Due: ts(c.Due), Stability: c.Stability, Difficulty: c.Difficulty,
		ScheduledDays: int32(c.ScheduledDays), Reps: int32(c.Reps), Lapses: int32(c.Lapses),
		State: int16(c.State), LastReview: ts(c.LastReview), RemainingSteps: int32(c.RemainingSteps),
	}
}

// ts converts a time to a timestamptz; the zero time is NULL.
func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: !t.IsZero()}
}
