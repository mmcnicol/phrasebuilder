package web

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/open-spaced-repetition/go-fsrs/v4"

	"github.com/mmcnicol/phrasebuilder/internal/srs"
	"github.com/mmcnicol/phrasebuilder/internal/store"
)

// ReviewStore keeps learners' cards and reports. *store.Store implements it.
type ReviewStore interface {
	Snapshot(ctx context.Context, userID int64, now time.Time) (srs.Snapshot, error)
	Item(ctx context.Context, userID int64, sentenceID string, ex srs.Exercise, now time.Time) (store.ReviewItem, error)
	Answer(ctx context.Context, a store.Answer) (bool, error)
	Undo(ctx context.Context, userID int64, now time.Time) (store.Undone, bool, error)
	Today(ctx context.Context, userID int64, now time.Time) (store.Today, error)
	AddReport(ctx context.Context, userID int64, sentenceID, note string) error
	OpenReports(ctx context.Context) ([]store.Report, error)
	ResolveReport(ctx context.Context, id int64) error
}

// cardView is what the review page's card area shows.
type cardView struct {
	Due, NewLeft int
	Item         *store.ReviewItem // nil when done
	Ahead        bool              // a learning card shown before it is due
	Grades       []grade
	Today        store.Today // when done
	Now          time.Time
	CanUndo      bool
}

type grade struct {
	Rating   int
	Label    string
	Interval time.Duration
}

var gradeLabels = []string{"Again", "Hard", "Good", "Easy"}

// next builds the view of the user's next item.
func (s *server) next(ctx context.Context, user int64, now time.Time) (cardView, error) {
	snap, err := s.cfg.Reviews.Snapshot(ctx, user, now)
	if err != nil {
		return cardView{}, err
	}
	v := cardView{Due: srs.DueNow(snap), NewLeft: srs.NewLeft(snap), Now: now}
	switch ch := srs.Choose(snap); ch.Kind {
	case srs.Done:
		v.Today, err = s.cfg.Reviews.Today(ctx, user, now)
		return v, err
	case srs.Learn:
		return v, s.withItem(ctx, &v, user, ch.New.SentenceID, srs.Recognise, now)
	default:
		v.Ahead = ch.Kind == srs.Ahead
		return v, s.withItem(ctx, &v, user, ch.Card.SentenceID, ch.Card.Exercise, now)
	}
}

func (s *server) withItem(ctx context.Context, v *cardView, user int64, sentenceID string, ex srs.Exercise, now time.Time) error {
	it, err := s.cfg.Reviews.Item(ctx, user, sentenceID, ex, now)
	if err != nil {
		return err
	}
	intervals, err := srs.Intervals(it.Card.Card, now)
	if err != nil {
		return err
	}
	v.Item = &it
	v.Grades = make([]grade, len(srs.Ratings))
	for i, r := range srs.Ratings {
		v.Grades[i] = grade{Rating: int(r), Label: gradeLabels[i], Interval: intervals[i]}
	}
	return nil
}

func (s *server) review(w http.ResponseWriter, r *http.Request) {
	v, err := s.next(r.Context(), userFrom(r.Context()), s.now())
	if err != nil {
		s.fail(w, "next card", err)
		return
	}
	s.render(w, http.StatusOK, "review", v)
}

func (s *server) answer(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	cardID, err1 := strconv.ParseInt(r.PostFormValue("card_id"), 10, 64)
	reps, err2 := strconv.Atoi(r.PostFormValue("reps"))
	rating, err3 := strconv.Atoi(r.PostFormValue("rating"))
	elapsed, _ := strconv.Atoi(r.PostFormValue("elapsed_ms")) // optional
	sentenceID := r.PostFormValue("sentence_id")
	if err := errors.Join(err1, err2, err3); err != nil || rating < int(fsrs.Again) || rating > int(fsrs.Easy) || (cardID == 0 && sentenceID == "") {
		s.error(w, http.StatusBadRequest, "That answer could not be read.")
		return
	}
	now := s.now()
	_, err := s.cfg.Reviews.Answer(r.Context(), store.Answer{
		UserID: user, CardID: cardID, SentenceID: sentenceID, Reps: reps,
		Rating: fsrs.Rating(rating), ElapsedMS: elapsed, Now: now,
	})
	if err != nil {
		s.fail(w, "answer", err)
		return
	}
	s.showNext(w, r, user, now, true)
}

func (s *server) undo(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	now := s.now()
	u, ok, err := s.cfg.Reviews.Undo(r.Context(), user, now)
	if err != nil {
		s.fail(w, "undo", err)
		return
	}
	if !ok {
		s.showNext(w, r, user, now, false)
		return
	}
	// Show the card that was undone, whatever is due.
	snap, err := s.cfg.Reviews.Snapshot(r.Context(), user, now)
	if err == nil {
		v := cardView{Due: srs.DueNow(snap), NewLeft: srs.NewLeft(snap), Now: now, CanUndo: true}
		if err = s.withItem(r.Context(), &v, user, u.SentenceID, u.Exercise, now); err == nil {
			s.card(w, r, v)
			return
		}
	}
	s.fail(w, "undone card", err)
}

func (s *server) showNext(w http.ResponseWriter, r *http.Request, user int64, now time.Time, canUndo bool) {
	v, err := s.next(r.Context(), user, now)
	if err != nil {
		s.fail(w, "next card", err)
		return
	}
	v.CanUndo = canUndo
	s.card(w, r, v)
}

// card sends htmx the card area; without htmx, it reloads the page.
func (s *server) card(w http.ResponseWriter, r *http.Request, v cardView) {
	if !isHTMX(r) {
		http.Redirect(w, r, "/review", http.StatusSeeOther)
		return
	}
	s.fragment(w, "review", "card", v)
}

const maxReportLen = 2000

func (s *server) addReport(w http.ResponseWriter, r *http.Request) {
	note := strings.TrimSpace(r.PostFormValue("note"))
	sentenceID := r.PostFormValue("sentence_id")
	if note == "" || sentenceID == "" || utf8.RuneCountInString(note) > maxReportLen {
		s.error(w, http.StatusBadRequest, fmt.Sprintf("Describe the problem in up to %d characters.", maxReportLen))
		return
	}
	if err := s.cfg.Reviews.AddReport(r.Context(), userFrom(r.Context()), sentenceID, note); err != nil {
		s.fail(w, "add report", err)
		return
	}
	if !isHTMX(r) {
		http.Redirect(w, r, "/review", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<p class="reported">Thanks, reported. See <a href="/reports">reports</a>.</p>`)
}

func (s *server) reports(w http.ResponseWriter, r *http.Request) {
	reports, err := s.cfg.Reviews.OpenReports(r.Context())
	if err != nil {
		s.fail(w, "list reports", err)
		return
	}
	s.render(w, http.StatusOK, "reports", map[string]any{"Reports": reports})
}

func (s *server) resolveReport(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil {
		err = s.cfg.Reviews.ResolveReport(r.Context(), id)
	}
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.fail(w, "resolve report", err)
		return
	}
	http.Redirect(w, r, "/reports", http.StatusSeeOther)
}

// fail logs an unexpected error and shows a page that does not reveal it.
func (s *server) fail(w http.ResponseWriter, what string, err error) {
	slog.Error(what, "err", err)
	s.error(w, http.StatusInternalServerError, "Something went wrong.")
}

// short formats an interval briefly: 1m, 10m, 3h, 4d, 2mo, 1y.
func short(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", max(1, int(math.Round(d.Minutes()))))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(math.Round(d.Hours())))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(math.Round(d.Hours()/24)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(math.Round(d.Hours()/24/30)))
	default:
		return fmt.Sprintf("%dy", int(math.Round(d.Hours()/24/365)))
	}
}
