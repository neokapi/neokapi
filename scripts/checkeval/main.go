// Command checkeval runs the content checks over a labeled corpus and reports
// precision/recall/F1 per check, so check quality is measured — not just
// asserted on a handful of unit cases — and can be regression-gated and tracked
// over time as the corpus grows from real corrections (issue #759). It mirrors
// the parity harness: a fixture corpus → a metric report → a dashboard, with a
// companion test (main_test.go) that fails on any new mistake.
//
//	go run ./scripts/checkeval            # regenerate the dashboard JSON
//	go test ./scripts/checkeval           # regression gate
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	coretools "github.com/neokapi/neokapi/core/tools"
)

// Case is one labeled corpus entry: an input plus the finding categories a
// correct check should produce.
type Case struct {
	ID         string   `json:"id"`
	Check      string   `json:"check"`
	Source     string   `json:"source"`
	Target     string   `json:"target"`
	TargetLang string   `json:"target_lang"`
	DNT        []string `json:"dnt"`
	Expect     []string `json:"expect"`
	// ExpectScore, when set, pins the rolled-up compliance score (0-100) the
	// case must produce — a score-calibration anchor (#758). A change to the
	// severity weights or a checker's severity choice that moves the score off
	// this value fails the gate.
	ExpectScore *int   `json:"expect_score,omitempty"`
	Note        string `json:"note"`
}

// Corpus is the labeled eval set.
type Corpus struct {
	Cases []Case `json:"cases"`
}

// CaseResult records the per-case outcome for the dashboard.
type CaseResult struct {
	ID          string   `json:"id"`
	Check       string   `json:"check"`
	Expect      []string `json:"expect"`
	Got         []string `json:"got"`
	TP          int      `json:"tp"`
	FP          int      `json:"fp"`
	FN          int      `json:"fn"`
	Score       int      `json:"score"`                  // rolled-up compliance score (0-100)
	ExpectScore *int     `json:"expect_score,omitempty"` // pinned score, if calibrated
	ScoreOK     bool     `json:"score_ok"`               // true when no ExpectScore or it matches
	Note        string   `json:"note"`
}

// Metric is precision/recall/F1 over a set of cases.
type Metric struct {
	Check     string  `json:"check"`
	Cases     int     `json:"cases"`
	TP        int     `json:"tp"`
	FP        int     `json:"fp"`
	FN        int     `json:"fn"`
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	F1        float64 `json:"f1"`
}

// Report is the full eval result consumed by the /check-eval dashboard.
type Report struct {
	GeneratedNote string             `json:"generated_note"`
	Total         Metric             `json:"total"`
	ByCheck       []Metric           `json:"by_check"`
	Cases         []CaseResult       `json:"cases"`
	Corrections   *CorrectionsReport `json:"corrections,omitempty"`
}

// runCase runs the case's check and returns the set of finding categories and
// the rolled-up compliance score over the findings (the score the gate pins for
// calibrated cases).
func runCase(c Case) (cats []string, score int, err error) {
	loc := model.LocaleID(c.TargetLang)
	b := &model.Block{ID: c.ID, Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: c.Source}}}}
	if c.Target != "" {
		tool.NewTargetView(b).SetTargetText(loc, c.Target)
	}
	var tl *tool.BaseTool
	switch c.Check {
	case "dnt":
		cfg := coretools.NewDNTCheckConfig(loc)
		cfg.Terms = c.DNT
		tl = coretools.NewDNTCheckTool(cfg)
	case "placeholder":
		tl = coretools.NewPlaceholderCheckTool(coretools.NewPlaceholderCheckConfig(loc))
	default:
		return nil, 0, fmt.Errorf("checkeval: unknown check %q in case %q", c.Check, c.ID)
	}
	if err := tl.Annotate(tool.NewBlockView(b)); err != nil {
		return nil, 0, fmt.Errorf("checkeval: run %q: %w", c.ID, err)
	}
	set := map[string]bool{}
	var findings []check.Finding
	if ann, ok := b.Annotations[check.AnnotationKey].(*check.FindingsAnnotation); ok {
		findings = ann.Findings
		for _, f := range findings {
			set[f.Category] = true
		}
	}
	return sortedKeys(set), check.CalculateScore(findings).Overall, nil
}

// Evaluate runs every case and aggregates per-check and overall metrics.
func Evaluate(corpus Corpus) (Report, error) {
	rep := Report{GeneratedNote: "Run `go run ./scripts/checkeval` to regenerate."}
	perCheck := map[string]*Metric{}
	for _, c := range corpus.Cases {
		got, score, err := runCase(c)
		if err != nil {
			return Report{}, err
		}
		exp := toSet(c.Expect)
		gotSet := toSet(got)
		tp, fp, fn := 0, 0, 0
		for cat := range gotSet {
			if exp[cat] {
				tp++
			} else {
				fp++
			}
		}
		for cat := range exp {
			if !gotSet[cat] {
				fn++
			}
		}
		scoreOK := c.ExpectScore == nil || *c.ExpectScore == score
		rep.Cases = append(rep.Cases, CaseResult{
			ID: c.ID, Check: c.Check, Expect: c.Expect, Got: got, TP: tp, FP: fp, FN: fn,
			Score: score, ExpectScore: c.ExpectScore, ScoreOK: scoreOK, Note: c.Note,
		})

		m := perCheck[c.Check]
		if m == nil {
			m = &Metric{Check: c.Check}
			perCheck[c.Check] = m
		}
		m.Cases++
		m.TP += tp
		m.FP += fp
		m.FN += fn
		rep.Total.TP += tp
		rep.Total.FP += fp
		rep.Total.FN += fn
		rep.Total.Cases++
	}
	rep.Total.Check = "all"
	score(&rep.Total)
	for _, k := range sortedMetricKeys(perCheck) {
		m := perCheck[k]
		score(m)
		rep.ByCheck = append(rep.ByCheck, *m)
	}
	return rep, nil
}

// score fills precision/recall/F1. With no positives to find or predict, the
// metric is 1.0 (nothing to get wrong).
func score(m *Metric) {
	if m.TP+m.FP == 0 {
		m.Precision = 1
	} else {
		m.Precision = float64(m.TP) / float64(m.TP+m.FP)
	}
	if m.TP+m.FN == 0 {
		m.Recall = 1
	} else {
		m.Recall = float64(m.TP) / float64(m.TP+m.FN)
	}
	if m.Precision+m.Recall == 0 {
		m.F1 = 0
	} else {
		m.F1 = 2 * m.Precision * m.Recall / (m.Precision + m.Recall)
	}
}

func toSet(xs []string) map[string]bool {
	s := map[string]bool{}
	for _, x := range xs {
		s[x] = true
	}
	return s
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedMetricKeys(m map[string]*Metric) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// LoadCorpus reads a corpus JSON file.
func LoadCorpus(path string) (Corpus, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Corpus{}, err
	}
	var c Corpus
	if err := json.Unmarshal(data, &c); err != nil {
		return Corpus{}, fmt.Errorf("checkeval: parse %s: %w", path, err)
	}
	return c, nil
}

func main() {
	corpusPath := flag.String("corpus", "core/check/evaldata/corpus.json", "labeled corpus JSON")
	correctionsPath := flag.String("corrections", "core/check/evaldata/corrections.json", "simulated correction-stream JSON")
	out := flag.String("out", "web/docs/src/pages/check-eval/_eval.json", "dashboard report JSON")
	flag.Parse()

	corpus, err := LoadCorpus(*corpusPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	rep, err := Evaluate(corpus)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if cc, cerr := LoadCorrectionsCorpus(*correctionsPath); cerr == nil {
		cr := EvaluateCorrections(cc)
		rep.Corrections = &cr
	} else {
		fmt.Fprintln(os.Stderr, "checkeval: corrections stream:", cerr)
	}
	data, _ := json.MarshalIndent(rep, "", "  ")
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	calibrated, scoreDrift := 0, 0
	for _, c := range rep.Cases {
		if c.ExpectScore != nil {
			calibrated++
			if !c.ScoreOK {
				scoreDrift++
			}
		}
	}
	fmt.Printf("checkeval: %d cases · P %.2f R %.2f F1 %.2f · FP %d FN %d · calibrated %d/%d (drift %d) → %s\n",
		rep.Total.Cases, rep.Total.Precision, rep.Total.Recall, rep.Total.F1, rep.Total.FP, rep.Total.FN,
		calibrated-scoreDrift, calibrated, scoreDrift, *out)
	if rep.Corrections != nil {
		c := rep.Corrections
		fmt.Printf("checkeval: corrections-stream · %d promoted/%d · P %.2f R %.2f F1 %.2f · caught %d missed %d over-flagged %d\n",
			c.Promoted, c.Total, c.Precision, c.Recall, c.F1, c.TP, c.FN, c.FP)
	}
}
