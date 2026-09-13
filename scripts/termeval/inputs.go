package main

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// FormsEntry is one term's reviewed forms in testdata/forms.json.
type FormsEntry struct {
	ConceptID string   `json:"concept_id"`
	Locale    string   `json:"locale"`
	Term      string   `json:"term"`
	Forms     []string `json:"forms"`
}

// loadForms reads the reviewed forms, grouped by the bundle they were proposed
// for ("dogfood" or "samples").
func loadForms(path string) (map[string][]FormsEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Dogfood []FormsEntry `json:"dogfood"`
		Samples []FormsEntry `json:"samples"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return map[string][]FormsEntry{"dogfood": doc.Dogfood, "samples": doc.Samples}, nil
}

// withForms returns a copy of concepts in which every term the entries name
// declares the reviewed forms. The concepts passed in are not modified.
func withForms(concepts []terms.Concept, entries []FormsEntry) []terms.Concept {
	type key struct {
		concept string
		locale  model.LocaleID
		text    string
	}
	want := make(map[key][]string, len(entries))
	for _, e := range entries {
		want[key{e.ConceptID, model.NormalizeLocale(model.LocaleID(e.Locale)), strings.ToLower(e.Term)}] = e.Forms
	}
	out := make([]terms.Concept, len(concepts))
	for i, c := range concepts {
		c.Terms = slices.Clone(c.Terms)
		for j := range c.Terms {
			t := &c.Terms[j]
			if forms, ok := want[key{c.ID, model.NormalizeLocale(t.Locale), strings.ToLower(t.Text)}]; ok {
				t.Forms = terms.NormalizeForms(t.Text, forms)
			}
		}
		out[i] = c
	}
	return out
}

// Labels classify demands and fails. They are agent-made, pending a person's
// review.
type Labels struct {
	Note   string        `json:"note"`
	Source []SourceLabel `json:"source"`
	Target []TargetLabel `json:"target"`

	sourceIndex map[string]string
	targetIndex map[string]string
}

// SourceLabel classifies what a source word is, for one term: "use" (a use of
// the term), or "identifier", "sibling", "placeholder" or "polysemy" (a match
// that is not a use of it).
type SourceLabel struct {
	Term  string `json:"term"`
	Word  string `json:"word"`
	Class string `json:"class"`
	Note  string `json:"note,omitempty"`
}

// TargetLabel classifies why one unit's target fails a rule: "form",
// "compound", "derivation" or "admitted" (the target is acceptable, so the fail
// is false), or "different-word", "paraphrase", "identifier", "drift" or
// "polysemy" (the fail is true).
type TargetLabel struct {
	Corpus string `json:"corpus"`
	Unit   string `json:"unit"`
	Term   string `json:"term"`
	Class  string `json:"class"`
	Note   string `json:"note,omitempty"`
}

// sourceClasses and targetClasses are the classes a label may carry.
var (
	sourceClasses = map[string]bool{"use": true, "identifier": true, "sibling": true, "placeholder": true, "polysemy": true}
	targetClasses = map[string]bool{
		"form": true, "compound": true, "derivation": true, "admitted": true,
		"different-word": true, "paraphrase": true, "identifier": true, "drift": true, "polysemy": true,
	}
)

// acceptableTarget reports whether a target class makes a fail a false fail.
func acceptableTarget(class string) bool {
	switch class {
	case "form", "compound", "derivation", "admitted":
		return true
	}
	return false
}

// loadLabels reads the label file. A missing file is an empty set, so the
// first run can emit the rows to label.
func loadLabels(path string) (*Labels, error) {
	l := &Labels{}
	data, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(data, l); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	l.sourceIndex = map[string]string{}
	for _, s := range l.Source {
		if !sourceClasses[s.Class] {
			return nil, fmt.Errorf("%s: source label %q/%q has unknown class %q", path, s.Term, s.Word, s.Class)
		}
		l.sourceIndex[strings.ToLower(s.Term)+"\x00"+strings.ToLower(s.Word)] = s.Class
	}
	l.targetIndex = map[string]string{}
	for _, t := range l.Target {
		if !targetClasses[t.Class] {
			return nil, fmt.Errorf("%s: target label %s/%s/%q has unknown class %q", path, t.Corpus, t.Unit, t.Term, t.Class)
		}
		l.targetIndex[t.Corpus+"\x00"+t.Unit+"\x00"+strings.ToLower(t.Term)] = t.Class
	}
	return l, nil
}

func (l *Labels) source(term, word string) (string, bool) {
	c, ok := l.sourceIndex[strings.ToLower(term)+"\x00"+strings.ToLower(word)]
	return c, ok
}

func (l *Labels) target(corpus, unit, term string) (string, bool) {
	c, ok := l.targetIndex[corpus+"\x00"+unit+"\x00"+strings.ToLower(term)]
	return c, ok
}
