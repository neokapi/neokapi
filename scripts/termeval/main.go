// Command termeval measures how term-check treats reviewed translations, so the
// choice between declared forms and a lexicon analyser rests on numbers from
// real content rather than on examples.
//
// It reads three corpora (the dogfood project's Norwegian content memory, the
// compass sample's catalogs and the tidewatch sample's Norwegian memory) and
// checks every unit under three matching modes:
//
//   - substring: the term and its preferred rendering as case-folded
//     substrings, the way term-check matched before stage 1;
//   - stage1: term-check as it is, with no declared forms;
//   - stage1+forms: term-check with the reviewed forms in testdata/forms.json.
//
// Per corpus, language and mode it reports the demands (a rule whose term the
// source uses), the fails, the false fails among labelled true uses, the
// demands labelled as not a use of the term, and the constructed negatives each
// mode wrongly passes.
//
// The labels in testdata/labels.json were made by an agent and are pending a
// person's review.
//
//	go run ./scripts/termeval
//	go run ./scripts/termeval -json report.json
//	go run ./scripts/termeval -unlabelled rows.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
)

// Mode names.
const (
	modeSubstring = "substring"
	modeStage1    = "stage1"
	modeForms     = "stage1+forms"
)

var modes = []string{modeSubstring, modeStage1, modeForms}

// corpusLanguages is the source language each corpus is written in.
var corpusSource = map[string]model.LocaleID{"dogfood": "en", "compass": "en-GB", "tidewatch": "en-GB"}

func main() {
	root := flag.String("root", ".", "repository root")
	jsonOut := flag.String("json", "", "write the report as JSON to this path")
	unlabelled := flag.String("unlabelled", "", "write the demands and fails that carry no label to this path")
	formsPath := flag.String("forms", "", "reviewed forms to apply in the stage1+forms mode (default: testdata/forms.json)")
	labelsPath := flag.String("labels", "", "labels to classify demands and fails with (default: testdata/labels.json)")
	flag.Parse()

	rep, err := run(*root, *formsPath, *labelsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "termeval:", err)
		os.Exit(1)
	}
	printTable(os.Stdout, rep)
	if *jsonOut != "" {
		if err := writeJSON(*jsonOut, rep); err != nil {
			fmt.Fprintln(os.Stderr, "termeval:", err)
			os.Exit(1)
		}
	}
	if *unlabelled != "" {
		if err := writeJSON(*unlabelled, rep.Unlabelled); err != nil {
			fmt.Fprintln(os.Stderr, "termeval:", err)
			os.Exit(1)
		}
	}
}

// Report is the whole measurement.
type Report struct {
	Note       string      `json:"note"`
	Rows       []Row       `json:"rows"`
	Unlabelled []Unlabeled `json:"unlabelled,omitempty"`
}

// Row is one corpus, language and mode.
type Row struct {
	Corpus string `json:"corpus"`
	Lang   string `json:"lang"`
	Mode   string `json:"mode"`
	Units  int    `json:"units"`
	// Demands are (unit, rule) pairs whose term the source uses, and Fails the
	// demands the target does not satisfy.
	Demands int `json:"demands"`
	Fails   int `json:"fails"`
	// TrueUses are demands whose source use is labelled a use of the term, and
	// FalseFails the fails among them whose target is labelled acceptable.
	TrueUses   int `json:"true_uses"`
	FalseFails int `json:"false_fails"`
	// OverDemands are demands whose source match is labelled not a use of the
	// term: an identifier, a sibling concept, a placeholder, another sense.
	OverDemands int `json:"over_demands"`
	// Negatives are constructed from satisfied demands; the Passed counts are
	// the negatives the mode wrongly passes.
	Negatives      int `json:"negatives"`
	DeletedPassed  int `json:"deleted_passed"`
	ClippedPassed  int `json:"clipped_passed"`
	UnlabelledRows int `json:"unlabelled_rows"`
	// FailClasses counts the labelled fails by class, and FalseFailClasses the
	// false fails among them.
	FailClasses      map[string]int `json:"fail_classes,omitempty"`
	FalseFailClasses map[string]int `json:"false_fail_classes,omitempty"`
}

// Unlabeled is a row a person or agent still has to classify.
type Unlabeled struct {
	Kind   string   `json:"kind"` // "source" or "target"
	Corpus string   `json:"corpus,omitempty"`
	Unit   string   `json:"unit,omitempty"`
	Term   string   `json:"term"`
	Word   string   `json:"word,omitempty"`
	Source string   `json:"source,omitempty"`
	Target string   `json:"target,omitempty"`
	Modes  []string `json:"modes"`
}

// run loads the corpora, rules, forms and labels and measures every mode.
func run(root, formsPath, labelsPath string) (*Report, error) {
	testdata := filepath.Join(root, "scripts", "termeval", "testdata")
	if formsPath == "" {
		formsPath = filepath.Join(testdata, "forms.json")
	}
	if labelsPath == "" {
		labelsPath = filepath.Join(testdata, "labels.json")
	}
	units, err := loadCorpus(root)
	if err != nil {
		return nil, err
	}
	dogfood, err := readBundle(filepath.Join(root, ".kapi", "terms.json"))
	if err != nil {
		return nil, err
	}
	samples, err := readBundle(filepath.Join(testdata, "samples.terms.json"))
	if err != nil {
		return nil, err
	}
	forms, err := loadForms(formsPath)
	if err != nil {
		return nil, err
	}
	labels, err := loadLabels(labelsPath)
	if err != nil {
		return nil, err
	}

	conceptsFor := map[string][]terms.Concept{"dogfood": dogfood, "compass": samples, "tidewatch": samples}
	formsFor := map[string]string{"dogfood": "dogfood", "compass": "samples", "tidewatch": "samples"}

	rep := &Report{Note: "Labels are agent-made and pending a person's review.", Unlabelled: []Unlabeled{}}
	rows := map[string]*Row{}
	pending := map[string]*Unlabeled{}
	for _, u := range units {
		src := corpusSource[u.Corpus]
		for _, mode := range modes {
			concepts := conceptsFor[u.Corpus]
			if mode == modeForms {
				concepts = withForms(concepts, forms[formsFor[u.Corpus]])
			}
			rules := rulesFor(mode, concepts, src, u.Target)
			key := u.Corpus + "\x00" + string(u.Target) + "\x00" + mode
			row := rows[key]
			if row == nil {
				row = &Row{Corpus: u.Corpus, Lang: string(u.Target), Mode: mode}
				rows[key] = row
			}
			row.Units++
			measureUnit(row, mode, u, rules, src, labels, pending)
		}
	}
	for _, r := range rows {
		rep.Rows = append(rep.Rows, *r)
	}
	sort.Slice(rep.Rows, func(i, j int) bool {
		a, b := rep.Rows[i], rep.Rows[j]
		if a.Corpus != b.Corpus {
			return a.Corpus < b.Corpus
		}
		if a.Lang != b.Lang {
			return a.Lang < b.Lang
		}
		return modeIndex(a.Mode) < modeIndex(b.Mode)
	})
	for _, p := range pending {
		rep.Unlabelled = append(rep.Unlabelled, *p)
	}
	sort.Slice(rep.Unlabelled, func(i, j int) bool {
		a, b := rep.Unlabelled[i], rep.Unlabelled[j]
		return a.Kind+a.Corpus+a.Unit+a.Term+a.Word < b.Kind+b.Corpus+b.Unit+b.Term+b.Word
	})
	return rep, nil
}

// measureUnit adds one unit's demands, fails, labels and negatives to row.
func measureUnit(row *Row, mode string, u Unit, rules []profile.TermRule, src model.LocaleID, labels *Labels, pending map[string]*Unlabeled) {
	source := model.RunsText(u.Source)
	target := model.RunsText(u.TargetRuns)
	demanded, failed := decide(mode, rules, src, u.Target, source, target)

	for _, rule := range rules {
		if !demanded[rule.Term] {
			continue
		}
		row.Demands++
		word := matchedWord(mode, source, rule.Term)
		class, ok := labels.source(rule.Term, word)
		switch {
		case !ok:
			row.UnlabelledRows++
			note(pending, Unlabeled{Kind: "source", Term: rule.Term, Word: word}, mode)
		case class == "use":
			row.TrueUses++
		default:
			row.OverDemands++
		}

		if failed[rule.Term] {
			row.Fails++
			tclass, tok := labels.target(u.Corpus, u.ID, rule.Term)
			switch {
			case !tok:
				row.UnlabelledRows++
				note(pending, Unlabeled{Kind: "target", Corpus: u.Corpus, Unit: u.ID, Term: rule.Term, Source: source, Target: target}, mode)
			default:
				row.FailClasses = increment(row.FailClasses, tclass)
				if ok && class == "use" && acceptableTarget(tclass) {
					row.FalseFails++
					row.FalseFailClasses = increment(row.FalseFailClasses, tclass)
				}
			}
			continue
		}

		// A satisfied demand yields negatives: the words that carry a rendering
		// deleted, or replaced by a clipped word.
		for _, neg := range negativesForAll(target, surfacesFor(mode, rule)) {
			row.Negatives++
			_, negFailed := decide(mode, []profile.TermRule{rule}, src, u.Target, source, neg.Target)
			if negFailed[rule.Term] {
				continue
			}
			if neg.Kind == "deleted" {
				row.DeletedPassed++
			} else {
				row.ClippedPassed++
			}
		}
	}
}

func increment(m map[string]int, key string) map[string]int {
	if m == nil {
		m = map[string]int{}
	}
	m[key]++
	return m
}

func note(pending map[string]*Unlabeled, row Unlabeled, mode string) {
	key := row.Kind + "\x00" + row.Corpus + "\x00" + row.Unit + "\x00" + row.Term + "\x00" + row.Word
	if p, ok := pending[key]; ok {
		if !slices.Contains(p.Modes, mode) {
			p.Modes = append(p.Modes, mode)
		}
		return
	}
	row.Modes = []string{mode}
	pending[key] = &row
}

// rulesFor derives the rules a mode checks with from the same concepts.
func rulesFor(mode string, concepts []terms.Concept, src, tgt model.LocaleID) []profile.TermRule {
	rules := terms.RulesFromConcepts(concepts, src, tgt)
	out := rules[:0]
	for _, r := range rules {
		if r.Replacement == "" {
			continue
		}
		switch mode {
		case modeSubstring:
			r = profile.TermRule{Term: r.Term, Replacement: r.Replacement, ConceptID: r.ConceptID}
		case modeStage1:
			r.Forms, r.ReplacementForms = nil, nil
			accepted := make([]profile.Rendering, 0, len(r.Accepted))
			for _, a := range r.Accepted {
				accepted = append(accepted, profile.Rendering{Text: a.Text})
			}
			r.Accepted = accepted
		}
		out = append(out, r)
	}
	return out
}

// decide returns the rules a mode demands of source and those target fails,
// keyed by term.
func decide(mode string, rules []profile.TermRule, src, tgt model.LocaleID, source, target string) (demanded, failed map[string]bool) {
	if mode == modeSubstring {
		demanded, failed = map[string]bool{}, map[string]bool{}
		for _, r := range rules {
			if !containsFold(source, r.Term) {
				continue
			}
			demanded[r.Term] = true
			if !containsFold(target, r.Replacement) {
				failed[r.Term] = true
			}
		}
		return demanded, failed
	}
	cfg := &coretools.TermCheckConfig{TermRules: rules, SourceLocale: src, TargetLocale: tgt}
	// Against an empty target every demanded rule fails, so the violations
	// name exactly the rules term-check demands.
	errs, warns := coretools.TermCheckViolations(cfg, source, "")
	demanded = violatedTerms(errs, warns)
	errs, warns = coretools.TermCheckViolations(cfg, source, target)
	return demanded, violatedTerms(errs, warns)
}

var violationTerm = regexp.MustCompile(`^term ("(?:[^"\\]|\\.)*") found in source`)

func violatedTerms(lists ...[]string) map[string]bool {
	out := map[string]bool{}
	for _, list := range lists {
		for _, msg := range list {
			if m := violationTerm.FindStringSubmatch(msg); m != nil {
				if term, err := strconv.Unquote(m[1]); err == nil {
					out[term] = true
				}
			}
		}
	}
	return out
}

// surfacesFor is every string a mode accepts as the rule's rendering.
func surfacesFor(mode string, rule profile.TermRule) []string {
	if mode == modeSubstring {
		return []string{rule.Replacement}
	}
	var out []string
	for _, r := range rule.Renderings() {
		out = append(out, r.Text)
		out = append(out, r.Forms...)
	}
	return out
}

// matchedWord is the source word a demand was read from, lowercased: the word
// containing the term for substring matching, and the word the term or its
// inflection spans otherwise.
func matchedWord(mode string, source, term string) string {
	lower := strings.ToLower(source)
	needle := strings.ToLower(term)
	if mode != modeSubstring {
		p := check.PrepareText(check.TermText(source))
		if hits := check.FindEnglishInflectionsIn(p, term, false); len(hits) > 0 {
			return strings.ToLower(source[hits[0][0]:hits[0][1]])
		}
	}
	for _, w := range wordPattern.FindAllString(lower, -1) {
		if strings.Contains(w, needle) {
			return w
		}
	}
	return needle
}

func containsFold(text, sub string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(sub))
}

func modeIndex(m string) int {
	for i, x := range modes {
		if x == m {
			return i
		}
	}
	return len(modes)
}

func readBundle(path string) ([]terms.Concept, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	file, err := ktb.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return file.Concepts, nil
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func printTable(w *os.File, rep *Report) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "corpus\tlang\tmode\tunits\tdemands\tfails\tfalse fails / true uses\tover-demands\tnegatives passed (deleted, clipped)\tunlabelled")
	for _, r := range rep.Rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%d\t%d / %d\t%d\t%d, %d of %d\t%d\n",
			r.Corpus, r.Lang, r.Mode, r.Units, r.Demands, r.Fails, r.FalseFails, r.TrueUses, r.OverDemands,
			r.DeletedPassed, r.ClippedPassed, r.Negatives, r.UnlabelledRows)
	}
	_ = tw.Flush()

	fmt.Fprintln(w, "\nfails by label class (false fails marked *)")
	for _, r := range rep.Rows {
		if len(r.FailClasses) == 0 {
			continue
		}
		classes := make([]string, 0, len(r.FailClasses))
		for c := range r.FailClasses {
			classes = append(classes, c)
		}
		sort.Strings(classes)
		parts := make([]string, 0, len(classes))
		for _, c := range classes {
			mark := ""
			if acceptableTarget(c) {
				mark = "*"
			}
			parts = append(parts, fmt.Sprintf("%s%s %d", c, mark, r.FailClasses[c]))
		}
		fmt.Fprintf(w, "  %s %s %s: %s\n", r.Corpus, r.Lang, r.Mode, strings.Join(parts, ", "))
	}
	fmt.Fprintln(w, "\n"+rep.Note)
}
