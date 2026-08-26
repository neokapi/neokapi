package tools_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// render draws one side of a diff the way the dashboard does, so a failure
// reads as the picture a reader would have seen rather than as a span list.
func render(spans []tools.Span) string {
	var b strings.Builder
	for _, s := range spans {
		switch s.Op {
		case tools.SpanSame:
			b.WriteString(s.Text)
		case tools.SpanCosmetic:
			b.WriteString("~" + s.Text + "~")
		case tools.SpanAdded:
			b.WriteString("+" + s.Text + "+")
		case tools.SpanRemoved:
			b.WriteString("-" + s.Text + "-")
		}
	}
	return b.String()
}

func TestDiffEdit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		prior, current string
		wantPrior      string
		wantCurrent    string
	}{
		{
			name:        "a trailing period is punctuation appearing",
			prior:       "Get started",
			current:     "Get started.",
			wantPrior:   "Get started",
			wantCurrent: "Get started~.~",
		},
		{
			name:        "a capitalised word reads as the same word written differently",
			prior:       "Get started",
			current:     "Get Started",
			wantPrior:   "Get ~started~",
			wantCurrent: "Get ~Started~",
		},
		{
			name:        "a hyphen inside a word does not split it",
			prior:       "account setup",
			current:     "account set-up",
			wantPrior:   "account ~setup~",
			wantCurrent: "account ~set-up~",
		},
		{
			name:        "a negation is one added word and nothing else",
			prior:       "You must accept the terms",
			current:     "You must not accept the terms",
			wantPrior:   "You must accept the terms",
			wantCurrent: "You must +not+ accept the terms",
		},
		{
			name:        "a swapped word is a removal and an addition",
			prior:       "Click the button",
			current:     "Click the link",
			wantPrior:   "Click the -button-",
			wantCurrent: "Click the +link+",
		},
		{
			name:        "a comma replacing nothing aligns with the space it joined",
			prior:       "You must accept the terms before we activate",
			current:     "You must accept the terms, before we activate",
			wantPrior:   "You must accept the terms~ ~before we activate",
			wantCurrent: "You must accept the terms~, ~before we activate",
		},
		{
			name:        "a number change is a word change",
			prior:       "Cancel within 30 days",
			current:     "Cancel within 3 days",
			wantPrior:   "Cancel within -30- days",
			wantCurrent: "Cancel within +3+ days",
		},
		{
			name:        "the disguise: not able is not notable",
			prior:       "not able",
			current:     "notable",
			wantPrior:   "-not- -able-",
			wantCurrent: "+notable+",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := tools.DiffEdit(tt.prior, tt.current)
			assert.Equal(t, tt.wantPrior, render(d.Prior))
			assert.Equal(t, tt.wantCurrent, render(d.Current))
		})
	}
}

// TestDiffEditReproducesBothSources is what makes the drawing trustworthy: a
// renderer that walked these spans and dropped one would be showing a reader an
// edit that is not the edit that happened.
func TestDiffEditReproducesBothSources(t *testing.T) {
	t.Parallel()

	pairs := [][2]string{
		{"Get started", "Get started."},
		{"You must accept the terms", "You must not accept the terms"},
		{"Click the button below", "When you're ready, click the button below"},
		{"2019-2020", "2019—2020"},
		{"", "Something appeared"},
		{"Everything vanished", ""},
	}

	for _, p := range pairs {
		d := tools.DiffEdit(p[0], p[1])
		var prior, current strings.Builder
		for _, s := range d.Prior {
			prior.WriteString(s.Text)
		}
		for _, s := range d.Current {
			current.WriteString(s.Text)
		}
		assert.Equal(t, p[0], prior.String(), "the prior side must render the prior source")
		assert.Equal(t, p[1], current.String(), "the current side must render the current source")
	}
}

// TestDiffAgreesWithTheVerdict pins the property the page depends on: the
// highlighting a reader sees IS the verdict beside it, not a second opinion
// that happens to look similar.
func TestDiffAgreesWithTheVerdict(t *testing.T) {
	t.Parallel()

	pairs := [][2]string{
		{"Get started", "Get started"},
		{"Get started", "Get started."},
		{"Get started", "Get Started"},
		{"account setup", "account set-up"},
		{"Don't stop", "Don’t stop"},
		{"You must accept the terms", "You must not accept the terms"},
		{"Click the button", "Click the link"},
		{"not able", "notable"},
		{"Click the button below", "Below, click the button"},
		{"Finish signing up", "Choose a plan"},
	}

	for _, p := range pairs {
		d := tools.DiffEdit(p[0], p[1])
		moved := false
		for _, s := range append(append([]tools.Span{}, d.Prior...), d.Current...) {
			if s.Op == tools.SpanAdded || s.Op == tools.SpanRemoved {
				moved = true
			}
		}
		require.Equal(t, tools.ClassifyEdit(p[0], p[1]), d.Kind)
		assert.Equal(t, d.Kind == tools.EditSubstantive, moved,
			"a word marked as moved is exactly what makes an edit substantive: %q → %q", p[0], p[1])
	}
}

// TestContainsWords is the check a consistency measurement depends on, and the
// reason it cannot be strings.Contains.
func TestContainsWords(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		haystack, needle string
		want             bool
	}{
		// The case that produced this function: the approved Norwegian for a
		// shopping basket is kurven, the word a model reaches for instead is
		// handlekurven, and strings.Contains says the drift is a match.
		{"the drift is not the word", "Legg dette i handlekurven", "kurven", false},
		{"the word is the word", "Legg denne i kurven", "kurven", true},
		{"inflected into the sentence", "Åpne innstillingene for arbeidsområdet", "arbeidsområdet", true},

		{"multi-word, contiguous", "Du kan si opp abonnementet", "si opp", true},
		{"multi-word, not contiguous", "Du kan si det opp", "si opp", false},

		{"case folds", "Logg inn for å fortsette", "logg inn", true},
		{"punctuation between is a separator", "Logg inn, for å fortsette", "logg inn", true},

		{"empty needle matches nothing", "anything at all", "", false},
		{"absent", "Velg en plan", "abonnement", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tools.ContainsWords(tt.haystack, tt.needle))
		})
	}
}
