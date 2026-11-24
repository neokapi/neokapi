package tool

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/blockstore"
)

func TestFeedbackFingerprint(t *testing.T) {
	assert.Equal(t, "ctx", FeedbackFingerprint("ctx", UnitReviewFeedback{}), "no feedback keeps the governing stamp")
	one := UnitReviewFeedback{Rejections: []ReviewRejection{{TargetHash: "h1", Reasons: []string{"tone", "meaning"}, Note: "Too formal."}}}
	a := FeedbackFingerprint("ctx", one)
	assert.NotEqual(t, "ctx", a)

	reordered := UnitReviewFeedback{Rejections: []ReviewRejection{{TargetHash: "h1", Reasons: []string{"meaning", "tone"}, Note: "Too formal.", Text: "Bonjour"}}}
	assert.Equal(t, a, FeedbackFingerprint("ctx", reordered), "reason order and whether the wording is held do not move it")

	restated := UnitReviewFeedback{Rejections: []ReviewRejection{{TargetHash: "h1", Reasons: []string{"grammar"}, Note: "Use tu."}}}
	assert.Equal(t, a, FeedbackFingerprint("ctx", restated),
		"a rejection restated with another note or other reasons judges no new draft, so it asks for none")

	another := UnitReviewFeedback{Rejections: []ReviewRejection{one.Rejections[0], {TargetHash: "h2", Note: "Still too formal."}}}
	assert.NotEqual(t, a, FeedbackFingerprint("ctx", another), "a rejection of another draft moves it")
}

func TestReviewFeedbackForBlockIsKeyedByFile(t *testing.T) {
	f := ReviewFeedback{FeedbackUnitKey("src/a.json", "title"): {Held: true}}
	ctx := blockstore.WithSourceRel(t.Context(), "src/a.json")
	assert.True(t, f.ForBlock(ctx, "title").Held)
	other := blockstore.WithSourceRel(t.Context(), "src/b.json")
	assert.False(t, f.ForBlock(other, "title").Held, "the same key in another file is another unit")
}

func TestAnUnknownReasonHasAPhrase(t *testing.T) {
	assert.Equal(t, "another reason", RejectionReasonPhrase("new_code"), "an unknown code never reaches a prompt bare")
}

// Feedback written into a configuration is refused: only a host's grant is
// taken.
func TestTakeReviewFeedback(t *testing.T) {
	grant := ReviewFeedbackGrant{Units: ReviewFeedback{"a": {Held: true}}}
	cfg := map[string]any{ReviewFeedbackKey: grant, "other": 1}
	got, err := TakeReviewFeedback(cfg)
	require.NoError(t, err)
	assert.True(t, got.Units["a"].Held)
	assert.NotContains(t, cfg, ReviewFeedbackKey, "the grant is taken out of the configuration")

	_, err = TakeReviewFeedback(map[string]any{ReviewFeedbackKey: map[string]any{"a": map[string]any{"held": true}}})
	require.ErrorIs(t, err, ErrReviewFeedbackFromConfig)

	got, err = TakeReviewFeedback(map[string]any{})
	require.NoError(t, err)
	assert.Empty(t, got.Units)
}
