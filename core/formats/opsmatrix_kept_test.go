package formats_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"slices"
	"testing"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The kept-content cells hold every format of the matrix to what it keeps of
// a block's content. A format that reads content back without a space or a
// line break it was written with must say so (format.ContentKeeper), or the
// change service refuses every later write of that content as dropped.

// Each cell adds one edge to every block of a fixture, writes the document
// and reads it back. It holds when every block reads back as given, or as the
// writer's KeptRuns gives it. A fixture names a cell that does not hold in its
// unkept map, and the matrix asserts that it still does not.

// contentEdge is white space added at one end of a block's content.
type contentEdge struct {
	name          string
	before, after string
}

var contentEdges = []contentEdge{
	{name: "space_after", after: " "},
	{name: "line_break_after", after: "\n"},
	{name: "space_before", before: " "},
}

// key names the cell in a fixture's unkept map.
func (e contentEdge) key(edition editionRole) string { return e.name + "@" + string(edition) }

// editions lists the editions a fixture's documents hold.
func (fx opsFixture) editions() []editionRole {
	if fx.target == "" {
		return []editionRole{sourceEdition}
	}
	return []editionRole{sourceEdition, targetEdition}
}

// tryRead reads data as readEditable does and returns the error a reader
// reports, for a document a cell may have left unreadable.
func (fx opsFixture) tryRead(t *testing.T, data []byte, locale model.LocaleID) (blocks []*model.Block, err error) {
	t.Helper()
	ctx := context.Background()
	reader, writer := fx.newPair(t)
	store, err := format.NewWiredSkeleton(reader, writer)
	require.NoError(t, err)
	require.NotNil(t, store, "%s has no skeleton pair", fx.format)
	defer store.Close()
	if err := reader.Open(ctx, &model.RawDocument{
		URI:          "opsmatrix." + string(fx.format),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: locale,
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}); err != nil {
		return nil, err
	}
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			err = res.Error
			continue
		}
		if res.Part == nil {
			continue
		}
		if b, ok := res.Part.Resource.(*model.Block); ok {
			blocks = append(blocks, b)
		}
	}
	if cerr := reader.Close(); err == nil {
		err = cerr
	}
	return blocks, err
}

// keepsEdge writes the fixture with edge added to every block of edition that
// takes it and reports what, if anything, the format did not keep: "" when
// every block read back whole or as the writer's KeptRuns gives it.
func (fx opsFixture) keepsEdge(t *testing.T, edition editionRole, edge contentEdge) string {
	t.Helper()
	doc := fx.render(t, nil)
	var readLoc, writeLoc model.LocaleID
	key := model.EditionKey{}
	if edition == targetEdition {
		readLoc, writeLoc = fx.target, fx.target
		key = model.EditionKey{Locale: writeLoc}.Canonical()
	}
	_, keeper := fx.newPair(t)

	// want is, per block, the content it must read back with: nil for a block
	// the cell leaves alone.
	var want [][]string
	edited := 0
	out, werr := fx.editDocument(t, doc.input, readLoc, writeLoc, func(b *model.Block, env change.BlockEnv) {
		want = append(want, nil)
		runs, ok := editionRuns(b, edition, readLoc)
		if !ok || !b.Translatable || len(runs) == 0 || model.HasStructuredRuns(runs) {
			return
		}
		given := slices.Clone(runs)
		if edge.before != "" {
			given = append([]model.Run{model.TextR(edge.before)}, given...)
		}
		if edge.after != "" {
			given = append(given, model.TextR(edge.after))
		}
		res := change.ApplyBlock(b, []change.Op{{
			Kind: change.KindSetContent, At: change.Ref{Doc: "opsmatrix", Block: b.ID, Edition: key},
			IfMatch: change.AnyRevision, Body: &change.SetContent{Runs: given},
		}}, env)
		if res[0].Status != change.OpApplied {
			return
		}
		set, _ := editionRuns(b, edition, readLoc)
		holds := []string{editionContent(set)}
		if k, ok := keeper.(format.ContentKeeper); ok {
			holds = append(holds, editionContent(k.KeptRuns(b, set)))
		}
		want[len(want)-1] = holds
		edited++
	})
	require.Positive(t, edited, "the fixture holds no block that takes content")
	if werr != nil {
		return fmt.Sprintf("the writer refuses the document: %v", werr)
	}
	back, err := fx.tryRead(t, out, readLoc)
	if err != nil {
		return fmt.Sprintf("the document the writer wrote does not read: %v", err)
	}
	if len(back) != len(want) {
		return fmt.Sprintf("the document reads back as %d blocks, not %d", len(back), len(want))
	}
	for i, b := range back {
		if want[i] == nil {
			continue
		}
		runs, _ := editionRuns(b, edition, readLoc)
		if got := editionContent(runs); !slices.Contains(want[i], got) {
			return fmt.Sprintf("block %s was given %q and reads back %q", b.ID, want[i][0], got)
		}
	}
	return ""
}

// TestOperationsMatrixKeepsContentOrDeclaresWhatItKeeps runs the kept-content
// cells.
func TestOperationsMatrixKeepsContentOrDeclaresWhatItKeeps(t *testing.T) {
	for _, fx := range opsMatrix() {
		t.Run(fx.id(), func(t *testing.T) {
			cells := map[string]bool{}
			for _, edition := range fx.editions() {
				for _, edge := range contentEdges {
					cell := edge.key(edition)
					cells[cell] = true
					t.Run(cell, func(t *testing.T) {
						lost := fx.keepsEdge(t, edition, edge)
						reason, declared := fx.unkept[cell]
						switch {
						case declared && lost == "":
							t.Errorf("%s names %s as content it does not keep (%s), and it reads back whole or as its writer's KeptRuns gives it: take the entry out", fx.id(), cell, reason)
						case !declared && lost != "":
							t.Errorf("%s does not keep %s and does not say so: %s.\nKeep the content, or implement format.ContentKeeper on the writer with what the format keeps", fx.id(), cell, lost)
						}
					})
				}
			}
			for cell := range fx.unkept {
				assert.True(t, cells[cell], "%s names a kept-content cell %q the matrix does not run", fx.id(), cell)
			}
		})
	}
}

// TestOperationsMatrixKeepsWhatItReads pins the other half of a ContentKeeper's
// declaration: content the reader gave is content the format keeps, so
// KeptRuns returns every block of every fixture as it was read.
func TestOperationsMatrixKeepsWhatItReads(t *testing.T) {
	for _, fx := range opsMatrix() {
		_, writer := fx.newPair(t)
		keeper, ok := writer.(format.ContentKeeper)
		if !ok {
			continue
		}
		t.Run(fx.id(), func(t *testing.T) {
			for _, edition := range fx.editions() {
				var locale model.LocaleID
				if edition == targetEdition {
					locale = fx.target
				}
				for _, b := range fx.readEditable(t, fx.render(t, nil).input, locale) {
					runs, ok := editionRuns(b, edition, locale)
					if !ok {
						continue
					}
					assert.Equal(t, editionContent(runs), editionContent(keeper.KeptRuns(b, runs)),
						"block %s (%s) is content the reader gave", b.ID, b.Type)
				}
			}
		})
	}
}
