package main

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
)

// The edit ladder answers the question that decides when fuzzy fill can retire:
// what does the matcher actually score the edits an author makes?
//
// The intuition that fuzzy is a pricing-era mechanism is right about the LOW
// band. It is wrong about the high one. A trailing period on a real sentence is
// not a 78% match — it is a 98% match, and 98% is a block whose approved
// translation needs a punctuation mark, not a re-translation.
//
// So the two thresholds do different jobs and retire on different schedules.
// The fuzzy floor governs a band that is recorded and read by nothing. The fill
// floor governs the band where cosmetic edits land, and dropping it before a
// prior version reaches the prompt would lose a real behaviour and replace it
// with nothing.
//
// Measured on every run, because the answer depends on the scorer and the
// scorer can change under us.

// ladderOriginal is a realistic sentence rather than a two-word button. Length
// is the whole point: a period is 8% of "Get started" and 1.7% of a sentence,
// so a short fixture would answer a question nobody has.
const ladderOriginal = "Click the button below to continue with your account setup"

const ladderTarget = "Klikk på knappen nedenfor for å fortsette med kontooppsettet"

var ladderEdits = []struct {
	label string
	kind  string
	text  string
}{
	{"unchanged", "none", ladderOriginal},
	{"a trailing period", "cosmetic", ladderOriginal + "."},
	{"a comma added", "cosmetic", "Click the button below, to continue with your account setup"},
	{"a word capitalised", "cosmetic", "Click the Button below to continue with your account setup"},
	{"a hyphen added", "cosmetic", "Click the button below to continue with your account set-up"},
	{"a curly apostrophe", "cosmetic", "Click the button below to continue with your account setup’s"},
	{"one word swapped", "wording", "Click the link below to continue with your account setup"},
	{"the clause reordered", "wording", "To continue with your account setup, click the button below"},
	{"rewritten", "wording", "Finish signing up by choosing a plan"},
}

// LadderRung is one edit, what it scores, and what each threshold does with it.
type LadderRung struct {
	Edit  string `json:"edit"`
	Kind  string `json:"kind"`
	Text  string `json:"text"`
	Score int    `json:"score"`
	Match string `json:"match,omitempty"`
	// FilledAt95 and FilledAt100 are what the fill floor does with this edit at
	// each setting — the difference the retirement would make.
	FilledAt95  bool `json:"filledAt95"`
	FilledAt100 bool `json:"filledAt100"`
	// LookedUpAt70 reports whether the retiring fuzzy floor even asks. Below it,
	// nothing is recorded at all.
	LookedUpAt70 bool `json:"lookedUpAt70"`
}

// EditLadder is the whole measurement plus what it implies.
type EditLadder struct {
	Original string       `json:"original"`
	Target   string       `json:"target"`
	Rungs    []LadderRung `json:"rungs"`
	// LostByRetiring counts edits that fill today and would not at 100 — the
	// regression the retirement would ship if it landed before the replacement.
	LostByRetiring int `json:"lostByRetiring"`
	// InertBand counts edits looked up and recorded but never filled and never
	// sent to a model: the band that genuinely is bookkeeping.
	InertBand int `json:"inertBand"`
}

// buildEditLadder scores every edit against a corpus holding the original.
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

	out := &EditLadder{Original: ladderOriginal, Target: ladderTarget}
	for _, e := range ladderEdits {
		block := &model.Block{
			ID:           "b",
			Translatable: true,
			Source:       []model.Run{{Text: &model.TextRun{Text: e.text}}},
		}
		// MinScore floors at almost nothing so the ladder reports what the
		// scorer says rather than what a threshold already decided.
		matches, err := tm.Lookup(ctx, block, "en", "nb", memory.LookupOptions{
			MinScore: 0.01, MaxResults: 1,
		})
		if err != nil {
			return nil, fmt.Errorf("score %q: %w", e.label, err)
		}

		rung := LadderRung{Edit: e.label, Kind: e.kind, Text: e.text}
		if len(matches) > 0 {
			rung.Score = int(matches[0].Score * 100)
			rung.Match = string(matches[0].MatchType)
		}
		rung.LookedUpAt70 = rung.Score >= 70
		rung.FilledAt95 = rung.Score >= 95
		rung.FilledAt100 = rung.Score >= 100

		if rung.FilledAt95 && !rung.FilledAt100 {
			out.LostByRetiring++
		}
		if rung.LookedUpAt70 && !rung.FilledAt95 {
			out.InertBand++
		}
		out.Rungs = append(out.Rungs, rung)
	}
	return out, nil
}
