package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// Judging the pair, under conditions that make a verdict worth reading.
//
// The lab scored nothing on purpose: nobody has written down what a good user
// guide is, and a rubric invented for it would measure the rubric. That still
// holds for the aggregate. What a panel CAN do is point at specific
// differences, and the pointing is worth having even where the score is not.
//
// Three things make the difference between a panel and theatre:
//
//   - Different lenses, not copies. Three runs of one prompt share their
//     biases, so a 3-of-3 majority is one judge with extra cost. These ask
//     three questions with different failure modes, one of which (grounding) is
//     nearly objective because the source is right there.
//   - Blind, and order-swapped. A judge told which document is governed
//     confirms it, and a judge shown the voice guide knows what to look for.
//     Neither is given. Each pair is judged twice with the order reversed,
//     because a judge that always picks the first is measuring position.
//   - A null pair. Two documents from the SAME arm, where the right answer is a
//     coin flip. A panel with a strong preference there is measuring something
//     other than the condition, and nothing else in the run would reveal it.
//
// What this is not: independent. Four Claude models share their training, so
// these are independent RUNS, not independent minds. The page says so.
//
// The aggregate stays withheld until its agreement with a person is measured,
// which is the rule this repository already applies to the context eval's
// judged dimension. See docs/internals/evals.md.

// Lens is one question asked of a pair.
type Lens struct {
	ID string
	// Question is what the judge is asked. It never mentions governance, a
	// voice profile, or which document came from where.
	Question string
	// Why records what this lens catches that the others do not, for the page.
	Why string
}

// The three lenses, and what two runs of them established.
//
// A lens is worth its calls only if it agrees with itself when the same pair is
// shown the other way round. Two panels over the same 8 pairs, same judge, the
// second with deliberately sharpened questions:
//
//	             agreement          preference
//	grounding    4/7 -> 4/7         gov 5/bare 9  -> gov 3/bare 10
//	audience     5/8 -> 5/8         gov 7/bare 9  -> gov 9/bare 5
//	voice        6/8 -> 4/8         gov 12/bare 4 -> gov 10/bare 6
//
// Two things follow, and the second is the one that matters.
//
// The sharpening changed nothing measurable, and the audience preference
// REVERSED between runs on identical documents. A verdict that flips direction
// when the question is reworded is not a measurement of the documents.
//
// And the sample cannot support any of these numbers either way. 4/8 is 50%
// with a 95% interval of roughly 15% to 85%; 6/8 is 75% with 45% to 100%. Those
// overlap almost entirely, so "voice held at 6/8 and then fell to 4/8" is one
// pair moving, not a change in the lens. Eight pairs cannot separate a lens
// from chance, and cannot separate the lenses from each other.
//
// So the panel is UNDERPOWERED rather than broken, and the fix is pairs rather
// than lenses: distinguishing 75% agreement from 50% needs something like 30 to
// 40 pairs. Eight is 4 models x 2 coordinates. Getting to 40 means more
// coordinates, more models, or several samples per cell — and nothing about the
// questions is worth tuning until the controls can tell tuning apart from noise.
//
// What the panel is good for meanwhile is the annotations: a judge that cannot
// reliably rank two documents can still point at a specific difference between
// them, and a quoted passage is checkable in a way a preference count is not.
// The page publishes those and no score.
var lenses = []Lens{
	{
		ID: "grounding",
		Question: "Find the three most specific factual claims in each document: a flag's " +
			"exact behaviour, a file path, a default value, a name. For each, say whether the " +
			"repository supports it. Then answer: which document has fewer claims the source " +
			"does not support?",
		Why: "The nearest thing to an objective question here: the source is in the " +
			"repository, so a claim is either in it or invented. Asking for specific " +
			"claims first gives the judge something to point at; asking which document " +
			"was 'better grounded' agreed with itself 4 times in 7.",
	},
	{
		ID: "audience",
		Question: "The stated reader is described in the task. List the terms and concepts each " +
			"document expects that reader to already know without explaining them. Then answer: " +
			"which document expects less that this particular reader would not have?",
		Why: "Whether the writing lands for the person it names, which is what a " +
			"coordinate is for and what no term list can check. Naming the assumed " +
			"knowledge first makes the comparison concrete; asking which was easier to " +
			"follow agreed with itself 5 times in 8.",
	},
	{
		ID: "voice",
		Question: "Compare each document against the project's own prose in the repository " +
			"(README, GUIDE, FAQ). Name two habits the project's writing has: how it treats " +
			"its own name, whether it makes claims or shows them, how it refers to other " +
			"tools. Then answer: which document follows those habits more closely?",
		Why: "Whether it sounds like the project rather than like a model, judged " +
			"against the project's own writing rather than a rubric. The lens that " +
			"held on the first run, at 6 of 8, and the one whose annotations were " +
			"specific enough to check.",
	},
}

// Pair is two documents shown to a judge without saying where either came from.
type Pair struct {
	// Kind is "real" (the two arms) or "null" (two samples of the same arm).
	Kind string `json:"kind"`
	// Model and Audience name the cell both documents came from.
	Model    string `json:"model"`
	Audience string `json:"audience"`
	// A and B are the documents as shown, and AIs records which arm A was, kept
	// out of the judge's sight and used only to read the verdict afterwards.
	A    string `json:"-"`
	B    string `json:"-"`
	AIs  string `json:"aIs"`
	BIs  string `json:"bIs"`
	Task string `json:"-"`
}

// Verdict is one judge's answer to one lens on one pair, in one order.
type Verdict struct {
	Lens     string `json:"lens"`
	Model    string `json:"model"`
	Audience string `json:"audience"`
	Kind     string `json:"kind"`
	// Swapped records which way round the pair was shown.
	Swapped bool `json:"swapped"`
	// Chose is "A", "B" or "neither", as the judge saw them.
	Chose string `json:"chose"`
	// Won translates that into the arm, which is what a reader wants. Empty
	// when the judge chose neither.
	Won string `json:"won,omitempty"`
	// Quote is a passage from the document it preferred, and Because is why.
	// The quotes are the published half: they point at a difference rather than
	// asserting one.
	Quote   string `json:"quote,omitempty"`
	Because string `json:"because,omitempty"`
	Err     string `json:"error,omitempty"`
}

// judgeSchema forces an answer the run can read without parsing prose.
func judgeSchema() aiprovider.JSONSchema {
	return aiprovider.JSONSchema{
		Name:        "pair_verdict",
		Description: "Which of two documents better answers one question, and the passage that shows it",
		Strict:      true,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"chose": map[string]any{
					"type": "string",
					"enum": []string{"A", "B", "neither"},
				},
				"quote": map[string]any{
					"type":        "string",
					"description": "A short passage from the chosen document that shows why. Empty when neither.",
				},
				"because": map[string]any{
					"type":        "string",
					"description": "One sentence naming the difference, referring to the passage.",
				},
			},
			"required":             []string{"chose", "quote", "because"},
			"additionalProperties": false,
		},
	}
}

// judgePair asks one lens about one pair, in one order.
func judgePair(ctx context.Context, llm aiprovider.LLMProvider, lens Lens, p Pair, swapped bool) Verdict {
	v := Verdict{Lens: lens.ID, Model: p.Model, Audience: p.Audience, Kind: p.Kind, Swapped: swapped}

	first, second := p.A, p.B
	firstIs, secondIs := p.AIs, p.BIs
	if swapped {
		first, second = p.B, p.A
		firstIs, secondIs = p.BIs, p.AIs
	}

	var sys strings.Builder
	sys.WriteString("You are comparing two documents written for the same task. ")
	sys.WriteString("Answer only the question asked. ")
	// The one instruction that is about the judging rather than the documents.
	sys.WriteString("You are not told how either document was produced, and there is nothing to infer from that: ")
	sys.WriteString("judge what is in front of you. \"neither\" is a real answer and is better than a guess.")

	user := fmt.Sprintf("The task both documents were written for:\n\n%s\n\n"+
		"QUESTION: %s\n\n=== DOCUMENT A ===\n\n%s\n\n=== DOCUMENT B ===\n\n%s",
		p.Task, lens.Question, first, second)

	resp, err := llm.ChatStructured(ctx, []aiprovider.Message{
		aiprovider.TextMessage(aiprovider.RoleSystem, sys.String()),
		aiprovider.TextMessage(aiprovider.RoleUser, user),
	}, judgeSchema())
	if err != nil {
		v.Err = err.Error()
		return v
	}
	var out struct {
		Chose   string `json:"chose"`
		Quote   string `json:"quote"`
		Because string `json:"because"`
	}
	if err := json.Unmarshal([]byte(resp.Content), &out); err != nil {
		v.Err = err.Error()
		return v
	}
	v.Chose, v.Quote, v.Because = out.Chose, out.Quote, out.Because
	switch out.Chose {
	case "A":
		v.Won = firstIs
	case "B":
		v.Won = secondIs
	}
	return v
}

// JudgeSummary is what the run can say about the panel itself, which is the
// half that decides whether the rest means anything.
type JudgeSummary struct {
	// Preference counts real pairs by which arm won, per lens.
	Preference map[string]map[string]int `json:"preference"`
	// PositionAgreement counts pairs where the two orders agreed. A judge that
	// always picks the first document disagrees with itself every time.
	PositionAgreement map[string][2]int `json:"positionAgreement"`
	// NullSplit is how the null pairs fell, per lens. The right answer is a
	// coin flip; a lopsided split means the judge is measuring something the
	// pair does not differ on.
	NullSplit map[string][2]int `json:"nullSplit"`
	// Withheld says why no aggregate score is published.
	Withheld string `json:"withheld"`
}

// summarise reads the verdicts without deciding anything the controls do not
// support.
func summarise(vs []Verdict) JudgeSummary {
	s := JudgeSummary{
		Preference:        map[string]map[string]int{},
		PositionAgreement: map[string][2]int{},
		NullSplit:         map[string][2]int{},
		Withheld: "No preference score is published, and the controls are why rather than " +
			"caution. Over 8 pairs, order agreement sits at or near chance on every lens " +
			"(4/7, 5/8, 4/8), and between two runs on the SAME documents one lens reversed " +
			"direction. A proportion over 8 has a 95% interval near 40 points wide, so these " +
			"numbers cannot separate a lens from a coin. What is published is what the judges " +
			"pointed at: a quoted passage is checkable in a way a preference count is not.",
	}

	type key struct{ lens, model, audience, kind string }
	byPair := map[key][]Verdict{}
	for _, v := range vs {
		if v.Err != "" {
			continue
		}
		byPair[key{v.Lens, v.Model, v.Audience, v.Kind}] = append(byPair[key{v.Lens, v.Model, v.Audience, v.Kind}], v)
		if v.Kind == "real" && v.Won != "" {
			if s.Preference[v.Lens] == nil {
				s.Preference[v.Lens] = map[string]int{}
			}
			s.Preference[v.Lens][v.Won]++
		}
		if v.Kind == "null" {
			n := s.NullSplit[v.Lens]
			switch v.Chose {
			case "A":
				n[0]++
			case "B":
				n[1]++
			}
			s.NullSplit[v.Lens] = n
		}
	}

	// Position agreement: for each pair judged both ways, did the two orders
	// pick the same document?
	for k, group := range byPair {
		if k.kind != "real" || len(group) != 2 {
			continue
		}
		a := s.PositionAgreement[k.lens]
		a[1]++
		if group[0].Won != "" && group[0].Won == group[1].Won {
			a[0]++
		}
		s.PositionAgreement[k.lens] = a
	}
	return s
}

// sortVerdicts gives the dataset a stable order, so a re-run diffs cleanly.
func sortVerdicts(vs []Verdict) {
	sort.Slice(vs, func(i, j int) bool {
		a, b := vs[i], vs[j]
		if a.Lens != b.Lens {
			return a.Lens < b.Lens
		}
		if a.Audience != b.Audience {
			return a.Audience < b.Audience
		}
		if a.Model != b.Model {
			return a.Model < b.Model
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return !a.Swapped && b.Swapped
	})
}

// judgeProvider builds the provider the panel runs on.
func judgeProvider(providerID, model string) (aiprovider.LLMProvider, error) {
	return aiprovider.NewProvider(aiprovider.ProviderID(providerID), aiprovider.Config{
		APIKey: os.Getenv("ANTHROPIC_API_KEY"),
		Model:  model,
		// Greedy. The differences being judged are small and sampling noise
		// across seventy judgements would be read as one of them.
		Temperature: new(float64),
	})
}

// judgeConcurrency is how many judgements run at once. Each is one short call
// over two documents, so this is bounded by the provider rather than by the
// machine.
const judgeConcurrency = 6

// runPanel judges every pair this run produced, both ways round, on every lens.
//
// The real pairs and the null pairs go through the same code path, so nothing
// about a null pair can differ except what is inside it.
func runPanel(ctx context.Context, docs []Doc, tasks map[string]string, model string) ([]Verdict, error) {
	llm, err := judgeProvider("claude-code", model)
	if err != nil {
		return nil, err
	}
	defer llm.Close()

	var pairs []Pair
	for i := range docs {
		d := &docs[i]
		task := tasks[d.Audience]
		if d.Bare.Text != "" && d.Governed.Text != "" {
			pairs = append(pairs, Pair{
				Kind: "real", Model: d.Model, Audience: d.Audience, Task: task,
				A: d.Bare.Text, B: d.Governed.Text, AIs: "bare", BIs: "governed",
			})
		}
		if d.Bare.Text != "" && d.BareAgain.Text != "" {
			pairs = append(pairs, Pair{
				Kind: "null", Model: d.Model, Audience: d.Audience, Task: task,
				A: d.Bare.Text, B: d.BareAgain.Text, AIs: "bare", BIs: "bare",
			})
		}
	}
	if len(pairs) == 0 {
		return nil, errors.New("no pairs to judge: every cell is missing a document")
	}

	fmt.Fprintf(os.Stderr, "panel: %d pair(s) x %d lens(es) x 2 orders = %d judgement(s) on %s\n",
		len(pairs), len(lenses), len(pairs)*len(lenses)*2, model)

	// Every judgement is independent of every other, so they run together.
	// Sequentially this was 96 calls end to end, which made the panel too slow
	// to re-run and a panel nobody re-runs is a number that goes stale.
	type job struct {
		lens    Lens
		pair    Pair
		swapped bool
	}
	var jobs []job
	for _, lens := range lenses {
		for _, p := range pairs {
			for _, swapped := range []bool{false, true} {
				jobs = append(jobs, job{lens, p, swapped})
			}
		}
	}

	out := make([]Verdict, len(jobs))
	sem := make(chan struct{}, judgeConcurrency)
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			out[i] = judgePair(ctx, llm, j.lens, j.pair, j.swapped)
		})
	}
	wg.Wait()
	return out, nil
}

// reportPanel prints what the controls support, and nothing more.
func reportPanel(s JudgeSummary) {
	fmt.Fprintln(os.Stderr, "\npanel controls, which decide whether the rest can be read:")
	for _, l := range lenses {
		agree := s.PositionAgreement[l.ID]
		null := s.NullSplit[l.ID]
		fmt.Fprintf(os.Stderr, "  %-10s order-agreement %d/%d   null split %d/%d\n",
			l.ID, agree[0], agree[1], null[0], null[1])
	}
	fmt.Fprintln(os.Stderr, "\npreferences (published as annotations, withheld as a score):")
	for _, l := range lenses {
		p := s.Preference[l.ID]
		fmt.Fprintf(os.Stderr, "  %-10s governed %d, bare %d\n", l.ID, p["governed"], p["bare"])
	}
}
