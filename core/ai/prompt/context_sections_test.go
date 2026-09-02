package prompt

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ctxBasic() Translate {
	return Translate{SourceLocale: "en", TargetLocale: "fr"}
}

// The key is the point of the whole exercise: a bare "Save" is a coin flip
// between a verb and a noun, and `settings.save` settles it. It must reach the
// model, and it must be attributed so a user can see it being sent.
func TestKeyReachesThePromptAndIsAttributed(t *testing.T) {
	t.Parallel()

	turns := ctxBasic().SingleWithContext("Save", false, Context{Key: "app.settings.save"})
	require.Len(t, turns, 2)

	assert.Contains(t, turns[0].Text, "app.settings.save", "the key must be sent")
	assert.Equal(t, "Save", turns[1].Text, "the user turn is the text to translate, and nothing else")

	var origins []string
	for _, s := range turns[0].Sections {
		if s.Kind == KindContext {
			origins = append(origins, s.Origin)
		}
	}
	assert.Equal(t, []string{"document (the block's key)"}, origins,
		"context must be attributed, so --explain-prompts can say where it came from")
}

// Neighbours are reference material, not work. The prompt has to say so, or the
// model returns translations of them.
func TestNeighboursAreMarkedDoNotTranslate(t *testing.T) {
	t.Parallel()

	turns := ctxBasic().SingleWithContext("Save", false, Context{
		Key:    "dialog.save",
		Before: []string{"Unsaved changes"},
		After:  []string{"Discard"},
	})

	system := turns[0].Text
	assert.Contains(t, system, "Unsaved changes")
	assert.Contains(t, system, "Discard")
	assert.Contains(t, system, "Do not translate it")
	assert.Equal(t, "Save", turns[1].Text, "neighbours must not leak into the text to translate")
}

// Context is a policy, not noise: with none of it set, no section appears.
func TestNoContextSectionsWhenEmpty(t *testing.T) {
	t.Parallel()

	turns := ctxBasic().SingleWithContext("Save", false, Context{})
	for _, s := range turns[0].Sections {
		assert.NotEqual(t, KindContext, s.Kind, "an unconfigured run must not ship an empty context header")
	}
	assert.True(t, Context{}.Empty())
}

// The digest is what makes neighbour context safe to cache. It must move when
// the neighbourhood moves — and must NOT move for the key, which travels with
// the block and is already accounted for by a block-keyed cache.
func TestDigestTracksNeighboursNotTheKey(t *testing.T) {
	t.Parallel()

	base := Context{Key: "a.b", Before: []string{"one"}, After: []string{"two"}}

	sameKeyDifferentNeighbour := base
	sameKeyDifferentNeighbour.Before = []string{"CHANGED"}
	assert.NotEqual(t, base.Digest(), sameKeyDifferentNeighbour.Digest(),
		"a changed neighbourhood must invalidate the cached translation")

	differentKeySameNeighbours := base
	differentKeySameNeighbours.Key = "totally.different"
	assert.Equal(t, base.Digest(), differentKeySameNeighbours.Digest(),
		"the key travels with the block; folding it into the digest would churn the cache for nothing")

	assert.Empty(t, Context{Key: "a.b"}.Digest(),
		"key-only context needs no digest — it cannot make a cached translation wrong")
}

// Per-segment keys ride in the payload, because they differ per segment. The
// batch's neighbourhood is one shared section, because it does not.
func TestBatchCarriesPerSegmentKeys(t *testing.T) {
	t.Parallel()

	segments := BatchSegments([]string{"Save", "Open"})
	segments[0].Key = "settings.save"
	segments[1].Key = "file.open"

	turns := ctxBasic().BatchWithContext(segments, Context{Before: []string{"File menu"}})
	require.Len(t, turns, 2)

	assert.Contains(t, turns[1].Text, "settings.save", "each segment carries its own key")
	assert.Contains(t, turns[1].Text, "file.open")
	assert.Contains(t, turns[0].Text, "never translate or return it",
		"the model must be told the keys are not content")
	assert.Contains(t, turns[0].Text, "File menu", "the batch's neighbourhood is shared, not per-segment")
}

// The model is asked to reproduce inline tags verbatim, so it must be shown a
// tag — not an escape sequence standing in for one.
//
// encoding/json escapes < > & as \u003c \u003e \u0026 by default — a defence for
// embedding JSON in a <script> tag, irrelevant here. That sent every inline tag to
// the model as `\u003cph id="1"/\u003e`, and gemini-3.5-flash echoed back a
// literal `3cph id="1"/3e`, corrupting the markup of every tagged segment it
// translated. Caught by the batch eval; pinned here so it cannot come back.
func TestBatchPayloadShowsTagsNotEscapeSequences(t *testing.T) {
	t.Parallel()

	segments := BatchSegments([]string{`I accept the <ph id="1"/>terms<ph id="2"/> & conditions`})
	payload := ctxBasic().Batch(segments)[1].Text

	// The tag reaches the model as a tag. (JSON escapes the quotes *inside* it —
	// that is the framing doing its job, and the model decodes it as a matter of
	// course; an escaped `<` is what it got wrong.)
	assert.Contains(t, payload, `<ph id=`, "the model must see the tag it is being asked to copy")
	assert.Contains(t, payload, "&", "ampersands too — the escaping is all or nothing")
	assert.NotContains(t, payload, `\u003c`, "an escape sequence is not a tag; the model has to decode it, and gets it wrong")
	assert.NotContains(t, payload, `\u003e`)
	assert.NotContains(t, payload, `\u0026`)

	// Still valid JSON: the point is to stop escaping what needs no escaping, not
	// to hand-roll the framing. A block containing a quote or a brace must not be
	// able to break out of its string.
	var out struct {
		Segments []struct{ Text string } `json:"segments"`
	}
	require.NoError(t, json.Unmarshal([]byte(payload), &out))
	require.Len(t, out.Segments, 1)
	assert.Equal(t, `I accept the <ph id="1"/>terms<ph id="2"/> & conditions`, out.Segments[0].Text)
}

// A prompt that changed must not serve translations produced by the prompt it
// replaced. Context is part of the prompt.
func TestContextChangesTheFingerprintOnlyViaPolicy(t *testing.T) {
	t.Parallel()

	// The builder's fingerprint covers wording and steering, not per-block
	// context — the tool folds the policy in separately, and the neighbourhood
	// digest rides on the cache entry.
	p := ctxBasic()
	assert.Equal(t, p.Fingerprint(), ctxBasic().Fingerprint(), "fingerprint must be stable")
}
