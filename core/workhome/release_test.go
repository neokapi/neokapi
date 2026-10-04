package workhome_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/workhome"
	"github.com/neokapi/neokapi/core/workspace"
)

// A delivery releases what it wrote into an edition's file, and only that:
// the release expects the head the delivery read, so a write that lands in
// between stays kept, and a release of part of an edition leaves the rest.
// A release of the whole edition, a person's or an agent's write to a block,
// and a rebase each settle what a merge left divergent.

var de = model.EditionKey{Locale: "de"}

var merge = change.Actor{Kind: change.ActorTool, Name: "merge"}

// TestWorkspaceHome_ReleaseExpectsTheHeadItRead: a write that lands between a
// delivery's read and its release is never released unread.
func TestWorkspaceHome_ReleaseExpectsTheHeadItRead(t *testing.T) {
	ctx := context.Background()
	m := newMachine(t, t.TempDir())
	f := newKeptFixture(t, m, map[string]string{"a.json": `{"greeting": "Hello there", "farewell": "Goodbye"}` + "\n"}, filehome.Options{})
	f.draft(t, "a.json", map[string]string{"greeting": "Hallo"})

	delivered, err := m.home.Read(ctx, "a.json", de)
	require.NoError(t, err)
	require.Len(t, delivered.Rows, 1)

	// A reviewer's edit lands after the delivery read the edition.
	setGerman(t, f, "greeting", "Servus")

	_, err = m.home.Release(ctx, "a.json", de, workhome.Release{Token: delivered.Token, Actor: merge, Origin: "merge"})
	require.ErrorIs(t, err, workhome.ErrReleaseMoved)
	require.ErrorIs(t, err, workspace.ErrHeadMoved)
	assert.Equal(t, map[string]string{"greeting": "Servus"}, german(t, f), "the edit the delivery never read stays kept")

	again, err := m.home.Read(ctx, "a.json", de)
	require.NoError(t, err)
	id, err := m.home.Release(ctx, "a.json", de, workhome.Release{Token: again.Token, Actor: merge, Origin: "merge"})
	require.NoError(t, err)
	assert.NotEmpty(t, id)
	assert.Empty(t, german(t, f), "a release of what the delivery read lands")
}

// TestWorkspaceHome_ReleaseOfSomeBlocksKeepsTheRest: a release names the
// blocks a file now holds, and the workspace home keeps the others.
func TestWorkspaceHome_ReleaseOfSomeBlocksKeepsTheRest(t *testing.T) {
	ctx := context.Background()
	m := newMachine(t, t.TempDir())
	f := newKeptFixture(t, m, map[string]string{"a.json": `{"greeting": "Hello there", "farewell": "Goodbye"}` + "\n"}, filehome.Options{})
	f.draft(t, "a.json", map[string]string{"greeting": "Hallo", "farewell": "Tschüss"})

	held, err := m.home.Read(ctx, "a.json", de)
	require.NoError(t, err)
	_, err = m.home.Release(ctx, "a.json", de, workhome.Release{Token: held.Token, Blocks: []string{"greeting"}, Actor: merge, Origin: "merge"})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"farewell": "Tschüss"}, german(t, f))
}

// TestFold_SettlesDivergentWrites holds the rules by which a divergent write
// leaves an edition's head.
func TestFold_SettlesDivergentWrites(t *testing.T) {
	rev := func(text string) string { return model.RunsRevision(de, []model.Run{model.TextR(text)}) }
	set := func(block, before, after string) workhome.BlockWrite {
		return workhome.BlockWrite{Block: block, Before: before, After: after, Runs: model.CanonicalRunsJSON([]model.Run{model.TextR(after)})}
	}
	start := workhome.Write{Op: "01", Blocks: []workhome.BlockWrite{set("a", model.AbsentRevision, rev("A")), set("b", model.AbsentRevision, rev("B"))}}
	// Two writes staged on the same head: "02" advances it, "03" diverges.
	first := workhome.Write{Op: "02", Base: "01", Writer: true, Blocks: []workhome.BlockWrite{set("a", rev("A"), rev("A2"))}}
	other := workhome.Write{Op: "03", Base: "01", Writer: true, Blocks: []workhome.BlockWrite{set("a", rev("A"), rev("A3")), set("b", rev("B"), rev("B3"))}}

	cases := []struct {
		name string
		then workhome.Write
		left []string // the blocks the divergent write still lists; nil when it is settled
	}{
		{
			name: "a person's write to one of its blocks settles that block",
			then: workhome.Write{Op: "04", Base: "02", Writer: true, Blocks: []workhome.BlockWrite{set("a", rev("A2"), rev("A4"))}},
			left: []string{"b"},
		},
		{
			name: "a person's write to every one of its blocks settles it",
			then: workhome.Write{Op: "04", Base: "02", Writer: true, Blocks: []workhome.BlockWrite{set("a", rev("A2"), rev("A4")), set("b", rev("B"), rev("B4"))}},
		},
		{
			name: "a tool's draft settles nothing",
			then: workhome.Write{Op: "04", Base: "02", Blocks: []workhome.BlockWrite{set("a", rev("A2"), rev("A4"))}},
			left: []string{"a", "b"},
		},
		{
			name: "a release of the whole edition settles it",
			then: workhome.Write{Op: "04", Base: "02", Release: true, Blocks: []workhome.BlockWrite{
				{Block: "a", Before: rev("A2"), After: model.AbsentRevision}, {Block: "b", Before: rev("B"), After: model.AbsentRevision}}},
		},
		{
			name: "a rebase that names it as its cause settles it",
			then: workhome.Write{Op: "04", Base: "02", Cause: "03", Blocks: []workhome.BlockWrite{set("b", rev("B"), rev("B3"))}},
		},
		{
			name: "a write staged on an older head settles nothing",
			then: workhome.Write{Op: "04", Base: "01", Writer: true, Blocks: []workhome.BlockWrite{set("a", rev("A"), rev("A4")), set("b", rev("B"), rev("B4"))}},
			left: []string{"a", "b"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := workhome.Fold([]workhome.Write{start, first, other, tc.then})
			var left []string
			for _, d := range h.Divergent {
				if d.Op != "03" {
					continue
				}
				for _, m := range d.Blocks {
					left = append(left, m.Block)
				}
			}
			assert.Equal(t, tc.left, left)
		})
	}
}

// TestWorkspaceHome_AnEditToAConflictedBlockSettlesIt: a person who sets the
// wording of a block two machines both wrote settles the conflict, on both
// machines once their logs merge.
func TestWorkspaceHome_AnEditToAConflictedBlockSettlesIt(t *testing.T) {
	ctx := context.Background()
	a, b := twoMachines(t, map[string]string{"greeting": "Hallo", "thanks": "Danke"})
	setGerman(t, a, "thanks", "Vielen Dank")
	time.Sleep(3 * time.Millisecond)
	setGerman(t, b, "thanks", "Danke schön")
	mergeInto(t, a.m, b.m)
	mergeInto(t, b.m, a.m)
	conflicts, err := a.m.home.Conflicts(ctx)
	require.NoError(t, err)
	require.Len(t, conflicts, 1)

	setGerman(t, a, "thanks", "Herzlichen Dank")
	mergeInto(t, b.m, a.m)
	for _, f := range []*keptFixture{a, b} {
		conflicts, err := f.m.home.Conflicts(ctx)
		require.NoError(t, err)
		assert.Empty(t, conflicts, "the person's wording settles the conflict")
		assert.Equal(t, "Herzlichen Dank", german(t, f)["thanks"])
	}
}

// TestWorkspaceHome_ConflictsLeaveOutWhatTheRebaseCarries: a divergent write
// whose blocks have not moved is carried over by the next rebase, so it is
// not listed as a conflict before then either.
func TestWorkspaceHome_ConflictsLeaveOutWhatTheRebaseCarries(t *testing.T) {
	ctx := context.Background()
	a, b := twoMachines(t, map[string]string{"greeting": "Hallo", "thanks": "Danke"})
	setGerman(t, a, "greeting", "Guten Tag")
	time.Sleep(3 * time.Millisecond)
	setGerman(t, b, "thanks", "Danke schön")
	mergeInto(t, a.m, b.m)

	head, _, err := a.m.st.Heads.Head(ctx, "a.json", "de")
	require.NoError(t, err)
	require.Len(t, head.Divergent, 1, "the second write did not advance the head")
	conflicts, err := a.m.home.Conflicts(ctx)
	require.NoError(t, err)
	assert.Empty(t, conflicts, "and the rebase carries it over")
}

// TestWorkspaceHome_RebaseCarriesOnlyTheBlocksStillListed: a divergent write
// a person settled for one of its blocks is carried over for the others, and
// the person's wording stands.
func TestWorkspaceHome_RebaseCarriesOnlyTheBlocksStillListed(t *testing.T) {
	ctx := context.Background()
	a, b := twoMachines(t, map[string]string{"greeting": "Hallo", "thanks": "Danke"})
	setGerman(t, a, "thanks", "Vielen Dank")
	time.Sleep(3 * time.Millisecond)
	// One write on the second machine changes both blocks.
	page, err := b.svc.Read(ctx, change.ReadRequest{Doc: "de/a.json"})
	require.NoError(t, err)
	var ops []change.Op
	for _, blk := range page.Blocks {
		text := map[string]string{"greeting": "Grüß Gott", "thanks": "Danke schön"}[blk.Ref.Block]
		ops = append(ops, change.Op{Kind: change.KindSetContent, At: blk.Ref, IfMatch: blk.Rev, Body: &change.SetContent{Text: &text}})
	}
	res, err := b.svc.Apply(ctx, change.Set{Ops: ops}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	mergeInto(t, a.m, b.m)

	conflicts, err := a.m.home.Conflicts(ctx)
	require.NoError(t, err)
	require.Len(t, conflicts, 1)
	assert.Equal(t, []string{"thanks"}, conflicts[0].Blocks)

	setGerman(t, a, "thanks", "Herzlichen Dank")
	n, err := a.m.p.RebaseWorkspace(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, map[string]string{"greeting": "Grüß Gott", "thanks": "Herzlichen Dank"}, german(t, a),
		"the block nobody else wrote is carried over, and the settled one keeps the person's wording")
	hist, err := a.m.st.History.Edition(ctx, "a.json", "greeting", "de", 1)
	require.NoError(t, err)
	require.Len(t, hist, 1)
	assert.Equal(t, "rebase", hist[0].Origin)
	assert.Equal(t, []string{"set_content"}, hist[0].Ops, "the write carried over keeps the kinds of the operations that made it")
	conflicts, err = a.m.home.Conflicts(ctx)
	require.NoError(t, err)
	assert.Empty(t, conflicts)
}

// headMovingKeeper is a keeper whose head another process moves after the
// stage settled and before the commit: a desktop edit that lands in that
// window.
type headMovingKeeper struct {
	filehome.Keeper
	home  *workhome.Home
	moved bool
}

func (k *headMovingKeeper) Commit(ctx context.Context, w filehome.KeptWrite) (string, error) {
	if !k.moved {
		k.moved = true
		now, err := k.home.Edition(ctx, w.Doc, w.Edition)
		if err != nil {
			return "", err
		}
		other := model.Edition{Runs: []model.Run{model.TextR("Tschüss")}}
		if _, err := k.home.Commit(ctx, filehome.KeptWrite{Doc: w.Doc, Edition: w.Edition, Token: now.Token,
			Changes: []filehome.KeptChange{{Block: "farewell", Before: model.AbsentRevision, Edition: &other}},
			Record:  &change.Record{Actor: person, Origin: "desktop"}}); err != nil {
			return "", err
		}
	}
	return k.Keeper.Commit(ctx, w)
}

// TestWorkspaceHome_AHeadThatMovesAtCommitRefusesTheChange: a head that moves
// between the stage's settle and its commit refuses the change set, with
// nothing landed, rather than failing the call.
func TestWorkspaceHome_AHeadThatMovesAtCommitRefusesTheChange(t *testing.T) {
	ctx := context.Background()
	m := newMachine(t, t.TempDir())
	f := newKeptFixture(t, m, map[string]string{"a.json": `{"greeting": "Hello there", "farewell": "Goodbye"}` + "\n"}, filehome.Options{})
	f.draft(t, "a.json", map[string]string{"greeting": "Hallo"})

	svc := serviceOver(t, f.dir, &headMovingKeeper{Keeper: m.home, home: m.home})

	page, err := svc.Read(ctx, change.ReadRequest{Doc: "de/a.json", Blocks: []string{"greeting"}})
	require.NoError(t, err)
	text := "Servus"
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{{Kind: change.KindSetContent, At: page.Blocks[0].Ref,
		IfMatch: page.Blocks[0].Rev, Body: &change.SetContent{Text: &text}}}}, person)
	require.NoError(t, err, "a moved head is a refusal, not a failure")
	assert.Equal(t, change.SetRefused, res.Status)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeDocChanged, res.Ops[0].Error.Code)
	kept := german(t, f)
	assert.Equal(t, "Hallo", kept["greeting"], "the refused change landed nothing")
	assert.Equal(t, "Tschüss", kept["farewell"], "and the other write stands")
}

// serviceOver is a change service over the JSON documents in dir whose German
// editions keeper keeps.
func serviceOver(t *testing.T, dir string, keeper filehome.Keeper) *change.Service {
	t.Helper()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	home := filehome.New(keptLayout{root: dir, reg: reg, home: keeper}, filehome.Options{LockDir: filepath.Join(t.TempDir(), "locks")})
	return change.NewService(filehome.Formats{Registry: reg}, change.OneHome(home))
}
