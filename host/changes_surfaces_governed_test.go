package host

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/projector"
)

// The command-line surfaces build the change service with the host's hooks
// plugged in: an edit that introduces a failing term rule is refused at the
// commit check, and one that lands is recorded as a content.edit operation.

// guideParagraph reads the paragraph of the commit fixture's guide through
// the project's change service: its reference and revision.
func guideParagraph(t *testing.T, f commitFixture) change.BlockRead {
	t.Helper()
	svc, err := f.app.ChangeService(t.Context(), ChangeServiceOptions{Project: f.recipe, Origin: "test"})
	require.NoError(t, err)
	page, err := svc.Read(t.Context(), change.ReadRequest{Doc: "docs/guide.md"})
	require.NoError(t, err)
	return blockWith(t, page, "We use the widget every day.")
}

// editPayload decodes the content.edit operation op.
func editPayload(t *testing.T, op []byte) projector.Edit {
	t.Helper()
	var e projector.Edit
	require.NoError(t, json.Unmarshal(op, &e))
	return e
}

func TestApply_RunsTheCommitCheckAndRecordsTheEdit(t *testing.T) {
	f := newCommitFixture(t)
	const doc = "docs/guide.md"
	original := readFile(t, f.recipe, doc)
	p := guideParagraph(t, f)
	setContent := func(text string) string {
		return changeSetOf(t, map[string]any{"op": "set_content", "at": map[string]any{"doc": doc, "block": p.Ref.Block}, "if_match": p.Rev, "text": text})
	}

	cmd := f.command(t)
	res, err := applyJSON(t, f.app, cmd, setContent("We utilize the widget every day."), ApplyOptions{})
	assert.Equal(t, ExitGate, ExitCode(cmd, err))
	require.Equal(t, change.SetRefused, res.Status, "%+v", res.Ops)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeGateFailed, res.Ops[0].Error.Code)
	assert.Contains(t, findingRules(res.Ops[0].Findings), "terms.vocabulary")
	assert.Nil(t, res.Record)
	assert.Equal(t, original, readFile(t, f.recipe, doc), "a refused edit writes nothing")
	assert.Empty(t, editOps(t, f.app, f.root), "a refused edit records nothing")

	cmd = f.command(t)
	res, err = applyJSON(t, f.app, cmd, setContent("We use the widget each day."), ApplyOptions{})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Equal(t, "# Guide\n\nWe use the widget each day.\n", readFile(t, f.recipe, doc))
	require.NotNil(t, res.Record, "kapi apply records what it landed")
	ops := editOps(t, f.app, f.root)
	require.Len(t, ops, 1)
	assert.Equal(t, projector.KindEdit, ops[0].Kind)
	assert.Equal(t, *res.Record, ops[0].ID)
	e := editPayload(t, ops[0].Payload)
	assert.Equal(t, change.ActorPerson, e.Actor.Kind)
	assert.Equal(t, "apply", e.Origin.By)
	assert.Equal(t, "applied with `kapi apply`", e.Note)
	assert.NotEmpty(t, e.Fingerprint, "the record names the governance the edit was checked under")
}

func TestSed_RunsTheCommitCheckAndRecordsTheEdit(t *testing.T) {
	f := newCommitFixture(t)
	inProject(t, f.root)
	guide := filepath.Join(f.root, "docs", "guide.md")
	original := fileText(t, guide)
	sed := func(script string) (string, error) {
		t.Helper()
		prog, err := ParseSedProgram([]string{script})
		require.NoError(t, err)
		return captureStderr(t, func() error {
			return f.app.RunSed(t.Context(), sedCommand(), []string{"docs/guide.md"}, prog, SedOptions{InPlace: true})
		})
	}

	stderr, err := sed("s/use/utilize/")
	require.Error(t, err, "a refused edit is a ksed error")
	assert.Equal(t, ExitUsage, ExitCode(nil, err))
	assert.Contains(t, stderr, string(change.CodeGateFailed))
	assert.Equal(t, original, fileText(t, guide), "a refused edit writes nothing")
	assert.Empty(t, editOps(t, f.app, f.root), "a refused edit records nothing")

	stderr, err = sed("s/every day/each day/")
	require.NoError(t, err, stderr)
	assert.Equal(t, "# Guide\n\nWe use the widget each day.\n", fileText(t, guide))
	ops := editOps(t, f.app, f.root)
	require.Len(t, ops, 1, "ksed records what it landed")
	assert.Equal(t, projector.KindEdit, ops[0].Kind)
	e := editPayload(t, ops[0].Payload)
	assert.Equal(t, change.ActorPerson, e.Actor.Kind)
	assert.Equal(t, "ksed", e.Origin.By)
	assert.NotEmpty(t, e.Fingerprint)
	_, statErr := os.Stat(filepath.Join(f.root, ".kapi", "work", "locks"))
	assert.NoError(t, statErr, "ksed -i locks the file where kapi apply does")
}

// A project's recorder is opened before a commit takes its first lock, after
// the lock directory is prepared, so a recorder that cannot be opened stops
// the commit before anything is written; a record opens it too.
func TestLazyRecorder_OpensBeforeTheFirstLock(t *testing.T) {
	var order []string
	r := &lazyRecorder{open: sync.OnceValues(func() (change.Recorder, error) {
		order = append(order, "open")
		return nil, errors.New("no log")
	})}
	err := r.before(func() error { order = append(order, "prepare"); return nil })()
	require.EqualError(t, err, "no log")
	assert.Equal(t, []string{"prepare", "open"}, order)
	_, err = r.Record(context.Background(), change.Record{})
	require.EqualError(t, err, "no log")
	assert.Equal(t, []string{"prepare", "open"}, order, "the recorder is opened once")

	var none *lazyRecorder
	assert.Nil(t, none.before(nil), "outside a project the home prepares nothing")
}

func TestMCPApplyEdits_RunsTheCommitCheckAndRecordsTheEdit(t *testing.T) {
	f := newCommitFixture(t)
	f.app.InitRegistries()
	const doc = "docs/guide.md"
	original := readFile(t, f.recipe, doc)
	session := editSession(t, f.app, "governed-agent")

	var page mcpPage
	isErr, body := callEditTool(t, session, "read_blocks", map[string]any{"doc": doc, "project": f.recipe}, &page)
	require.False(t, isErr, body)
	ref, rev, _ := page.blockWith(t, "every day")
	setContent := func(text string) map[string]any {
		return map[string]any{"project": f.recipe, "ops": []any{map[string]any{"op": "set_content", "at": ref, "if_match": rev, "text": text}}}
	}

	var res change.Result
	isErr, body = callEditTool(t, session, "apply_edits", setContent("We utilize the widget every day."), &res)
	assert.True(t, isErr, "a refused change set is an error result")
	require.Equal(t, change.SetRefused, res.Status, body)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeGateFailed, res.Ops[0].Error.Code)
	assert.Contains(t, findingRules(res.Ops[0].Findings), "terms.vocabulary")
	assert.Nil(t, res.Record)
	assert.Equal(t, original, readFile(t, f.recipe, doc), "a refused edit writes nothing")
	assert.Empty(t, editOps(t, f.app, f.root), "a refused edit records nothing")

	res = change.Result{}
	isErr, body = callEditTool(t, session, "apply_edits", setContent("We use the widget each day."), &res)
	require.False(t, isErr, body)
	require.Equal(t, change.SetApplied, res.Status, body)
	assert.Equal(t, "# Guide\n\nWe use the widget each day.\n", readFile(t, f.recipe, doc))
	require.NotNil(t, res.Record, "apply_edits records what it landed")
	ops := editOps(t, f.app, f.root)
	require.Len(t, ops, 1)
	assert.Equal(t, projector.KindEdit, ops[0].Kind)
	assert.Equal(t, *res.Record, ops[0].ID)
	e := editPayload(t, ops[0].Payload)
	assert.Equal(t, change.Actor{Kind: change.ActorAgent, Name: "governed-agent", Session: MCPSessionID()}, e.Actor)
	assert.Equal(t, mcpChangeOrigin, e.Origin.By)
	assert.NotEmpty(t, e.Fingerprint)
}
