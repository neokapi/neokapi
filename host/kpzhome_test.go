package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/workhome"
)

// kpzEdit reads the first block of a KPZ document through the change service
// and sets its text, guarded by the revision it read.
func kpzEdit(t *testing.T, svc *change.Service, doc, text string) (*change.Result, change.Op) {
	t.Helper()
	ctx := context.Background()
	page, err := svc.Read(ctx, change.ReadRequest{Doc: doc})
	require.NoError(t, err)
	require.NotEmpty(t, page.Blocks)
	b := page.Blocks[0]
	op := change.Op{Kind: change.KindSetContent, At: b.Ref, IfMatch: b.Rev, Body: &change.SetContent{Text: &text}}
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{op}}, change.Actor{Kind: change.ActorPerson, Name: "editor"})
	require.NoError(t, err)
	return res, op
}

// TestKpzEdit_LandsInTheWorkspaceHomeAndReachesTheMergeAndThePack: kapi apply
// and the MCP edit tools reach a KPZ's documents through the change service.
// An edit lands in the workspace home as a recorded write, a replay is stale,
// the merge writes the edited source, and the pack carries the edited bytes,
// so a fresh cache built from the packed KPZ reads the edit.
func TestKpzEdit_LandsInTheWorkspaceHomeAndReachesTheMergeAndThePack(t *testing.T) {
	a, work := kpzWorkspace(t, "fr")
	dir := filepath.Dir(work)
	ctx := context.Background()
	svc, err := a.ChangeService(ctx, ChangeServiceOptions{Root: dir, Origin: "apply"})
	require.NoError(t, err)

	res, op := kpzEdit(t, svc, "work.kpz!messages.json", "Hello, edited in the workspace")
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	require.Len(t, res.Docs, 1)
	assert.Equal(t, workhome.Name, res.Docs[0].Home)
	assert.Equal(t, "work.kpz!messages.json", res.Docs[0].Doc)
	assert.True(t, res.Docs[0].Written)
	require.NotNil(t, res.Record)

	replay, err := svc.Apply(ctx, change.Set{Ops: []change.Op{op}}, change.Actor{Kind: change.ActorPerson, Name: "editor"})
	require.NoError(t, err)
	require.Equal(t, change.SetRefused, replay.Status)
	assert.Equal(t, change.CodeStale, replay.Ops[0].Error.Code)

	out := filepath.Join(dir, "out")
	require.NoError(t, a.MergeFromKpz(ctx, work, out))
	merged, err := os.ReadFile(filepath.Join(out, "messages.json"))
	require.NoError(t, err)
	assert.Contains(t, string(merged), "Hello, edited in the workspace")

	require.NoError(t, a.PackKpz(ctx, work))
	require.NoError(t, a.unpackKpz(ctx, work))
	svc, err = a.ChangeService(ctx, ChangeServiceOptions{Root: dir, Origin: "apply"})
	require.NoError(t, err)
	page, err := svc.Read(ctx, change.ReadRequest{Doc: "work.kpz!messages.json"})
	require.NoError(t, err)
	require.NotEmpty(t, page.Blocks)
	assert.Equal(t, "Hello, edited in the workspace", page.Blocks[0].Text)
}

// TestKpzEdit_ASkeletonOnlySourceIsNotOpened: a KPZ that carries a source as
// its skeleton alone holds none of its text, and an edit to it is refused as
// unsupported.
func TestKpzEdit_ASkeletonOnlySourceIsNotOpened(t *testing.T) {
	a, work := kpzWorkspace(t, "fr")
	ctx := context.Background()
	require.NoError(t, a.unpackKpz(ctx, work))
	svc, err := a.ChangeService(ctx, ChangeServiceOptions{Root: filepath.Dir(work), Origin: "apply"})
	require.NoError(t, err)
	_, err = svc.Read(ctx, change.ReadRequest{Doc: "work.kpz!messages.json"})
	var cerr *change.Error
	require.ErrorAs(t, err, &cerr)
	assert.Equal(t, change.CodeUnsupported, cerr.Code)
	assert.Contains(t, cerr.Message, "--with-source")
}

// TestKpzEdit_AReferenceToADocumentTheKPZDoesNotCarryIsNotFound: a reference
// into a KPZ names one of its sources; any other is not found, and nothing is
// opened.
func TestKpzEdit_AReferenceToADocumentTheKPZDoesNotCarryIsNotFound(t *testing.T) {
	a, work := kpzWorkspace(t, "fr")
	svc, err := a.ChangeService(context.Background(), ChangeServiceOptions{Root: filepath.Dir(work), Origin: "apply"})
	require.NoError(t, err)
	_, err = svc.Read(context.Background(), change.ReadRequest{Doc: "work.kpz!missing.json"})
	var cerr *change.Error
	require.ErrorAs(t, err, &cerr)
	assert.Equal(t, change.CodeNotFound, cerr.Code)
}

// TestKpzEdit_TheMCPEditToolsReachAKPZDocument: an agent reads a KPZ's source
// through read_blocks and edits it through apply_edits, by the reference and
// revision the read reported; the edit lands in the KPZ's workspace home.
func TestKpzEdit_TheMCPEditToolsReachAKPZDocument(t *testing.T) {
	_, work := kpzWorkspace(t, "fr")
	app := newToolboxApp(t)
	outsideAProject(t, filepath.Dir(work))
	session := editSession(t, app, "kpz-edit")

	var page mcpPage
	isErr, text := callEditTool(t, session, "read_blocks", map[string]any{"doc": "work.kpz!messages.json"}, &page)
	require.False(t, isErr, text)
	ref, rev, _ := page.blockWith(t, "Hello")
	set := map[string]any{"ops": []any{map[string]any{
		"op": "set_content", "at": ref, "if_match": rev, "text": "Hello from an agent",
	}}}
	var res mcpResult
	isErr, text = callEditTool(t, session, "apply_edits", set, &res)
	require.False(t, isErr, text)
	assert.Equal(t, "applied", res.Status, text)
	assert.NotEmpty(t, mustRev(t, session, "work.kpz!messages.json", "Hello from an agent"))
}
