package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// A parked locale has its drafts in the workspace home and no file on disk.
// kapi apply establishes such a draft with a decide operation on the source
// document and the locale's edition, and the decision counts where the gate
// reads it, as the review queue's own approval does.
func TestApply_DecideEstablishesAParkedDraft(t *testing.T) {
	a, cmd, recipe, dir := parkedReviewProject(t)
	out, _ := parkedReviewPass(t, a, cmd, recipe)
	require.False(t, parkedLocaleResult(t, out, "nl").Shippable, "nl parks on the first pass")
	keys := parkedQueueKeys(t, a, recipe, "nl")
	require.Len(t, keys, 4)

	applyCmd := NewEnvCommand(t.Context(), "apply")
	AddProjectFlag(applyCmd)
	require.NoError(t, applyCmd.Flags().Set("project", recipe))
	var ops []map[string]any
	for _, key := range keys[:2] {
		ops = append(ops, map[string]any{"op": "decide", "outcome": "establish", "if_match": "*",
			"at": map[string]any{"doc": "src/en.json", "block": key, "edition": "nl"}})
	}
	res, err := applyJSON(t, a, applyCmd, changeSetOf(t, ops...), ApplyOptions{})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	for _, op := range res.Ops {
		assert.Equal(t, change.OpApplied, op.Status, "%+v", op.Error)
	}
	assert.Equal(t, 50, parkedCoverage(t, a, cmd, recipe, dir, "nl").Pct["established"],
		"the decisions count where the gate reads them")
	assert.Len(t, parkedQueueKeys(t, a, recipe, "nl"), 2, "the decided drafts leave the queue")
}

// A decision on a parked draft names the revision the workspace home holds
// for it: a decision naming another (the edition read as absent) is refused
// stale with the draft's text, and once the edition's file is written the file
// is its home, so a decision naming the kept draft is refused stale with the
// text there. The decision never lands on wording the reviewer did not see.
func TestApply_DecideOnAParkedDraftNamesItsRevision(t *testing.T) {
	a, cmd, recipe, dir := parkedReviewProject(t)
	parkedReviewPass(t, a, cmd, recipe)
	keys := parkedQueueKeys(t, a, recipe, "nl")
	require.Len(t, keys, 4)

	svc, err := a.ChangeService(t.Context(), ChangeServiceOptions{Project: recipe})
	require.NoError(t, err)
	page, err := svc.Read(t.Context(), change.ReadRequest{Doc: "src/en.json", Editions: []model.EditionKey{{Locale: "nl"}}})
	require.NoError(t, err)
	kept := map[string]change.EditionRead{}
	for _, b := range page.Blocks {
		kept[b.Ref.Block] = b.Editions["nl"]
	}

	applyCmd := NewEnvCommand(t.Context(), "apply")
	AddProjectFlag(applyCmd)
	require.NoError(t, applyCmd.Flags().Set("project", recipe))
	decide := func(key, ifMatch string) map[string]any {
		return map[string]any{"op": "decide", "outcome": "establish", "if_match": ifMatch,
			"at": map[string]any{"doc": "src/en.json", "block": key, "edition": "nl"}}
	}

	res, err := applyJSON(t, a, applyCmd, changeSetOf(t, decide(keys[0], model.AbsentRevision)), ApplyOptions{})
	require.Error(t, err, "the parked draft is not absent")
	require.Equal(t, change.SetRefused, res.Status, "%+v", res.Ops)
	require.NotNil(t, res.Ops[0].Current)
	assert.Equal(t, kept[keys[0]].Text, res.Ops[0].Current.Text, "the refusal carries the draft")

	res, err = applyJSON(t, a, applyCmd, changeSetOf(t, decide(keys[0], kept[keys[0]].Rev)), ApplyOptions{})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	// Someone writes the Dutch file meanwhile.
	nl := filepath.Join(dir, "site", "locales", "nl.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(nl), 0o755))
	require.NoError(t, os.WriteFile(nl, []byte(`{"`+keys[1]+`": "Iets anders"}`+"\n"), 0o644))
	res, err = applyJSON(t, a, applyCmd, changeSetOf(t, decide(keys[1], kept[keys[1]].Rev)), ApplyOptions{})
	require.Error(t, err, "a refused change set exits non-zero")
	require.Equal(t, change.SetRefused, res.Status, "%+v", res.Ops)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeStale, res.Ops[0].Error.Code)
	require.NotNil(t, res.Ops[0].Current)
	assert.Equal(t, "Iets anders", res.Ops[0].Current.Text)
}
