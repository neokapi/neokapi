package check

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func englishHits(text, term string) []string {
	p := PrepareText(text)
	var out []string
	for _, h := range FindEnglishInflectionsIn(p, term, false) {
		out = append(out, text[h[0]:h[1]])
	}
	return out
}

// TestEnglishInflections_ReachInflection covers the under-demand the research
// measured: a rule on "alert" never fired on the navigation label "Alerts".
func TestEnglishInflections_ReachInflection(t *testing.T) {
	cases := []struct {
		term, text string
		want       []string
	}{
		{"alert", "Alerts", []string{"Alerts"}},
		{"alert", "Two alerts, one alert.", []string{"alerts", "alert"}},
		{"flow", "Flows run each step.", []string{"Flows"}},
		{"translate", "It translates what was translated.", []string{"translates", "translated"}},
		{"extract", "extracts, extracted and extracting", []string{"extracts", "extracted", "extracting"}},
		{"use", "It uses what was used.", []string{"uses", "used"}},
		{"box", "Two boxes", []string{"boxes"}},
		{"content memory", "The content  memory holds it.", []string{"content  memory"}},
		{"alert", "The alert's source", []string{"alert"}},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, englishHits(tc.text, tc.term), "%q in %q", tc.term, tc.text)
	}
}

// TestEnglishInflections_StopBeforeDerivation covers the over-demand the
// research measured: substring matching and stemming read sibling concepts and
// compounds as uses of the shorter term.
func TestEnglishInflections_StopBeforeDerivation(t *testing.T) {
	cases := []struct{ term, text string }{
		{"flow", "Define a workflow."},
		{"translate", "Run pseudo-translate first."},
		{"translate", "Mark it do-not-translate."},
		{"translate", "The translation is translatable."},
		{"translate", "Stop translating."},
		{"extract", "Run the extraction."},
		{"set", "Open the settings."},
		{"set", "A setting."},
		{"local", "Pick a locale."},
		{"use", "Every user is unused."},
		{"engine", "Engineering owns it."},
		{"redact", "Review the redaction."},
		{"memory", "Two content memories."},
	}
	for _, tc := range cases {
		assert.Empty(t, englishHits(tc.text, tc.term), "%q must not be found in %q", tc.term, tc.text)
	}
}

func TestEnglishInflections_Spellings(t *testing.T) {
	assert.Equal(t, []string{"alert", "alerts", "alertes", "alerted", "alerting"}, EnglishInflections("alert"))
	assert.Equal(t, []string{"use", "uses", "usees", "useed", "useing", "used"}, EnglishInflections(" use "))
	assert.Nil(t, EnglishInflections("  "))
}

func TestEnglishInflections_Cased(t *testing.T) {
	p := PrepareText("Compass and compasses")
	assert.Len(t, FindEnglishInflectionsIn(p, "Compass", true), 1)
	assert.Len(t, FindEnglishInflectionsIn(p, "Compass", false), 2)
}
