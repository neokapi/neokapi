package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// TestContextRuleExamples: a voice rule card lists the words its regex
// matches, and the examples endpoint shows where the rule matches the
// project's source text.
func TestContextRuleExamples(t *testing.T) {
	s, wsID, owner, proj := voiceWriteHarness(t)
	ctx := t.Context()

	profile := ownerProfile(t, s, wsID, owner)
	profile.Style.ProhibitedPatterns = []coreprofile.Pattern{
		{Regex: `\b(powerful|blazing)\b`, Description: "Hype words."},
		{Regex: `\b\w+ly\b`, Description: "Adverbs."},
	}
	require.NoError(t, s.VoiceStore.UpdateProfile(ctx, profile))
	proj.Properties[coreprofile.PropertyProfileID] = profile.ID
	require.NoError(t, s.ContentStore.UpdateProject(ctx, proj))

	var blocks []*model.Block
	for i, text := range []string{"A powerful engine.", "Nothing to see here.", "So blazing fast, and powerful too."} {
		b := model.NewRunsBlock("blk-"+string(rune('a'+i)), []model.Run{{Text: &model.TextRun{Text: text}}})
		b.Translatable = true
		blocks = append(blocks, b)
	}
	require.NoError(t, s.ContentStore.StoreBlocksForItem(ctx, proj.ID, "main", "docs/intro.md", blocks))

	rv := deckReviewer{userID: owner, name: "Owner", perms: platauth.PermAll}
	hype := deckVoiceRuleCard(t, s, wsID, proj, rv, `\b(powerful|blazing)\b`)
	assert.Equal(t, []string{"powerful", "blazing"}, hype.Pattern.Words)
	assert.Empty(t, deckVoiceRuleCard(t, s, wsID, proj, rv, `\b\w+ly\b`).Pattern.Words, "an open shape has no word list")

	get := func(profileID, id string) (int, ruleExamplesResponse) {
		c, rec := voiceRequest(s, http.MethodGet, "", wsID, owner, "ws", "rc", "id", proj.ID)
		c.Request().URL.RawQuery = url.Values{"profile": {profileID}, "id": {id}}.Encode()
		require.NoError(t, s.HandleContextRuleExamples(c))
		var out ruleExamplesResponse
		if rec.Code == http.StatusOK {
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
		}
		return rec.Code, out
	}

	code, out := get(hype.Item.Profile, hype.Item.ID)
	require.Equal(t, http.StatusOK, code)
	require.Len(t, out.Examples, 3, "the limit holds even where more text matches")
	// The walk goes in block id order, which is not the file's order.
	var matches []string
	for _, ex := range out.Examples {
		matches = append(matches, ex.Before+"["+ex.Match+"]"+ex.After)
		assert.Equal(t, "docs/intro.md", ex.Item)
	}
	assert.ElementsMatch(t, []string{
		"A [powerful] engine.",
		"So [blazing] fast, and powerful too.",
		"So blazing fast, and [powerful] too.",
	}, matches)
	assert.Equal(t, 2, out.Scanned, "the walk reads only blocks holding one of the rule's words")
	assert.False(t, out.Complete)

	code, _ = get(hype.Item.Profile, "nope")
	assert.Equal(t, http.StatusNotFound, code)
}
