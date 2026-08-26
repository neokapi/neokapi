package main

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/edit"
	corememory "github.com/neokapi/neokapi/core/memory"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/leverage"
)

// The edit ladder is the same block edited eleven ways, run twice through the
// real recycle tool: once under the policy that reads a percentage, once under
// the policy that reads the edit. What each one wrote into the target is the
// output, and it is shown rather than summarised.
//
// The finding is not that a percentage is mistuned. It is that a percentage
// ranks these wrongly, so no floor sorts them correctly: a negation on a real
// sentence scores 95 and a full stop on a button label scores 91, and any floor
// between them fills the meaning inversion and refuses the punctuation.
//
// Measured on every run, because the answer depends on the scorer and the
// scorer can change under us.

// ladderOriginal is a realistic sentence rather than a two-word button. Length
// is the whole point: a period is 8% of "Get started" and 1.7% of a sentence,
// so a short fixture would answer a question nobody has.
const ladderOriginal = "Click the button below when you're ready to continue with your account setup"

const ladderTarget = "Klikk på knappen nedenfor når du er klar til å fortsette med kontooppsettet"

var ladderEdits = []struct {
	label string
	// kind is what a reader would call this edit. A test asserts it against the
	// classifier: a hand label that drifts from the verdict is how a table like
	// this starts teaching the opposite of what it measures.
	kind string
	text string
	// harm says what filling the approved answer here would put in front of a
	// reader. Empty when filling is right.
	harm string
}{
	{label: "unchanged", kind: "none", text: ladderOriginal},
	{label: "a trailing period", kind: "cosmetic", text: ladderOriginal + "."},
	{label: "a comma added", kind: "cosmetic", text: "Click the button below, when you're ready to continue with your account setup"},
	{label: "a word capitalised", kind: "cosmetic", text: "Click the Button below when you're ready to continue with your account setup"},
	{label: "a hyphen added", kind: "cosmetic", text: "Click the button below when you're ready to continue with your account set-up"},
	{label: "a curly apostrophe", kind: "cosmetic", text: "Click the button below when you’re ready to continue with your account setup"},
	{
		label: "a possessive added", kind: "substantive",
		text: "Click the button below when you're ready to continue with your account's setup",
		harm: "the setup belongs to the account now, and the Norwegian compound says it does not",
	},
	{
		label: "a negation added", kind: "substantive",
		text: "Click the button below when you're not ready to continue with your account setup",
		harm: "the Norwegian tells a reader to click when they ARE ready, which is the opposite instruction",
	},
	{
		label: "one word swapped", kind: "substantive",
		text: "Click the link below when you're ready to continue with your account setup",
		harm: "the Norwegian still says knappen, the button, and there is no button on the page",
	},
	{
		label: "the clause reordered", kind: "substantive",
		text: "When you're ready to continue with your account setup, click the button below",
		harm: "same words, different sentence, and Norwegian word order does not follow English",
	},
	{
		label: "rewritten", kind: "substantive",
		text: "Finish signing up by choosing a plan",
		harm: "unrelated content",
	},
}

// LadderRung is one edit: what changed, what the matcher scored it, and what
// each policy wrote into the target file.
type LadderRung struct {
	Edit string `json:"edit"`
	Kind string `json:"kind"`
	Text string `json:"text"`
	// Diff is the classifier's own view of this edit, so the highlighting a
	// reader sees is the comparison the verdict was reached on.
	Diff edit.Diff `json:"diff"`
	// Score and Match are what the matcher says, which is what the old policy
	// read and nothing now reads.
	Score int    `json:"score"`
	Match string `json:"match,omitempty"`
	// Classified is what the classifier says. It does not soften with length,
	// so it ranks these the way a translator would and a percentage does not.
	Classified string `json:"classified"`
	SafeToFill bool   `json:"safeToFill"`
	// ByScore and ByKind are the target the REAL recycle tool wrote under each
	// policy: a 95% floor reading the score alone, and the shipped gate reading
	// the edit. Empty means the tool left the block for a translator.
	ByScore string `json:"byScore"`
	ByKind  string `json:"byKind"`
	// Harm says what a wrong fill puts in front of a reader, for the rungs
	// where the two policies disagree.
	Harm string `json:"harm,omitempty"`
	// Diverges marks a rung the two policies answer differently. These are the
	// whole finding; the rest are agreement.
	Diverges bool `json:"diverges,omitempty"`
}

// EditLadder is the whole measurement plus what it implies.
type EditLadder struct {
	Original string       `json:"original"`
	Target   string       `json:"target"`
	Rungs    []LadderRung `json:"rungs"`
	// FillFloor is the percentage the score-only policy was run at, so a reader
	// can see which number the left-hand column is obeying.
	FillFloor int `json:"fillFloor"`
	// WrongFills counts targets the score-only policy wrote that are not
	// translations of the source beside them. This is the number that says a
	// percentage is the wrong measure: it is not a tuning problem, it is a
	// ranking one, and no floor fixes a ranking.
	WrongFills int `json:"wrongFills"`
	// MissedFills counts the mirror: harmless edits a percentage refuses
	// because they are short, and the classifier fills.
	MissedFills int `json:"missedFills"`
	// Agreements counts rungs both policies answer the same way.
	Agreements int `json:"agreements"`
}

// buildEditLadder scores every edit against a corpus holding the original, and
// runs the real tool over it under both policies.
func buildEditLadder(ctx context.Context) (*EditLadder, error) {
	tm := memory.NewInMemoryStore()
	if err := tm.Add(ctx, memory.Entry{
		ID:          "original",
		HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: ladderOriginal}}},
			"nb": {{Text: &model.TextRun{Text: ladderTarget}}},
		},
	}); err != nil {
		return nil, fmt.Errorf("seed edit ladder: %w", err)
	}

	out := &EditLadder{Original: ladderOriginal, Target: ladderTarget, FillFloor: ladderFillFloor}
	for _, e := range ladderEdits {
		// MinScore floors at almost nothing so the ladder reports what the
		// scorer says rather than what a threshold already decided.
		matches, err := tm.Lookup(ctx, blockOf(e.text), "en", "nb", memory.LookupOptions{
			MinScore: 0.01, MaxResults: 1,
		})
		if err != nil {
			return nil, fmt.Errorf("score %q: %w", e.label, err)
		}

		kind := edit.Classify(ladderOriginal, e.text)
		rung := LadderRung{
			Edit:       e.label,
			Kind:       e.kind,
			Text:       e.text,
			Diff:       edit.Compare(ladderOriginal, e.text),
			Classified: string(kind),
			SafeToFill: kind.SafeToFill(),
		}
		if len(matches) > 0 {
			rung.Score = int(matches[0].Score * 100)
			rung.Match = string(matches[0].MatchType)
		}

		rung.ByScore, err = fillUnder(ctx, tm, e.text, scoreOnly)
		if err != nil {
			return nil, err
		}
		rung.ByKind, err = fillUnder(ctx, tm, e.text, readsTheEdit)
		if err != nil {
			return nil, err
		}

		switch {
		case rung.ByScore == rung.ByKind:
			out.Agreements++
		case rung.ByScore != "" && !rung.SafeToFill:
			rung.Diverges = true
			rung.Harm = e.harm
			out.WrongFills++
		default:
			rung.Diverges = true
			out.MissedFills++
		}
		out.Rungs = append(out.Rungs, rung)
	}
	return out, nil
}

// ladderFillFloor is the percentage the score-only policy is run at: the
// shipped default, so the comparison is against what a recipe actually does
// rather than against a floor chosen to lose.
const ladderFillFloor = 95

// fillPolicy is how a run of the recycle tool decides to write a target.
type fillPolicy int

const (
	// scoreOnly is the policy as it was before the classifier: a percentage and
	// a floor. Reached by handing the tool a provider that classifies nothing,
	// which is also what an older provider does, so this is a live code path
	// rather than a reconstruction.
	scoreOnly fillPolicy = iota
	// readsTheEdit is the shipped gate.
	readsTheEdit
)

// fillUnder runs the real recycle tool over a block and returns the target it
// wrote, or "" when it left the block for a translator.
func fillUnder(ctx context.Context, tm memory.ContentMemory, source string, policy fillPolicy) (string, error) {
	cfg := &tools.MemoryLeverageConfig{}
	cfg.Reset()
	cfg.SourceLocale = "en"
	cfg.TargetLocale = "nb"
	cfg.FillTargetThreshold = ladderFillFloor
	cfg.Memory = leverage.NewProvider(tm)
	if policy == scoreOnly {
		cfg.Memory = unclassified{leverage.NewProvider(tm)}
	}

	tl := tools.NewMemoryLeverageTool(cfg) //nolint:contextcheck // ctx is passed to Process below and travels inside the VariantView from there
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: blockOf(source)}
	close(in)
	if err := tl.Process(ctx, in, out); err != nil {
		return "", fmt.Errorf("recycle %q: %w", source, err)
	}
	close(out)

	part := <-out
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return "", fmt.Errorf("recycle %q: not a block", source)
	}
	return block.TargetText("nb"), nil
}

// unclassified is a provider that answers with a score and no verdict, which is
// what every provider did before the classifier and what an out-of-tree one
// still does. The tool falls back to the floor, so this run IS the old policy
// rather than a description of it.
type unclassified struct{ *leverage.Provider }

func (u unclassified) Lookup(ctx context.Context, req corememory.Request) (corememory.Match, bool) {
	m, ok := u.Provider.Lookup(ctx, req)
	m.Edit = ""
	return m, ok
}

func blockOf(text string) *model.Block {
	return &model.Block{
		ID:           "b",
		Translatable: true,
		Source:       []model.Run{{Text: &model.TextRun{Text: text}}},
	}
}
