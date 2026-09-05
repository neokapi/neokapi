package fixture

import (
	_ "embed"
	"encoding/json"
	"path/filepath"
	"sort"
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/stretchr/testify/require"
)

// The record every face is held to.
//
// The shapes are projections, not the faces' own structs. Three of the four
// questions are answered by types that differ per face: the desktop counts
// coverage from the block store while `kapi status` counts from the working
// tree, and the desktop's findings carry a category where the report carries a
// rule. So the contract is the facts every face must agree on, and each
// suite's projection is where a face states what it believes it is saying.

// Answers is the record every face is held to.
type Answers struct {
	ContextAt     ContextFacts `json:"context_at"`
	ContextSearch SearchFacts  `json:"context_search"`
	Status        StatusFacts  `json:"status"`
	Check         CheckFacts   `json:"check"`
}

// Findings is the recorded check findings, in their recorded order.
func (a Answers) Findings() []FindingFacts { return a.Check.Findings }

// PointFacts is where in the project a question landed.
type PointFacts struct {
	Path       string `json:"path"`
	Profile    string `json:"profile"`
	Channel    string `json:"channel"`
	Collection string `json:"collection"`
	Default    bool   `json:"default"`
}

// VoiceFacts is the voice in force at a point.
type VoiceFacts struct {
	Name  string `json:"name"`
	Field string `json:"field"`
}

// TermFacts is one term a face reported.
//
// Uses and UseBlocks are how often, and in how many blocks, the fixture's
// extracted content uses the term. Every face reads them from the same
// context-graph edges the fixture's extraction wrote, which is what makes one
// number assertable across faces: a face that counted another way, or from
// another moment, would disagree with the record here. The by-location answer
// reports terms without their uses on every face, so its rows carry zero.
type TermFacts struct {
	ConceptID   string `json:"concept_id"`
	Term        string `json:"term"`
	Status      string `json:"status"`
	Discouraged bool   `json:"discouraged"`
	Replacement string `json:"replacement"`
	Uses        int    `json:"uses"`
	UseBlocks   int    `json:"use_blocks"`
}

// ContextFacts is the answer to "what applies here".
type ContextFacts struct {
	Point      PointFacts  `json:"point"`
	Voice      VoiceFacts  `json:"voice"`
	Terms      []TermFacts `json:"terms"`
	TermsTotal int         `json:"terms_total"`
}

// SearchFacts is the answer to "what do we know about this".
type SearchFacts struct {
	Query     string      `json:"query"`
	Scope     string      `json:"scope"`
	Terms     []TermFacts `json:"terms"`
	Precedent []string    `json:"precedent"`
}

// LocaleFacts is one locale's standing.
type LocaleFacts struct {
	Locale     string `json:"locale"`
	Translated int    `json:"translated_pct"`
}

// StatusFacts is the answer to "where does this project stand".
type StatusFacts struct {
	Project     string        `json:"project"`
	Collections []string      `json:"collections"`
	Locales     []LocaleFacts `json:"locales"`
}

// FindingFacts is one thing a check objected to.
type FindingFacts struct {
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

// CheckFacts is what a check objected to.
//
// The verdict is deliberately absent. Each face gates on its own bar (the CLI
// takes it from flags, the MCP tools apply check.DefaultGate, and the desktop
// reports a score with no gate at all), so a contract that included pass/fail
// would fail on a difference the faces are entitled to have. What they may not
// disagree about is what is wrong with the content.
type CheckFacts struct {
	Findings []FindingFacts `json:"findings"`
}

// CheckFactsFrom projects a check report.
//
// Severity, message and suggestion are the facts all three faces carry. The
// report's rule ids and gate live on two of them, and holding the third to a
// field it has no place to put would fail the contract for a shape difference
// rather than a disagreement.
func CheckFactsFrom(r check.Report) CheckFacts {
	var f CheckFacts
	for _, d := range r.Findings {
		f.Findings = append(f.Findings, FindingFacts{
			Severity:   string(d.Severity),
			Message:    d.Message,
			Suggestion: d.Suggestion,
		})
	}
	SortFindings(f.Findings)
	return f
}

// SortFindings orders findings so a face that collects them in a different
// order still agrees about what it found.
func SortFindings(f []FindingFacts) {
	sort.Slice(f, func(i, j int) bool {
		if f[i].Message != f[j].Message {
			return f[i].Message < f[j].Message
		}
		return f[i].Severity < f[j].Severity
	})
}

// SortLocales orders locales by name.
func SortLocales(l []LocaleFacts) {
	sort.Slice(l, func(i, j int) bool { return l[i].Locale < l[j].Locale })
}

// SortTerms orders term rows by concept then term, the order the record keeps.
func SortTerms(out []TermFacts) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].ConceptID != out[j].ConceptID {
			return out[i].ConceptID < out[j].ConceptID
		}
		return out[i].Term < out[j].Term
	})
}

//go:embed testdata/answers.json
var goldenAnswers []byte

// Golden is the recorded answer set, embedded so a suite in any module reads
// the same bytes without reaching across module boundaries for a testdata
// directory it cannot see.
func Golden(t *testing.T) Answers {
	t.Helper()
	var a Answers
	require.NoError(t, json.Unmarshal(goldenAnswers, &a), "decode the recorded answers")
	return a
}

// GoldenPath is where the recorded answers live, relative to this package's
// directory, for the suite that rewrites them under -update.
func GoldenPath() string { return filepath.Join("testdata", "answers.json") }

// Marshal renders an answer set the way the record stores it.
func Marshal(t *testing.T, a Answers) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(a, "", "  ")
	require.NoError(t, err)
	return append(raw, '\n')
}
