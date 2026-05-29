package cli

import (
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeVoice is a voiceTransport that returns canned similarity scores per text,
// so the finding logic is tested without spawning the plugin.
type fakeVoice struct{ scoreFor func(text string) []float64 }

func (f fakeVoice) similarity(text string, _ []string) ([]float64, error) {
	return f.scoreFor(text), nil
}

func block(id, text string) *model.Block {
	return &model.Block{ID: id, Source: []model.Run{{Text: &model.TextRun{Text: text}}}}
}

func TestVoiceSimilarityFindings(t *testing.T) {
	blocks := []*model.Block{block("a", "On voice."), block("b", "Off voice.")}
	refs := []string{"ref1", "ref2"}
	ft := fakeVoice{scoreFor: func(text string) []float64 {
		if text == "On voice." {
			return []float64{0.95, 0.60} // best 0.95 ≥ 0.80 → no finding
		}
		return []float64{0.50, 0.40} // best 0.50 < 0.80 → flagged
	}}

	f, err := voiceSimilarityFindings(blocks, refs, ft, 0.80)
	require.NoError(t, err)
	require.Len(t, f, 1, "only the off-voice block is flagged")
	assert.Equal(t, "voice", f[0].Category)
	assert.Equal(t, check.SeverityMinor, f[0].Severity, "advisory, never a hard gate")
	assert.Equal(t, "Off voice.", f[0].OriginalText)
	assert.Equal(t, "0.500", f[0].Metadata["similarity"])
}

func TestVoiceSimilarityFindings_NoRefsNoop(t *testing.T) {
	f, err := voiceSimilarityFindings([]*model.Block{block("a", "x")}, nil, fakeVoice{}, 0.80)
	require.NoError(t, err)
	assert.Nil(t, f)
}
