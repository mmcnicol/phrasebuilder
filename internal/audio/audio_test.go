package audio

import (
	"context"
	"regexp"
	"testing"
	"testing/fstest"

	"github.com/mmcnicol/phrasebuilder/internal/content"
)

func TestPath(t *testing.T) {
	p := Path("fr", "Un café, s'il vous plaît.")
	if !regexp.MustCompile(`^fr/[0-9a-f]{16}\.mp3$`).MatchString(p) {
		t.Errorf("Path = %q", p)
	}
	if Path("fr", "Un café, s'il vous plaît.") != p {
		t.Error("Path is not stable")
	}
	if Path("fr", "Un thé, s'il vous plaît.") == p {
		t.Error("different text, same path")
	}

	old := Voices["fr"]
	t.Cleanup(func() { Voices["fr"] = old })
	Voices["fr"] = Voice{LanguageCode: "fr-FR", Name: "another-voice"}
	if Path("fr", "Un café, s'il vous plaît.") == p {
		t.Error("different voice, same path")
	}
}

type fakeSynth struct{ calls []string }

func (f *fakeSynth) Synthesize(_ context.Context, v Voice, text string) ([]byte, error) {
	f.calls = append(f.calls, text)
	return []byte("mp3:" + v.Name + ":" + text), nil
}

type fakeBucket map[string][]byte

func (b fakeBucket) Exists(_ context.Context, path string) (bool, error) {
	_, ok := b[path]
	return ok, nil
}

func (b fakeBucket) Put(_ context.Context, path string, mp3 []byte) error {
	b[path] = mp3
	return nil
}

func course(t *testing.T) *content.Course {
	t.Helper()
	units, problems := content.Load(fstest.MapFS{"fr/unit-01-cafe.yaml": {Data: []byte(`unit: cafe
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
        - { fr: "un café", en: "a coffee" }
        - { fr: "un thé", en: "a tea" }
  - id: un-cafe
    status: draft
    frame: "Un café ?"
    en: "A coffee?"
    note: "Rising tone for a question."
`)}})
	c, more := content.Build(units)
	if problems = append(problems, more...); len(problems) > 0 {
		t.Fatal(problems)
	}
	return c
}

func TestGenerate(t *testing.T) {
	c := course(t)
	clips := Clips(c)
	// 3 sentences + 2 words.
	if len(clips) != 5 {
		t.Fatalf("got %d clips, want 5: %+v", len(clips), clips)
	}

	bucket := fakeBucket{}
	bucket[Path("fr", "un thé")] = []byte("already there")
	synth := &fakeSynth{}
	st, err := Generate(context.Background(), clips, synth, bucket, nil)
	if err != nil {
		t.Fatal(err)
	}
	if st.Existing != 1 || st.Created != 4 || len(synth.calls) != 4 {
		t.Errorf("stats = %+v, calls = %v", st, synth.calls)
	}
	if got := string(bucket[Path("fr", "Un café ?")]); got != "mp3:"+Voices["fr"].Name+":Un café ?" {
		t.Errorf("stored %q", got)
	}

	// A second run creates nothing.
	synth.calls = nil
	if st, err = Generate(context.Background(), clips, synth, bucket, nil); err != nil || st.Created != 0 || len(synth.calls) != 0 {
		t.Errorf("second run: %+v, %v, calls %v", st, err, synth.calls)
	}
}

func TestMissingVoices(t *testing.T) {
	c := course(t)
	if m := MissingVoices(c); len(m) != 0 {
		t.Errorf("MissingVoices = %v", m)
	}
	c.Units[0].Language = "xx"
	if m := MissingVoices(c); len(m) != 1 || m[0] != "xx" {
		t.Errorf("MissingVoices = %v", m)
	}
}
