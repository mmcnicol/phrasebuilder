// Package audio generates an MP3 for every sentence and word with a
// text-to-speech service and stores it in a bucket.
//
// Files are named by a hash of the voice and text, so they never change:
// regenerating only fills gaps, and changed text or a new voice gets a new
// file.
package audio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/mmcnicol/phrasebuilder/internal/content"
)

// Voice is a text-to-speech voice.
type Voice struct {
	LanguageCode string // BCP-47, e.g. fr-FR
	Name         string // e.g. fr-FR-Neural2-F
}

// Voices is the one fixed voice per course language.
var Voices = map[string]Voice{
	"fr": {LanguageCode: "fr-FR", Name: "fr-FR-Chirp3-HD-Kore"}, // chosen from samples, 2026-10-25
}

// Path is where the audio for text in a course language is stored, e.g.
// fr/1a2b3c4d5e6f7a8b.mp3. It panics if the language has no voice; check
// with MissingVoices first.
func Path(language, text string) string {
	v, ok := Voices[language]
	if !ok {
		panic("audio: no voice for language " + language)
	}
	sum := sha256.Sum256([]byte(v.Name + "\x00" + text))
	return language + "/" + hex.EncodeToString(sum[:8]) + ".mp3"
}

// MissingVoices lists course languages that have no voice.
func MissingVoices(c *content.Course) []string {
	var missing []string
	seen := map[string]bool{}
	for _, u := range c.Units {
		if _, ok := Voices[u.Language]; !ok && !seen[u.Language] {
			missing = append(missing, u.Language)
			seen[u.Language] = true
		}
	}
	return missing
}

// Synthesizer turns text into MP3 audio.
type Synthesizer interface {
	Synthesize(ctx context.Context, v Voice, text string) ([]byte, error)
}

// Bucket stores audio files.
type Bucket interface {
	Exists(ctx context.Context, path string) (bool, error)
	Put(ctx context.Context, path string, mp3 []byte) error
}

// Clip is one text to speak and where its audio goes.
type Clip struct {
	Language, Text, Path string
}

// Clips lists every sentence and word in c once, sorted by path.
func Clips(c *content.Course) []Clip {
	unitLang := map[string]string{}
	for _, u := range c.Units {
		unitLang[u.ID] = u.Language
	}
	patternLang := map[string]string{}
	for _, p := range c.Patterns {
		patternLang[p.ID] = unitLang[p.UnitID]
	}
	byPath := map[string]Clip{}
	add := func(lang, text string) {
		p := Path(lang, text)
		byPath[p] = Clip{Language: lang, Text: text, Path: p}
	}
	for _, s := range c.Sentences {
		add(patternLang[s.PatternID], s.Text)
	}
	for _, w := range c.Words {
		add(w.Language, w.Text)
	}
	clips := make([]Clip, 0, len(byPath))
	for _, cl := range byPath {
		clips = append(clips, cl)
	}
	sort.Slice(clips, func(i, j int) bool { return clips[i].Path < clips[j].Path })
	return clips
}

// Stats counts what Generate did.
type Stats struct {
	Existing, Created int
	Characters        int // sent to the text-to-speech service
}

// Generate makes the audio for every clip not already in the bucket.
// progress, if not nil, is called after each new file.
func Generate(ctx context.Context, clips []Clip, synth Synthesizer, bucket Bucket, progress func(Clip)) (Stats, error) {
	var st Stats
	for _, cl := range clips {
		ok, err := bucket.Exists(ctx, cl.Path)
		if err != nil {
			return st, fmt.Errorf("check %s: %w", cl.Path, err)
		}
		if ok {
			st.Existing++
			continue
		}
		mp3, err := synth.Synthesize(ctx, Voices[cl.Language], cl.Text)
		if err != nil {
			return st, fmt.Errorf("synthesize %q: %w", cl.Text, err)
		}
		st.Characters += len([]rune(cl.Text))
		if err := bucket.Put(ctx, cl.Path, mp3); err != nil {
			return st, fmt.Errorf("store %s: %w", cl.Path, err)
		}
		st.Created++
		if progress != nil {
			progress(cl)
		}
	}
	return st, nil
}
