package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// codeSpan builds the run shape a markdown code span produces: a paired code
// bracketing text the reader marked do-not-translate.
func codeSpan(id, text string, protected bool) []Run {
	return []Run{
		PcOpenR(PcOpenRun{ID: id, Type: "fmt:code"}),
		{Text: &TextRun{Text: text, NoTranslate: protected}},
		PcCloseR(PcCloseRun{ID: id, Type: "fmt:code"}),
	}
}

func TestRestoreNonTranslatablePutsTheCommandBack(t *testing.T) {
	source := append([]Run{TextR("Run ")}, codeSpan("1", "kapi check --ship", true)...)
	source = append(source, TextR(" in CI."))

	// What a provider that ignores <code> hands back.
	translated := append([]Run{TextR("Kjør ")}, codeSpan("1", "kapi sjekk --send", false)...)
	translated = append(translated, TextR(" i CI."))

	got := RestoreNonTranslatable(translated, source)

	assert.Equal(t, "Kjør kapi check --ship i CI.", RunsText(got),
		"the sentence is translated and the command is not")
}

func TestRestoreLeavesOrdinaryCodeAlone(t *testing.T) {
	// A bracket whose content was never protected — emphasis, a link — must
	// keep the provider's translation.
	source := []Run{
		PcOpenR(PcOpenRun{ID: "1", Type: "fmt:bold"}),
		TextR("the check"),
		PcCloseR(PcCloseRun{ID: "1", Type: "fmt:bold"}),
	}
	translated := []Run{
		PcOpenR(PcOpenRun{ID: "1", Type: "fmt:bold"}),
		TextR("sjekken"),
		PcCloseR(PcCloseRun{ID: "1", Type: "fmt:bold"}),
	}

	got := RestoreNonTranslatable(translated, source)
	assert.Equal(t, "sjekken", RunsText(got))
}

// A bracket holding both protected and translatable text is left to the
// ordinary path: replacing all of it would drop the half that was translatable.
func TestRestoreSkipsMixedBrackets(t *testing.T) {
	source := []Run{
		PcOpenR(PcOpenRun{ID: "1", Type: "fmt:code"}),
		{Text: &TextRun{Text: "kapi ", NoTranslate: true}},
		TextR("check"),
		PcCloseR(PcCloseRun{ID: "1", Type: "fmt:code"}),
	}
	translated := []Run{
		PcOpenR(PcOpenRun{ID: "1", Type: "fmt:code"}),
		TextR("kapi sjekk"),
		PcCloseR(PcCloseRun{ID: "1", Type: "fmt:code"}),
	}

	got := RestoreNonTranslatable(translated, source)
	assert.Equal(t, "kapi sjekk", RunsText(got))
}

func TestRestoreIsANoOpWithoutProtectedText(t *testing.T) {
	source := []Run{TextR("Run the check.")}
	translated := []Run{TextR("Kjør sjekken.")}

	got := RestoreNonTranslatable(translated, source)
	require.Len(t, got, 1)
	assert.Equal(t, "Kjør sjekken.", RunsText(got))
}

// The restored run keeps its marking, so a second pass over the same content
// protects it again rather than treating it as ordinary prose.
func TestRestoredRunStaysMarked(t *testing.T) {
	source := codeSpan("1", "kapi up", true)
	translated := codeSpan("1", "kapi opp", false)

	got := RestoreNonTranslatable(translated, source)

	var marked bool
	for _, r := range got {
		if r.Text != nil && r.Text.NoTranslate {
			marked = true
		}
	}
	assert.True(t, marked)
}
