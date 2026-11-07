package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// TestVoiceEditOf_ListsFieldsAndConstraintsByID: a change lists each field
// that moved, constraints under their id, and leaves the version the store
// increments out.
func TestVoiceEditOf_ListsFieldsAndConstraintsByID(t *testing.T) {
	before := &coreprofile.VoiceProfile{
		ID: "docs", Name: "Docs", Version: 3,
		Tone: coreprofile.ToneProfile{Formality: "formal"},
		Constraints: []coreprofile.Constraint{
			{ID: "c1", Kind: "forbid", Statement: "No exclamation marks."},
			{ID: "c2", Kind: "forbid", Statement: "No emoji."},
		},
	}
	after := &coreprofile.VoiceProfile{
		ID: "docs", Name: "Docs", Version: 4,
		Tone: coreprofile.ToneProfile{Formality: "casual"},
		Constraints: []coreprofile.Constraint{
			{ID: "c1", Kind: "forbid", Statement: "Never use exclamation marks."},
		},
	}

	edit, err := voiceEditOf(before, after)
	require.NoError(t, err)
	assert.Equal(t, "docs", edit.Profile)
	assert.NotEmpty(t, edit.Previous)
	assert.NotEmpty(t, edit.Wrote)

	fields := map[string]contextop.VoiceChange{}
	for _, c := range edit.Changes {
		fields[c.Field] = c
	}
	assert.Equal(t, contextop.VoiceChange{Field: "tone.formality", From: "formal", To: "casual"}, fields["tone.formality"])
	assert.Equal(t, "Never use exclamation marks.", fields["constraints[c1].statement"].To)
	assert.Contains(t, fields, "constraints[c2]", "a removed constraint is one change")
	assert.Equal(t, contextop.VoiceChange{Field: "constraints[c2]", From: "No emoji."}, fields["constraints[c2]"])
	assert.NotContains(t, fields, "version")
	assert.Equal(t, "constraints, tone", voiceEditPhrase(edit))
}

// TestVoiceEditOf_ACreatedProfileKeepsNoPrevious: a write that created the
// profile has nothing to restore.
func TestVoiceEditOf_ACreatedProfileKeepsNoPrevious(t *testing.T) {
	edit, err := voiceEditOf(nil, &coreprofile.VoiceProfile{ID: "new", Name: "New"})
	require.NoError(t, err)
	assert.Empty(t, edit.Previous)
	assert.NotEmpty(t, edit.Changes)
}

// TestVoiceEditOf_ComparesPatternsByPosition: a list of objects with no id is
// compared item by item, so editing one pattern's description is one change,
// and an added item reads as its own words.
func TestVoiceEditOf_ComparesPatternsByPosition(t *testing.T) {
	before := &coreprofile.VoiceProfile{ID: "docs", Name: "Docs"}
	before.Style.ProhibitedPatterns = []coreprofile.Pattern{{Regex: "!", Description: "Exclamation in prose"}}
	after := &coreprofile.VoiceProfile{ID: "docs", Name: "Docs"}
	after.Style.ProhibitedPatterns = []coreprofile.Pattern{
		{Regex: "!", Description: "Exclamation mark in prose"},
		{Regex: "blazing", Description: "Marketing superlative"},
	}

	edit, err := voiceEditOf(before, after)
	require.NoError(t, err)
	assert.Equal(t, []contextop.VoiceChange{
		{Field: "style.prohibited_patterns[1].description", From: "Exclamation in prose", To: "Exclamation mark in prose"},
		{Field: "style.prohibited_patterns[2]", To: "Marketing superlative"},
	}, edit.Changes)
}
