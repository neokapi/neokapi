package filehome_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/model"
)

// homeModes are the two ways the file home writes an edition's own file: into
// the file's own skeleton, and from the document's (Options.Materialize, as a
// pull writes a translation).
var homeModes = []struct {
	name string
	opts filehome.Options
}{
	{name: "in place"},
	{name: "materialized", opts: filehome.Options{Materialize: true}},
}

// blockEdition reads doc and returns the block whose own text is text, with its
// reference addressed at edition k.
func blockEdition(t *testing.T, f *fixture, doc, text string, k model.EditionKey) (change.BlockRead, change.Ref) {
	t.Helper()
	page, err := f.svc.Read(context.Background(), change.ReadRequest{Doc: doc, Editions: []model.EditionKey{k}})
	require.NoError(t, err)
	for _, b := range page.Blocks {
		if b.Text == text {
			at := b.Ref
			at.Edition = k
			return b, at
		}
	}
	t.Fatalf("%s holds no block that reads %q", doc, text)
	return change.BlockRead{}, change.Ref{}
}

// setRunsOp sets an edition to one run of text. Edit text would spell a line
// break at an end of the content as the characters it is written with.
func setRunsOp(at change.Ref, ifMatch, text string) change.Op {
	return change.Op{Kind: change.KindSetContent, At: at, IfMatch: ifMatch,
		Body: &change.SetContent{Runs: []model.Run{model.TextR(text)}}}
}

// TestFileHome_ABlockScalarHoldsEveryEnding pins that a YAML block scalar
// holds an edition whatever line breaks it ends with, under every chomping
// indicator: the edit reaches the file, a read gives the edition back whole,
// and the same edit sent again is unchanged.
func TestFileHome_ABlockScalarHoldsEveryEnding(t *testing.T) {
	ctx := context.Background()
	de := mustEdition(t, "de")
	for _, mode := range homeModes {
		for _, indicator := range []string{"|", "|-", "|+", ">", ">-", ">+"} {
			for _, ending := range []string{"", "\n", "\n\n"} {
				t.Run(fmt.Sprintf("%s/%s/%q", mode.name, indicator, ending), func(t *testing.T) {
					f := newTargetFixture(t, map[string]string{
						"guide.yaml":    "title: Welcome\nbody: " + indicator + "\n  Read this first.\nlast: Done\n",
						"de/guide.yaml": "title: Willkommen\nbody: " + indicator + "\n  Lies das zuerst.\nlast: Fertig\n",
					}, mode.opts)
					page, err := f.svc.Read(ctx, change.ReadRequest{Doc: "guide.yaml", Editions: []model.EditionKey{de}})
					require.NoError(t, err)
					require.Len(t, page.Blocks, 3)
					body := page.Blocks[1]
					held, ok := body.Editions["de"]
					require.True(t, ok, "the German file holds the block")
					at := body.Ref
					at.Edition = de
					want := "Lies dies zuerst." + ending

					res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{setRunsOp(at, held.Rev, want)}}, person)
					require.NoError(t, err)
					require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops[0].Error)
					require.Equal(t, change.OpApplied, res.Ops[0].Status)

					page, err = f.svc.Read(ctx, change.ReadRequest{Doc: "guide.yaml", Editions: []model.EditionKey{de}})
					require.NoError(t, err)
					require.Len(t, page.Blocks, 3, "%s", f.read(t, "de/guide.yaml"))
					got := page.Blocks[1].Editions["de"]
					assert.Equal(t, want, got.Text, "the file holds the edition whole: %q", f.read(t, "de/guide.yaml"))
					assert.Equal(t, "Fertig", page.Blocks[2].Editions["de"].Text, "the block after it stays")

					written := f.read(t, "de/guide.yaml")
					res, err = f.svc.Apply(ctx, change.Set{Ops: []change.Op{setRunsOp(at, got.Rev, want)}}, person)
					require.NoError(t, err)
					require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops[0].Error)
					assert.Equal(t, change.OpUnchanged, res.Ops[0].Status)
					assert.Equal(t, written, f.read(t, "de/guide.yaml"))
				})
			}
		}
	}
}

// TestFileHome_AnEditTheFormatDoesNotKeepWholeIsUnchanged pins what the
// service does with an edit that differs from what the file holds only in
// what the file's format does not keep. A Markdown or MDX paragraph holds no
// white space at its ends, so the paragraph with a space or a line break
// added there is written as the bytes the file already has. The file holds
// all of the edit its format keeps: the edit is unchanged, and nothing is
// refused.
func TestFileHome_AnEditTheFormatDoesNotKeepWholeIsUnchanged(t *testing.T) {
	ctx := context.Background()
	de := mustEdition(t, "de")
	for _, mode := range homeModes {
		for _, ext := range []string{"md", "mdx"} {
			for _, tc := range []struct{ name, edit string }{
				{"a space at the end", "Lies das zuerst. "},
				{"a line break at the end", "Lies das zuerst.\n"},
				{"a space at the start", " Lies das zuerst."},
			} {
				t.Run(mode.name+"/"+ext+"/"+tc.name, func(t *testing.T) {
					german := "# Willkommen\n\nLies das zuerst.\n"
					f := newTargetFixture(t, map[string]string{
						"guide." + ext:    "# Welcome\n\nRead this first.\n",
						"de/guide." + ext: german,
					}, mode.opts)
					body, at := blockEdition(t, f, "guide."+ext, "Read this first.", de)
					held := body.Editions["de"]
					require.Equal(t, "Lies das zuerst.", held.Text)

					res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{setRunsOp(at, held.Rev, tc.edit)}}, person)
					require.NoError(t, err)
					require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops[0].Error)
					assert.Equal(t, change.OpUnchanged, res.Ops[0].Status)
					assert.Equal(t, res.Ops[0].Before, res.Ops[0].After, "the edition keeps the revision the file gives it")
					assert.Equal(t, german, f.read(t, "de/guide."+ext))
				})
			}
		}
	}
}

// TestFileHome_AnEditWithOtherWordsIsNotTakenForOneTheFormatKeeps pins the
// limit of that rule: only the white space at the ends is what Markdown does
// not keep. An edit with other words reaches the file.
func TestFileHome_AnEditWithOtherWordsIsNotTakenForOneTheFormatKeeps(t *testing.T) {
	ctx := context.Background()
	de := mustEdition(t, "de")
	f := newTargetFixture(t, map[string]string{
		"guide.md":    "# Welcome\n\nRead this first.\n",
		"de/guide.md": "# Willkommen\n\nLies das zuerst.\n",
	}, filehome.Options{Materialize: true})
	body, at := blockEdition(t, f, "guide.md", "Read this first.", de)

	res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{setRunsOp(at, body.Editions["de"].Rev, "Lies dies zuerst. ")}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops[0].Error)
	assert.Equal(t, change.OpApplied, res.Ops[0].Status)
	assert.Contains(t, f.read(t, "de/guide.md"), "Lies dies zuerst.")
}
