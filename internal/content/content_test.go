package content

import (
	"strings"
	"testing"
	"testing/fstest"
)

const cafe = `unit: cafe
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
    status: checked
    frame: "Où sont les toilettes ?"
    en: "Where are the toilets?"
    note: "Où = where."
`

func build(t *testing.T, files map[string]string) (*Course, Problems) {
	t.Helper()
	fsys := fstest.MapFS{}
	for name, data := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(data)}
	}
	units, problems := Load(fsys)
	c, more := Build(units)
	return c, append(problems, more...)
}

func TestBuildValid(t *testing.T) {
	c, problems := build(t, map[string]string{"fr/unit-01-cafe.yaml": cafe})
	if len(problems) > 0 {
		t.Fatalf("unexpected problems:\n%v", problems)
	}
	if len(c.Units) != 1 || c.Units[0].ID != "fr/cafe" || c.Units[0].Title != "At the café" {
		t.Errorf("units = %+v", c.Units)
	}
	if len(c.Patterns) != 2 || c.Patterns[1].ID != "fr/ou-sont-les-toilettes" || c.Patterns[1].Position != 2 {
		t.Errorf("patterns = %+v", c.Patterns)
	}
	wantWords := []Word{
		{ID: "fr/un-cafe", Language: "fr", Text: "un café", En: "a coffee", Gender: "m"},
		{ID: "fr/l-addition", Language: "fr", Text: "l'addition", En: "the bill", Gender: "f"},
	}
	if len(c.Words) != len(wantWords) {
		t.Fatalf("words = %+v", c.Words)
	}
	for i, w := range wantWords {
		if c.Words[i] != w {
			t.Errorf("word %d = %+v, want %+v", i, c.Words[i], w)
		}
	}
	wantSentences := []struct{ id, text, en string }{
		{"fr/je-voudrais-noun/un-cafe", "Je voudrais un café, s'il vous plaît.", "I would like a coffee, please."},
		{"fr/je-voudrais-noun/l-addition", "Je voudrais l'addition, s'il vous plaît.", "I would like the bill, please."},
		{"fr/ou-sont-les-toilettes", "Où sont les toilettes ?", "Where are the toilets?"},
	}
	if len(c.Sentences) != len(wantSentences) {
		t.Fatalf("sentences = %+v", c.Sentences)
	}
	for i, w := range wantSentences {
		s := c.Sentences[i]
		if s.ID != w.id || s.Text != w.text || s.En != w.en {
			t.Errorf("sentence %d = %q %q %q, want %q %q %q", i, s.ID, s.Text, s.En, w.id, w.text, w.en)
		}
	}
	if sw := c.Sentences[1].Words; len(sw) != 1 || sw[0] != (SentenceWord{WordID: "fr/l-addition", Slot: "item"}) {
		t.Errorf("sentence words = %+v", sw)
	}
}

func TestCartesianProductAndCapitalisation(t *testing.T) {
	c, problems := build(t, map[string]string{"fr/unit-01-gare.yaml": `unit: gare
order: 1
title: At the station
patterns:
  - id: n-billets-pour
    status: draft
    frame: "{n} pour {ville}, s'il vous plaît."
    en: "{n} to {ville}, please."
    note: "pour = to (a destination)."
    slots:
      n:
        - { fr: "un billet", en: "one ticket" }
        - { fr: "deux billets", en: "two tickets" }
      ville:
        - { fr: "Paris", en: "Paris" }
        - { fr: "Lyon", en: "Lyon" }
        - { fr: "Nice", en: "Nice" }
`})
	if len(problems) > 0 {
		t.Fatalf("unexpected problems:\n%v", problems)
	}
	if len(c.Sentences) != 6 {
		t.Fatalf("got %d sentences, want 6", len(c.Sentences))
	}
	first, last := c.Sentences[0], c.Sentences[5]
	if first.ID != "fr/n-billets-pour/un-billet/paris" || first.Text != "Un billet pour Paris, s'il vous plaît." || first.En != "One ticket to Paris, please." {
		t.Errorf("first = %+v", first)
	}
	if last.ID != "fr/n-billets-pour/deux-billets/nice" || last.Position != 6 {
		t.Errorf("last = %+v", last)
	}
	if len(first.Words) != 2 || first.Words[1].Slot != "ville" {
		t.Errorf("first words = %+v", first.Words)
	}
}

func TestWordsSharedAcrossPatterns(t *testing.T) {
	two := cafe + `  - id: avez-vous
    status: draft
    frame: "Avez-vous {item} ?"
    en: "Do you have {item}?"
    note: "Avez-vous = do you have."
    slots:
      item:
        - { fr: "un café", en: "a coffee", g: m }
`
	c, problems := build(t, map[string]string{"fr/unit-01-cafe.yaml": two})
	if len(problems) > 0 {
		t.Fatalf("unexpected problems:\n%v", problems)
	}
	if len(c.Words) != 2 {
		t.Errorf("got %d words, want 2 (un café shared)", len(c.Words))
	}
}

// Each case changes the valid unit and expects one problem containing want.
func TestProblems(t *testing.T) {
	tests := []struct {
		name  string
		file  string
		edit  func(string) string
		extra map[string]string
		want  string
	}{
		{name: "unknown key", edit: r("    note: \"Où", "    nots: \"Où"), want: "field nots not found"},
		{name: "empty file", edit: func(string) string { return "" }, want: "file is empty"},
		{name: "bad unit id", edit: r("unit: cafe", "unit: Café"), want: `unit id "Café" must be a lowercase slug`},
		{name: "file name", file: "fr/unit-02-cafe.yaml", want: "file name should be unit-01-cafe.yaml"},
		{name: "unknown language", file: "xx/unit-01-cafe.yaml", want: `unknown language directory "xx"`},
		{name: "missing title", edit: r("title: At the café\n", ""), want: "title is missing"},
		{name: "bad status", edit: r("status: checked", "status: done"), want: `status "done" must be draft or checked`},
		{name: "missing status", edit: r("    status: checked\n", ""), want: "status is missing"},
		{name: "missing note", edit: r("    note: \"Où = where.\"\n", ""), want: "pattern ou-sont-les-toilettes: note is missing"},
		{name: "long note", edit: r("Où = where.", strings.Repeat("x", 301)), want: "note is 301 characters"},
		{name: "no final punctuation", edit: r(`"Where are the toilets?"`, `"Where are the toilets"`), want: "en should end with"},
		{name: "slot not in en", edit: r("I would like {item}, please.", "I would like it, please."), want: "slot {item} is in frame but not in en"},
		{name: "slot only in en", edit: r(`"Where are the toilets?"`, `"Where are the {x}?"`), want: "slot {x} in en is not in frame"},
		{name: "undefined slot", edit: func(s string) string {
			return r("like {item},", "like {thing} {item},")(r("Je voudrais {item},", "Je voudrais {thing} {item},")(s))
		}, want: "slot {thing} in frame has no fillers"},
		{name: "unused slot", edit: r(`        - { fr: "l'addition", en: "the bill", g: f }`+"\n", `        - { fr: "l'addition", en: "the bill", g: f }`+"\n      spare:\n        - { fr: \"x\", en: \"x\" }\n"), want: `slot "spare" is not used in frame`},
		{name: "duplicate filler", edit: r(`"l'addition", en: "the bill", g: f`, `"un café", en: "a coffee", g: m`), want: `filler "un café" appears twice`},
		{name: "bad gender", edit: r("g: f }", "g: x }"), want: "g must be m, f or blank"},
		{name: "missing filler en", edit: r(`en: "the bill", `, ""), want: "filler needs both fr and en"},
		{name: "duplicate pattern id", edit: r("id: ou-sont-les-toilettes", "id: je-voudrais-noun"), want: `pattern id "je-voudrais-noun" is also used at fr/unit-01-cafe.yaml:5`},
		{name: "same slug different text", edit: r(`"l'addition", en: "the bill"`, `"un cafe", en: "the bill"`), want: "have the same id fr/un-cafe"},
		{name: "duplicate order", extra: map[string]string{"fr/unit-01-hotel.yaml": strings.ReplaceAll(strings.ReplaceAll(cafe, "unit: cafe", "unit: hotel"), "- id: ", "- id: h-")}, want: "order 1 is also used by fr/unit-01-cafe.yaml"},
		{name: "inconsistent word", extra: map[string]string{"fr/unit-02-bar.yaml": strings.NewReplacer("unit: cafe", "unit: bar", "order: 1", "order: 2", "- id: ", "- id: b-", `"a coffee"`, `"a black coffee"`).Replace(cafe)}, want: `filler "un café" is defined differently at fr/unit-01-cafe.yaml:12`},
		{name: "too many sentences", edit: r(`        - { fr: "l'addition", en: "the bill", g: f }`+"\n", manyFillers(20)), want: "makes 21 sentences; the limit is 20"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := tt.file
			if file == "" {
				file = "fr/unit-01-cafe.yaml"
			}
			data := cafe
			if tt.edit != nil {
				data = tt.edit(cafe)
			}
			files := map[string]string{file: data}
			for k, v := range tt.extra {
				files[k] = v
			}
			_, problems := build(t, files)
			if len(problems) != 1 || !strings.Contains(problems[0].String(), tt.want) {
				t.Errorf("want one problem containing %q, got:\n%v", tt.want, problems)
			}
		})
	}
}

func TestProblemLines(t *testing.T) {
	_, problems := build(t, map[string]string{"fr/unit-01-cafe.yaml": strings.Replace(cafe, "g: f }", "g: x }", 1)})
	if len(problems) != 1 || problems[0].Line != 13 || !strings.HasPrefix(problems[0].String(), "fr/unit-01-cafe.yaml:13: ") {
		t.Errorf("got %v, want a problem at fr/unit-01-cafe.yaml:13", problems)
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"un café":          "un-cafe",
		"L'addition":       "l-addition",
		"  Où sont-ils ? ": "ou-sont-ils",
		"Straße":           "strasse",
		"un œuf":           "un-oeuf",
		"2 billets":        "2-billets",
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func r(old, new string) func(string) string {
	return func(s string) string {
		if !strings.Contains(s, old) {
			panic("test edit does not apply: " + old)
		}
		return strings.Replace(s, old, new, 1)
	}
}

func manyFillers(n int) string {
	var sb strings.Builder
	for i := range n {
		sb.WriteString("        - { fr: \"chose " + string(rune('a'+i)) + "\", en: \"thing\" }\n")
	}
	return sb.String()
}
