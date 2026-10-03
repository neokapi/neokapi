package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
)

// op is one operation of a change set as the editor sends it.
type op map[string]any

// setTarget is a set_content of a translation's text, guarded by ifMatch.
func setTarget(item, blockID, locale, ifMatch, text string) op {
	return op{"op": "set_content", "at": map[string]string{"doc": item, "block": blockID, "edition": locale},
		"if_match": ifMatch, "text": text}
}

// decideTarget is a decision on a translation, guarded by ifMatch.
func decideTarget(item, blockID, locale, ifMatch, outcome string) op {
	return op{"op": "decide", "at": map[string]string{"doc": item, "block": blockID, "edition": locale},
		"if_match": ifMatch, "outcome": outcome}
}

// changeSet is the JSON of a change set of ops.
func changeSet(t *testing.T, ops ...op) string {
	t.Helper()
	out, err := json.Marshal(map[string]any{"ops": ops})
	require.NoError(t, err)
	return string(out)
}

// applyChanges sends ops through the ApplyChanges binding and decodes the
// result it answers.
func applyChanges(t *testing.T, app *App, projectID string, ops ...op) change.Result {
	t.Helper()
	out, err := app.ApplyChanges(projectID, changeSet(t, ops...))
	require.NoError(t, err)
	var res change.Result
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	return res
}

// queuedSave is the outbox entry of an offline save of a translation.
func queuedSave(t *testing.T, text string) changeSetOp {
	t.Helper()
	return changeSetOp{ProjectID: "p1", Stream: editorStream,
		Set: json.RawMessage(changeSet(t, setTarget("hello.txt", "b1", "fr", "r:00000000000000aa", text)))}
}

// targetRevision is the revision the app serves for a block's translation.
func targetRevision(t *testing.T, app *App, projectID, blockID, locale string) string {
	t.Helper()
	b, err := app.GetBlock(projectID, blockID)
	require.NoError(t, err)
	rev := b.TargetRevisions[locale]
	require.NotEmpty(t, rev, "a served block names each translation's revision")
	return rev
}

// saveTarget saves a translation on the revision the app serves for it.
func saveTarget(t *testing.T, app *App, projectID, item, blockID, locale, text string) {
	t.Helper()
	res := applyChanges(t, app, projectID, setTarget(item, blockID, locale, targetRevision(t, app, projectID, blockID, locale), text))
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
}

// In local mode the cache is the project: it judges every precondition, and a
// translation saved over someone's newer wording is refused with that wording.
func TestApplyChangesInLocalMode(t *testing.T) {
	app, info, item := setupProjectWithFile(t)
	blocks, err := app.GetItemBlocks(info.ID, item)
	require.NoError(t, err)
	require.Len(t, blocks, 3)
	b0, b1 := blocks[0].ID, blocks[1].ID
	assert.Equal(t, "absent", blocks[0].TargetRevisions["fr"], "an untranslated block is served at absent")

	res := applyChanges(t, app, info.ID, setTarget(item, b0, "fr", "absent", "Bonjour le monde"))
	require.Equal(t, change.SetApplied, res.Status)
	after := res.Ops[0].After
	assert.NotEmpty(t, after)
	assert.Equal(t, after, targetRevision(t, app, info.ID, b0, "fr"), "the result names the revision the block is now served at")

	cases := []struct {
		name string
		ops  []op
		code change.Code
	}{
		{"a save over wording that moved", []op{setTarget(item, b0, "fr", "absent", "Salut")}, change.CodeStale},
		{"a decision on wording that moved", []op{decideTarget(item, b0, "fr", "r:0000000000000000", "establish")}, change.CodeStale},
		{"an approval of a block with no translation", []op{decideTarget(item, b1, "fr", "absent", "establish")}, change.CodeUnsupported},
		{"a block the project does not hold", []op{setTarget(item, "no-such-block", "fr", "absent", "x")}, change.CodeNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := applyChanges(t, app, info.ID, tc.ops...)
			assert.Equal(t, change.SetRefused, res.Status)
			require.NotNil(t, res.Ops[0].Error)
			assert.Equal(t, tc.code, res.Ops[0].Error.Code)
			if tc.code == change.CodeStale {
				require.NotNil(t, res.Ops[0].Current, "a stale refusal names the wording that stands")
				assert.Equal(t, "Bonjour le monde", res.Ops[0].Current.Text)
			}
			assert.Equal(t, after, targetRevision(t, app, info.ID, b0, "fr"), "a refusal writes nothing")
		})
	}

	t.Run("a change set that does not decode", func(t *testing.T) {
		out, err := app.ApplyChanges(info.ID, `{"ops":[{"op":"set_content","wat":1}]}`)
		require.NoError(t, err)
		var res change.Result
		require.NoError(t, json.Unmarshal([]byte(out), &res))
		assert.Equal(t, change.SetRefused, res.Status)
		require.NotNil(t, res.Error)
		assert.Equal(t, change.CodeInvalid, res.Error.Code)
	})

	t.Run("one refusal writes nothing of the change set", func(t *testing.T) {
		res := applyChanges(t, app, info.ID,
			setTarget(item, b1, "fr", "absent", "Bienvenue"),
			setTarget(item, b0, "fr", "absent", "Salut"))
		assert.Equal(t, change.SetRefused, res.Status)
		assert.Equal(t, change.OpNotApplied, res.Ops[0].Status)
		assert.Equal(t, "absent", targetRevision(t, app, info.ID, b1, "fr"))
	})

	// Two edits of one translation whose positions overlap are refused, the
	// message naming each by its place in the change set, a decision sent
	// before them counted.
	t.Run("an overlap names operations by their place in the change set", func(t *testing.T) {
		rev := targetRevision(t, app, info.ID, b0, "fr")
		at := map[string]string{"doc": item, "block": b0, "edition": "fr"}
		res := applyChanges(t, app, info.ID,
			decideTarget(item, b0, "fr", rev, "establish"),
			op{"op": "replace_text", "at": at, "if_match": rev, "edits": []map[string]any{{"start": 0, "end": 7, "text": "Salut"}}},
			op{"op": "replace_text", "at": at, "if_match": rev, "edits": []map[string]any{{"start": 3, "end": 10, "text": "x"}}})
		require.Equal(t, change.SetRefused, res.Status)
		require.NotNil(t, res.Ops[2].Error, "%+v", res.Ops)
		assert.Equal(t, change.CodeGuard, res.Ops[2].Error.Code)
		assert.Contains(t, res.Ops[2].Error.Message, "operation 2 names a position in text operation 1")
		assert.Equal(t, rev, targetRevision(t, app, info.ID, b0, "fr"), "a refusal writes nothing")
	})

	// Every if_match names the edition as the change set found it, so a
	// decision beside a save of the same translation names the revision both
	// were made against, not the one the save leaves.
	t.Run("a decision is judged against the revision the change set began on", func(t *testing.T) {
		res := applyChanges(t, app, info.ID,
			setTarget(item, b1, "de", "absent", "Willkommen"),
			decideTarget(item, b1, "de", "absent", "establish"))
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
		assert.Equal(t, "established", targetStatus(t, app, info.ID, item, b1, "de"))

		saved := targetRevision(t, app, info.ID, b1, "de")
		res = applyChanges(t, app, info.ID,
			setTarget(item, b1, "de", saved, "Herzlich willkommen"),
			decideTarget(item, b1, "de", "r:0000000000000000", "withdraw"))
		require.Equal(t, change.SetRefused, res.Status)
		require.NotNil(t, res.Ops[1].Error)
		assert.Equal(t, change.CodeStale, res.Ops[1].Error.Code)
		require.NotNil(t, res.Ops[1].Current)
		assert.Equal(t, saved, res.Ops[1].Current.Rev, "the refusal names the revision the set began on")
		assert.Equal(t, "Willkommen", res.Ops[1].Current.Text)
		assert.Equal(t, saved, targetRevision(t, app, info.ID, b1, "de"), "a refusal writes nothing")
	})
}

// reviewedBlock translates and approves the first block of a fresh project in
// local mode, returning the app, project, item name and block id.
func reviewedBlock(t *testing.T, locale, text string) (*App, *ProjectInfo, string, string) {
	t.Helper()
	app, info, item := setupProjectWithFile(t)
	blocks, err := app.GetItemBlocks(info.ID, item)
	require.NoError(t, err)
	blockID := blocks[0].ID

	saveTarget(t, app, info.ID, item, blockID, locale, text)
	res := applyChanges(t, app, info.ID, decideTarget(item, blockID, locale, targetRevision(t, app, info.ID, blockID, locale), "establish"))
	require.Equal(t, change.SetApplied, res.Status)
	require.Equal(t, "established", targetStatus(t, app, info.ID, item, blockID, locale))
	return app, info, item, blockID
}

func targetStatus(t *testing.T, app *App, projectID, itemName, blockID, locale string) string {
	t.Helper()
	blocks, err := app.GetItemBlocks(projectID, itemName)
	require.NoError(t, err)
	for _, b := range blocks {
		if b.ID == blockID {
			return b.Targets[locale].Status
		}
	}
	t.Fatalf("block %q not found in %q", blockID, itemName)
	return ""
}

// An edit invalidates the approval of the wording it replaced, and a save of
// the same wording leaves the approval standing. Reject and withdraw move the
// translation down the ladder.
func TestApplyChangesMovesTheReviewLadder(t *testing.T) {
	cases := []struct {
		name string
		do   func(t *testing.T, app *App, projectID, item, blockID string)
		want string
	}{
		{"an edit drops an approval to translated", func(t *testing.T, app *App, projectID, item, blockID string) {
			saveTarget(t, app, projectID, item, blockID, "fr", "Salut")
		}, "translated"},
		{"the same wording keeps the approval", func(t *testing.T, app *App, projectID, item, blockID string) {
			saveTarget(t, app, projectID, item, blockID, "fr", "Bonjour")
		}, "established"},
		{"a rejection drops it to draft", func(t *testing.T, app *App, projectID, item, blockID string) {
			res := applyChanges(t, app, projectID, decideTarget(item, blockID, "fr", targetRevision(t, app, projectID, blockID, "fr"), "reject"))
			require.Equal(t, change.SetApplied, res.Status)
		}, "draft"},
		{"a withdrawal drops it to translated", func(t *testing.T, app *App, projectID, item, blockID string) {
			res := applyChanges(t, app, projectID, decideTarget(item, blockID, "fr", targetRevision(t, app, projectID, blockID, "fr"), "withdraw"))
			require.Equal(t, change.SetApplied, res.Status)
		}, "translated"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app, info, item, blockID := reviewedBlock(t, "fr", "Bonjour")
			tc.do(t, app, info.ID, item, blockID)
			assert.Equal(t, tc.want, targetStatus(t, app, info.ID, item, blockID, "fr"))
			assert.Empty(t, targetStatus(t, app, info.ID, item, blockID, "de"), "a decision is per language")
		})
	}
}

// Offline, the change set applies to the cache and queues as it was sent, with
// the revision the editor read: the server judges it on replay. The cache does
// not refuse the edit for a revision of its own.
func TestApplyChangesOfflineQueuesTheChangeSetWithItsPreconditions(t *testing.T) {
	app, info, item := setupProjectWithFile(t)
	q := newTestQueue(t)
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
	}
	app.offlineQueue = q
	blocks, err := app.GetItemBlocks(info.ID, item)
	require.NoError(t, err)
	b0 := blocks[0].ID

	app.mu.Lock()
	app.connState = StateOffline
	app.mu.Unlock()

	// The editor read the translation on the server, at a revision the cache
	// does not hold.
	const read = "r:00000000000000aa"
	res := applyChanges(t, app, info.ID, setTarget(item, b0, "fr", read, "Bonjour le monde"))
	require.Equal(t, change.SetApplied, res.Status)
	blocks, err = app.GetItemBlocks(info.ID, item)
	require.NoError(t, err)
	assert.Equal(t, "Bonjour le monde", blocks[0].Targets["fr"].Text, "the cache shows the edit")

	pending, err := q.PeekPending(10)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, string(opChangeSet), pending[0].Operation)
	var queued changeSetOp
	require.NoError(t, json.Unmarshal([]byte(pending[0].Payload), &queued))
	assert.Equal(t, info.ID, queued.ProjectID)
	assert.Equal(t, editorStream, queued.Stream)
	set, err := change.Decode(bytes.NewReader(queued.Set))
	require.NoError(t, err)
	require.Len(t, set.Ops, 1)
	assert.Equal(t, read, set.Ops[0].IfMatch, "the queued change set keeps the revision the editor read")

	t.Run("a refusal queues nothing", func(t *testing.T) {
		res := applyChanges(t, app, info.ID, decideTarget(item, blocks[1].ID, "fr", "absent", "establish"))
		assert.Equal(t, change.SetRefused, res.Status)
		assert.Equal(t, 1, q.PendingCount())
	})
}

// A queued entry this build cannot replay, such as one of the retired
// per-field kinds, is marked failed, where the failed count shows it, and the
// queue drains past it.
func TestReplayMarksAnEntryItCannotReplayFailed(t *testing.T) {
	calls := 0
	app, _ := newGovTestApp(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	})
	q := newTestQueue(t)
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
	}
	app.offlineQueue = q
	require.NoError(t, q.Enqueue("update_block_target", map[string]string{
		"project_id": "p1", "block_id": "b1", "target_locale": "fr", "text": "Bonjour",
	}))
	require.NoError(t, q.Enqueue(string(opChangeSet), map[string]any{"project_id": 7}))

	app.replayPendingChanges(context.Background())

	assert.Zero(t, calls, "nothing reaches the server")
	assert.Equal(t, 0, q.PendingCount())
	assert.Equal(t, 2, q.FailedCount())
}
