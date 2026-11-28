package change_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// noEdgeSpace is a format that keeps no space and no line break at the ends
// of a block's content, as Markdown keeps none around a paragraph.
type noEdgeSpace struct{}

func (noEdgeSpace) KeptRuns(_ *model.Block, runs []model.Run) []model.Run {
	return format.TrimRunEdges(runs, " \n")
}

// setTextRun sets an edition to one run of text. Edit text would spell a line
// break at an end of the content as the characters it is written with.
func setTextRun(at change.Ref, ifMatch, text string) change.Op {
	return change.Op{Kind: change.KindSetContent, At: at, IfMatch: ifMatch,
		Body: &change.SetContent{Runs: []model.Run{model.TextR(text)}}}
}

// TestService_AnEditTheFormatDoesNotKeepWholeIsUnchanged pins the rule for an
// edit that differs from the edition only in what the edition's format does
// not keep (Place.Keeps): the place already says all of the edit it can say,
// so the edit is unchanged and nothing is written. An edit that says something
// else is applied, and so is every edit where the format declares that it
// keeps content whole.
func TestService_AnEditTheFormatDoesNotKeepWholeIsUnchanged(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		keeps   format.ContentKeeper
		edition string
		edit    string
		status  change.OpStatus
		holds   string
	}{
		{name: "a space after the translation", keeps: noEdgeSpace{}, edition: "nb", edit: "Hei ", status: change.OpUnchanged, holds: "Hei"},
		{name: "a line break after the translation", keeps: noEdgeSpace{}, edition: "nb", edit: "Hei\n", status: change.OpUnchanged, holds: "Hei"},
		{name: "a space before the document's own text", keeps: noEdgeSpace{}, edit: " Hello", status: change.OpUnchanged, holds: "Hello"},
		{name: "other words with a space after them", keeps: noEdgeSpace{}, edition: "nb", edit: "Hallo ", status: change.OpApplied, holds: "Hallo "},
		{name: "a space inside the translation", keeps: noEdgeSpace{}, edition: "nb", edit: "H ei", status: change.OpApplied, holds: "H ei"},
		{name: "a format that declares nothing", edition: "nb", edit: "Hei ", status: change.OpApplied, holds: "Hei "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newMemHome(map[string][]memBlock{"a": {textBlock("greeting", "Hello", "nb", "Hei")}})
			h.keeps = tc.keeps
			svc := newMemService(h)
			b := readBlock(t, svc, "a", "greeting")
			at, rev := b.Ref, b.Rev
			if tc.edition != "" {
				at, rev = atEdition(b.Ref, tc.edition), b.Editions[tc.edition].Rev
			}

			res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setTextRun(at, rev, tc.edit)}}, person.Actor)
			require.NoError(t, err)
			require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops[0].Error)
			assert.Equal(t, tc.status, res.Ops[0].Status)
			if tc.status == change.OpUnchanged {
				assert.Equal(t, res.Ops[0].Before, res.Ops[0].After, "the edition keeps its revision")
			}

			after := readBlock(t, svc, "a", "greeting")
			got := after.Text
			if tc.edition != "" {
				got = after.Editions[tc.edition].Text
			}
			assert.Equal(t, tc.holds, got)
		})
	}
}

// TestService_AChangeTheWriterDropsIsRefused pins what the rule above leaves
// as it was: a change the home's writer does not put in the file is refused,
// whatever the format declares it keeps, and the refusal says what the file
// holds of the edition.
func TestService_AChangeTheWriterDropsIsRefused(t *testing.T) {
	ctx := context.Background()
	removeEdition := func(at change.Ref, ifMatch, _ string) change.Op {
		return change.Op{Kind: change.KindRemoveEdition, At: at, IfMatch: ifMatch, Body: &change.RemoveEdition{}}
	}
	for _, tc := range []struct {
		name    string
		keeps   format.ContentKeeper
		edition string
		op      func(at change.Ref, ifMatch, text string) change.Op
		edit    string
		says    string
	}{
		{name: "other words, in a format that keeps no space at the ends", keeps: noEdgeSpace{}, edition: "nb", op: setTextRun, edit: "Hallo ",
			says: "the file still holds the edition as it was before the change"},
		{name: "a space after the translation, in a format that declares nothing", edition: "nb", op: setTextRun, edit: "Hei ",
			says: "the file still holds the edition as it was before the change"},
		{name: "an edition the file does not hold", keeps: noEdgeSpace{}, edition: "de", op: setTextRun, edit: "Hallo",
			says: "the file holds no such edition"},
		{name: "an edition the change removes", keeps: noEdgeSpace{}, edition: "nb", op: removeEdition,
			says: "the file still holds the edition"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newMemHome(map[string][]memBlock{"a": {textBlock("greeting", "Hello", "nb", "Hei")}})
			h.keeps, h.drops = tc.keeps, true
			svc := newMemService(h)
			b := readBlock(t, svc, "a", "greeting")
			rev := model.AbsentRevision
			if ed, ok := b.Editions[tc.edition]; ok {
				rev = ed.Rev
			}

			res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{tc.op(atEdition(b.Ref, tc.edition), rev, tc.edit)}}, person.Actor)
			require.NoError(t, err)
			require.Equal(t, change.SetRefused, res.Status)
			require.NotNil(t, res.Ops[0].Error)
			assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code)
			assert.Equal(t, "edition", res.Ops[0].Error.Capability)
			assert.True(t, strings.HasSuffix(res.Ops[0].Error.Message, "would be dropped: "+tc.says),
				"the refusal ends with what the file holds: %s", res.Ops[0].Error.Message)
		})
	}
}
