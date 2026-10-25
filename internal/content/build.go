package content

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Limits on content, to catch mistakes.
const (
	MaxSentencesPerPattern = 20
	MaxNoteRunes           = 300
)

// Course is checked, expanded content, ready to load into the database.
// IDs are prefixed with the language, e.g. "fr/cafe".
type Course struct {
	Units     []UnitRow
	Patterns  []PatternRow
	Words     []Word
	Sentences []Sentence
}

// UnitRow is a unit without its patterns.
type UnitRow struct {
	ID       string
	Language string
	Position int
	Title    string
}

// PatternRow is a pattern without its slots.
type PatternRow struct {
	ID       string
	UnitID   string
	Position int
	Frame    string
	En       string
	Note     string
	Status   string
}

// Word is a slot filler. The same French text is one word across patterns.
type Word struct {
	ID       string // e.g. fr/un-cafe
	Language string
	Text     string
	En       string
	Gender   string
}

// Sentence is a pattern with every slot filled.
type Sentence struct {
	ID        string // e.g. fr/je-voudrais-noun/un-cafe
	PatternID string
	Position  int
	Text      string
	En        string
	Words     []SentenceWord
}

// SentenceWord records which word fills which slot of a sentence.
type SentenceWord struct {
	WordID string
	Slot   string
}

var (
	idRE   = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	slotRE = regexp.MustCompile(`\{([^{}]*)\}`)
)

// Build checks units and expands them into a Course. If there are any
// problems, the Course is incomplete and must not be loaded.
func Build(units []*Unit) (*Course, Problems) {
	b := &builder{
		c:        &Course{},
		words:    map[string]wordAt{},
		units:    map[string]string{},
		orders:   map[string]string{},
		patterns: map[string]string{},
	}
	for _, u := range units {
		b.unit(u)
	}
	return b.c, b.problems
}

type wordAt struct {
	Word
	at string
}

type builder struct {
	c        *Course
	problems Problems
	words    map[string]wordAt // by ID
	units    map[string]string // unit ID -> file
	orders   map[string]string // lang/order -> file
	patterns map[string]string // pattern ID -> file:line
}

func (b *builder) add(file string, line int, format string, args ...any) {
	b.problems = append(b.problems, Problem{File: file, Line: line, Msg: fmt.Sprintf(format, args...)})
}

func (b *builder) unit(u *Unit) {
	f := u.File
	if _, ok := Languages[u.Language]; !ok {
		b.add(f, 0, "unknown language directory %q", u.Language)
		return
	}
	validID := idRE.MatchString(u.ID)
	if !validID {
		b.add(f, 0, "unit id %q must be a lowercase slug like at-the-cafe", u.ID)
	}
	if u.Order < 1 {
		b.add(f, 0, "order must be 1 or more")
	}
	if strings.TrimSpace(u.Title) == "" {
		b.add(f, 0, "title is missing")
	}
	if len(u.Patterns) == 0 {
		b.add(f, 0, "unit has no patterns")
	}
	if want := fmt.Sprintf("unit-%02d-%s.yaml", u.Order, u.ID); validID && fileBase(f) != want {
		b.add(f, 0, "file name should be %s to match the unit id and order", want)
	}

	unitID := u.Language + "/" + u.ID
	if other, ok := b.units[unitID]; ok {
		b.add(f, 0, "unit id %q is also used in %s", u.ID, other)
	}
	b.units[unitID] = f
	orderKey := fmt.Sprintf("%s/%d", u.Language, u.Order)
	if other, ok := b.orders[orderKey]; ok {
		b.add(f, 0, "order %d is also used by %s", u.Order, other)
	}
	b.orders[orderKey] = f

	b.c.Units = append(b.c.Units, UnitRow{ID: unitID, Language: u.Language, Position: u.Order, Title: u.Title})
	for i := range u.Patterns {
		b.pattern(u, unitID, i+1, &u.Patterns[i])
	}
}

func (b *builder) pattern(u *Unit, unitID string, pos int, p *Pattern) {
	f, line := u.File, p.Line
	patternID := u.Language + "/" + p.ID
	if !idRE.MatchString(p.ID) {
		b.add(f, line, "pattern id %q must be a lowercase slug like je-voudrais-noun", p.ID)
	}
	if other, ok := b.patterns[patternID]; ok {
		b.add(f, line, "pattern id %q is also used at %s", p.ID, other)
	}
	b.patterns[patternID] = fmt.Sprintf("%s:%d", f, line)

	switch p.Status {
	case "draft", "checked":
	case "":
		b.add(f, line, "pattern %s: status is missing (draft or checked)", p.ID)
	default:
		b.add(f, line, "pattern %s: status %q must be draft or checked", p.ID, p.Status)
	}
	for _, fv := range [][2]string{{"frame", p.Frame}, {"en", p.En}, {"note", p.Note}} {
		if strings.TrimSpace(fv[1]) == "" {
			b.add(f, line, "pattern %s: %s is missing", p.ID, fv[0])
		}
	}
	if n := utf8.RuneCountInString(p.Note); n > MaxNoteRunes {
		b.add(f, line, "pattern %s: note is %d characters; keep it under %d", p.ID, n, MaxNoteRunes)
	}
	for _, fv := range [][2]string{{"frame", p.Frame}, {"en", p.En}} {
		if fv[1] != "" && !strings.ContainsAny(lastRune(fv[1]), ".?!…") {
			b.add(f, line, "pattern %s: %s should end with . ? ! or …", p.ID, fv[0])
		}
	}

	// Slots, in the order they appear in the frame.
	slots := slotNames(p.Frame)
	seen := map[string]bool{}
	for _, s := range slots {
		if seen[s] {
			b.add(f, line, "pattern %s: slot {%s} appears twice in frame", p.ID, s)
		}
		seen[s] = true
		if !idRE.MatchString(s) {
			b.add(f, line, "pattern %s: slot name {%s} must be a lowercase slug", p.ID, s)
		}
		if _, ok := p.Slots[s]; !ok {
			b.add(f, line, "pattern %s: slot {%s} in frame has no fillers under slots", p.ID, s)
		}
	}
	enSlots := map[string]bool{}
	for _, s := range slotNames(p.En) {
		enSlots[s] = true
		if !seen[s] {
			b.add(f, line, "pattern %s: slot {%s} in en is not in frame", p.ID, s)
		}
	}
	for _, s := range slots {
		if !enSlots[s] {
			b.add(f, line, "pattern %s: slot {%s} is in frame but not in en", p.ID, s)
		}
	}
	for _, s := range sortedKeys(p.Slots) {
		if !seen[s] {
			b.add(f, p.SlotLines[s], "pattern %s: slot %q is not used in frame", p.ID, s)
		}
	}

	// Fillers become words.
	fillerWords := map[string][]string{} // slot -> word IDs, parallel to p.Slots[slot]
	for _, s := range slots {
		dup := map[string]bool{}
		for _, fl := range p.Slots[s] {
			wid := b.word(u, p, fl)
			if dup[fl.Text] {
				b.add(f, fl.Line, "pattern %s: filler %q appears twice in slot {%s}", p.ID, fl.Text, s)
			}
			dup[fl.Text] = true
			fillerWords[s] = append(fillerWords[s], wid)
		}
	}

	total := 1
	for _, s := range slots {
		total *= len(p.Slots[s])
	}
	if total > MaxSentencesPerPattern {
		b.add(f, line, "pattern %s makes %d sentences; the limit is %d", p.ID, total, MaxSentencesPerPattern)
		return
	}

	b.c.Patterns = append(b.c.Patterns, PatternRow{
		ID: patternID, UnitID: unitID, Position: pos,
		Frame: p.Frame, En: p.En, Note: p.Note, Status: p.Status,
	})
	if total == 0 {
		return // a slot with no fillers; already reported
	}

	// Cartesian product of fillers, first slot varying slowest.
	idx := make([]int, len(slots))
	for n := 1; n <= total; n++ {
		s := Sentence{PatternID: patternID, Position: n, Text: p.Frame, En: p.En}
		idParts := []string{patternID}
		for i, slot := range slots {
			fl := p.Slots[slot][idx[i]]
			wid := fillerWords[slot][idx[i]]
			s.Text = strings.Replace(s.Text, "{"+slot+"}", fl.Text, 1)
			s.En = strings.Replace(s.En, "{"+slot+"}", fl.En, 1)
			s.Words = append(s.Words, SentenceWord{WordID: wid, Slot: slot})
			idParts = append(idParts, strings.TrimPrefix(wid, u.Language+"/"))
		}
		s.ID = strings.Join(idParts, "/")
		s.Text = capitalise(s.Text)
		s.En = capitalise(s.En)
		b.c.Sentences = append(b.c.Sentences, s)

		for i := len(idx) - 1; i >= 0; i-- {
			idx[i]++
			if idx[i] < len(p.Slots[slots[i]]) {
				break
			}
			idx[i] = 0
		}
	}
}

// word checks a filler and records it as a word, returning its ID.
func (b *builder) word(u *Unit, p *Pattern, fl Filler) string {
	f := u.File
	w := Word{Language: u.Language, Text: strings.TrimSpace(fl.Text), En: strings.TrimSpace(fl.En), Gender: fl.G}
	if w.Text == "" || w.En == "" {
		b.add(f, fl.Line, "pattern %s: filler needs both %s and en", p.ID, u.Language)
	}
	switch fl.G {
	case "", "m", "f":
	default:
		b.add(f, fl.Line, "pattern %s: filler %q: g must be m, f or blank", p.ID, fl.Text)
	}
	slug := Slug(w.Text)
	if slug == "" {
		slug = "-"
	}
	w.ID = u.Language + "/" + slug

	at := fmt.Sprintf("%s:%d", f, fl.Line)
	if prev, ok := b.words[w.ID]; ok {
		switch {
		case prev.Text != w.Text:
			b.add(f, fl.Line, "filler %q and %q (%s) have the same id %s; make them differ by more than accents or punctuation", w.Text, prev.Text, prev.at, w.ID)
		case prev.En != w.En || prev.Gender != w.Gender:
			b.add(f, fl.Line, "filler %q is defined differently at %s (en %q, g %q); use the same en and g everywhere", w.Text, prev.at, prev.En, prev.Gender)
		}
		return w.ID
	}
	b.words[w.ID] = wordAt{Word: w, at: at}
	b.c.Words = append(b.c.Words, w)
	return w.ID
}

// Slug turns text into a lowercase ASCII slug: "L'addition" -> "l-addition",
// "un café" -> "un-cafe".
func Slug(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	plain, _, err := transform.String(t, s)
	if err != nil {
		plain = s
	}
	var sb strings.Builder
	dash := false
	emit := func(s string) {
		if dash && sb.Len() > 0 {
			sb.WriteByte('-')
		}
		dash = false
		sb.WriteString(s)
	}
	for _, r := range strings.ToLower(plain) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			emit(string(r))
		case r == 'ß':
			emit("ss")
		case r == 'œ':
			emit("oe")
		case r == 'æ':
			emit("ae")
		default:
			dash = true
		}
	}
	return sb.String()
}

func slotNames(s string) []string {
	var names []string
	for _, m := range slotRE.FindAllStringSubmatch(s, -1) {
		names = append(names, m[1])
	}
	return names
}

func capitalise(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

func lastRune(s string) string {
	s = strings.TrimSpace(s)
	r, _ := utf8.DecodeLastRuneInString(s)
	return string(r)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func fileBase(f string) string {
	if i := strings.LastIndexByte(f, '/'); i >= 0 {
		return f[i+1:]
	}
	return f
}
