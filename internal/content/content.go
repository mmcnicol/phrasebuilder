// Package content reads course units from YAML files, checks them, and
// expands each pattern into the sentences and words a learner drills.
//
// Files live at <lang>/unit-NN-<unit>.yaml, for example
// fr/unit-01-cafe.yaml. See docs/milestones/02-content-pipeline.md for the
// schema.
package content

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Languages are the course languages, by code.
var Languages = map[string]string{
	"fr": "French",
	"it": "Italian",
	"de": "German",
}

// Unit is one YAML file: a situation such as "At the café".
type Unit struct {
	ID       string    `yaml:"unit"`
	Order    int       `yaml:"order"`
	Title    string    `yaml:"title"`
	Patterns []Pattern `yaml:"patterns"`

	Language string `yaml:"-"`
	File     string `yaml:"-"`
}

// Pattern is a sentence frame with {slots}, carrying one grammar point.
type Pattern struct {
	ID     string              `yaml:"id"`
	Status string              `yaml:"status"`
	Frame  string              `yaml:"frame"`
	En     string              `yaml:"en"`
	Note   string              `yaml:"note"`
	Slots  map[string][]Filler `yaml:"slots"`

	Line      int            `yaml:"-"`
	SlotLines map[string]int `yaml:"-"`
}

// Filler is a word or phrase that fits a slot.
type Filler struct {
	Text string `yaml:"fr"`
	En   string `yaml:"en"`
	G    string `yaml:"g"` // grammatical gender: m, f or blank

	Line int `yaml:"-"`
}

// Problem is a content error, located in a file.
type Problem struct {
	File string
	Line int
	Msg  string
}

func (p Problem) String() string {
	if p.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", p.File, p.Line, p.Msg)
	}
	return fmt.Sprintf("%s: %s", p.File, p.Msg)
}

// Problems is a list of content errors.
type Problems []Problem

func (ps Problems) Error() string {
	s := make([]string, len(ps))
	for i, p := range ps {
		s[i] = p.String()
	}
	return strings.Join(s, "\n")
}

// Load reads every <lang>/*.yaml file in fsys, sorted by language and file
// name. Files that cannot be parsed are reported as problems; the rest are
// returned.
func Load(fsys fs.FS) ([]*Unit, Problems) {
	var units []*Unit
	var problems Problems
	files, err := fs.Glob(fsys, "*/*.yaml")
	if err != nil {
		return nil, Problems{{File: ".", Msg: err.Error()}}
	}
	sort.Strings(files)
	for _, file := range files {
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			problems = append(problems, Problem{File: file, Msg: err.Error()})
			continue
		}
		u, err := parse(data)
		if err != nil {
			problems = append(problems, yamlProblems(file, err)...)
			continue
		}
		u.Language = path.Dir(file)
		u.File = file
		units = append(units, u)
	}
	if len(files) == 0 {
		problems = append(problems, Problem{File: ".", Msg: "no <lang>/*.yaml content files found"})
	}
	return units, problems
}

// parse decodes one unit strictly (unknown keys are errors) and records the
// line of each pattern, slot and filler for error messages.
func parse(data []byte) (*Unit, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var u Unit
	if err := dec.Decode(&u); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("file is empty")
		}
		return nil, err
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if len(root.Content) == 0 {
		return &u, nil
	}
	patterns := mapValue(root.Content[0], "patterns")
	if patterns == nil {
		return &u, nil
	}
	for i, pn := range patterns.Content {
		if i >= len(u.Patterns) {
			break
		}
		p := &u.Patterns[i]
		p.Line = pn.Line
		slots := mapValue(pn, "slots")
		if slots == nil {
			continue
		}
		p.SlotLines = make(map[string]int)
		for j := 0; j+1 < len(slots.Content); j += 2 {
			name, fillers := slots.Content[j].Value, slots.Content[j+1]
			p.SlotLines[name] = slots.Content[j].Line
			for k, fn := range fillers.Content {
				if k < len(p.Slots[name]) {
					p.Slots[name][k].Line = fn.Line
				}
			}
		}
	}
	return &u, nil
}

var yamlLineRE = regexp.MustCompile(`^line (\d+): (.*)$`)

// yamlProblems turns a YAML error, which may hold several messages such as
// "line 49: field nots not found in type content.Pattern", into Problems.
func yamlProblems(file string, err error) Problems {
	msgs := []string{strings.TrimPrefix(err.Error(), "yaml: ")}
	var te *yaml.TypeError
	if errors.As(err, &te) {
		msgs = te.Errors
	}
	var ps Problems
	for _, m := range msgs {
		p := Problem{File: file, Msg: m}
		if sm := yamlLineRE.FindStringSubmatch(m); sm != nil {
			p.Line, _ = strconv.Atoi(sm[1])
			p.Msg = strings.Replace(sm[2], "in type content.", "in ", 1)
		}
		ps = append(ps, p)
	}
	return ps
}

// mapValue returns the value for key in a YAML mapping node, or nil.
func mapValue(n *yaml.Node, key string) *yaml.Node {
	if n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}
