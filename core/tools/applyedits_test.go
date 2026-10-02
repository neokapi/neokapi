package tools

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockWithCode builds a translatable block whose source is "Click <ph/> here"
// with one placeholder inline code, returning the block and its current
// placeholder rendering + canonical content hash.
func blockWithCode(id string) (*model.Block, string, string) {
	b := &model.Block{
		ID:           id,
		Translatable: true,
		Source: []model.Run{
			{Text: &model.TextRun{Text: "Click "}},
			{Ph: &model.PlaceholderRun{ID: "1"}},
			{Text: &model.TextRun{Text: " here"}},
		},
		Targets:    map[model.VariantKey]*model.Target{},
		Properties: map[string]string{},
	}
	return b, model.RunsPlaceholderText(b.Source), model.ComputeContentHash(b.SourceText())
}

func applyOne(t *testing.T, tl *tool.BaseTool, b *model.Block) {
	t.Helper()
	part := &model.Part{Type: model.PartBlock, Resource: b}
	_, err := tl.ApplyContext(context.Background(), part)
	require.NoError(t, err)
}

func TestApplyEdits_FaithfulEdit(t *testing.T) {
	b, _, hash := blockWithCode("p1")
	report := &ApplyReport{}
	// New text keeps the placeholder tag intact, edits surrounding prose.
	edits := map[string]Edit{"p1": {Text: `Press <x id="1/"/> now`, ContentHash: hash}}
	tl := NewApplyEditsTool(edits, nil, report)

	applyOne(t, tl, b)

	assert.Equal(t, []string{"p1"}, report.Applied)
	assert.Empty(t, report.GuardFailed)
	assert.Empty(t, report.Stale)
	// The placeholder run survived and the prose changed.
	assert.Contains(t, model.RunsPlaceholderText(b.Source), `<x id="1/"/>`)
	assert.Contains(t, b.SourceText(), "Press")
	assert.Contains(t, b.SourceText(), "now")
}

func TestApplyEdits_GuardRejectsDroppedCode(t *testing.T) {
	b, _, hash := blockWithCode("p1")
	before := b.SourceText()
	report := &ApplyReport{}
	// New text drops the placeholder — must be rejected, source left unchanged.
	edits := map[string]Edit{"p1": {Text: "Press now", ContentHash: hash}}
	tl := NewApplyEditsTool(edits, nil, report)

	applyOne(t, tl, b)

	assert.Equal(t, []string{"p1"}, report.GuardFailed)
	assert.Empty(t, report.Applied)
	assert.Equal(t, before, b.SourceText(), "source must be unchanged when an edit would drop a code")
}

func TestApplyEdits_DriftGuard(t *testing.T) {
	b, _, _ := blockWithCode("p1")
	before := b.SourceText()
	report := &ApplyReport{}
	edits := map[string]Edit{"p1": {Text: `Press <x id="1/"/> now`, ContentHash: "deadbeef-stale"}}
	tl := NewApplyEditsTool(edits, nil, report)

	applyOne(t, tl, b)

	assert.Equal(t, []string{"p1"}, report.Stale)
	assert.Empty(t, report.Applied)
	assert.Equal(t, before, b.SourceText(), "a stale content_hash must not write")
}

func TestApplyEdits_IdempotentNoOp(t *testing.T) {
	b, cur, hash := blockWithCode("p1")
	report := &ApplyReport{}
	// Supplying the block's current text is a no-op even with a (now-irrelevant)
	// hash — checked before the drift guard so re-running a landed change-set is
	// idempotent.
	edits := map[string]Edit{"p1": {Text: cur, ContentHash: hash}}
	tl := NewApplyEditsTool(edits, nil, report)

	applyOne(t, tl, b)

	assert.Equal(t, []string{"p1"}, report.Skipped)
	assert.Empty(t, report.Applied)
	assert.Empty(t, report.Stale)
}

func TestApplyEdits_MatchByContentHash(t *testing.T) {
	b, _, hash := blockWithCode("p1")
	report := &ApplyReport{}
	// No ID match; resolves by canonical content hash.
	edits := map[string]Edit{hash: {Text: `Press <x id="1/"/> now`, ContentHash: hash}}
	tl := NewApplyEditsTool(nil, edits, report)

	applyOne(t, tl, b)

	assert.Equal(t, []string{"p1"}, report.Applied)
}

func TestApplyEdits_NoEntryPassesThrough(t *testing.T) {
	b, before, _ := blockWithCode("p1")
	report := &ApplyReport{}
	tl := NewApplyEditsTool(map[string]Edit{"other": {Text: "x"}}, nil, report)

	applyOne(t, tl, b)

	assert.Empty(t, report.Applied)
	assert.Empty(t, report.Skipped)
	assert.Equal(t, before, model.RunsPlaceholderText(b.Source))
}

// An edit whose id, or for an edit given without one whose content hash, names
// no block changed nothing, and the report says so instead of letting
// the pass read as clean.
func TestApplyEdits_EditMatchingNoBlockIsNotFound(t *testing.T) {
	b, cur, hash := blockWithCode("p1")
	untranslatable := &model.Block{
		ID:     "code1",
		Source: []model.Run{{Text: &model.TextRun{Text: "fmt.Println()"}}},
	}
	tests := []struct {
		name   string
		byID   map[string]Edit
		byHash map[string]Edit
		want   []string
		ok     bool
	}{
		{
			name: "every edit matched a block",
			byID: map[string]Edit{"p1": {Text: cur, ContentHash: hash}},
			ok:   true,
		},
		{
			name: "an id no block has",
			byID: map[string]Edit{"p1": {Text: cur}, "p9": {Text: "x"}},
			want: []string{"p9"},
		},
		{
			name:   "a content hash no block has",
			byHash: map[string]Edit{"feedface": {Text: "x", ContentHash: "feedface"}},
			want:   []string{NotFoundHashPrefix + "feedface"},
		},
		{
			name:   "an edit matched by content hash",
			byHash: map[string]Edit{hash: {Text: cur, ContentHash: hash}},
			ok:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := &ApplyReport{}
			tl := NewApplyEditsTool(tt.byID, tt.byHash, report)
			blk := *b
			applyOne(t, tl, &blk)
			code := *untranslatable
			applyOne(t, tl, &code)

			assert.Equal(t, tt.want, report.NotFound())
			assert.Equal(t, tt.ok, report.OK())
		})
	}
}

// A block that is not translatable, such as a code block, is found by its id or
// its content hash like any other. An edit giving its text as it is reads as
// already applied; an edit changing it leaves the block as it was and is
// reported as not editable, which keeps the pass from reading as clean.
func TestApplyEdits_BlockThatIsNotEditable(t *testing.T) {
	const code = "fmt.Println()"
	hash := model.ComputeContentHash(code)
	tests := []struct {
		name        string
		byID        map[string]Edit
		byHash      map[string]Edit
		skipped     []string
		notEditable []string
	}{
		{name: "its id with its text", byID: map[string]Edit{"code1": {Text: code, ContentHash: hash}}, skipped: []string{"code1"}},
		{name: "its content hash with its text", byHash: map[string]Edit{hash: {Text: code, ContentHash: hash}}, skipped: []string{"code1"}},
		{name: "its id with a changed text", byID: map[string]Edit{"code1": {Text: "fmt.Print()", ContentHash: hash}}, notEditable: []string{"code1"}},
		{name: "its content hash with a changed text", byHash: map[string]Edit{hash: {Text: "fmt.Print()"}}, notEditable: []string{"code1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blk := &model.Block{ID: "code1", Source: []model.Run{{Text: &model.TextRun{Text: code}}}}
			report := &ApplyReport{}
			applyOne(t, NewApplyEditsTool(tt.byID, tt.byHash, report), blk)

			assert.Empty(t, report.NotFound(), "the edit found its block")
			assert.Equal(t, tt.skipped, report.Skipped)
			assert.Equal(t, tt.notEditable, report.NotEditable)
			assert.Empty(t, report.Applied)
			assert.Equal(t, code, blk.SourceText(), "the block keeps its text")
			assert.Equal(t, tt.notEditable == nil, report.OK())
		})
	}
}

func TestApplyEdits_RejectsFlattenedBranches(t *testing.T) {
	for _, source := range []model.Run{
		model.PluralR(model.PluralRun{Pivot: "count", Forms: map[model.PluralForm][]model.Run{
			model.PluralOne: {model.TextR("one item")}, model.PluralOther: {model.TextR("many items")},
		}}),
		model.SelectR(model.SelectRun{Pivot: "kind", Cases: map[string][]model.Run{
			"other": {model.TextR("items")}, "special": {model.TextR("special items")},
		}}),
	} {
		t.Run(string(source.Kind()), func(t *testing.T) {
			block := model.NewBlock("p1", "")
			block.Source = []model.Run{source}
			report := &ApplyReport{}
			applyOne(t, NewApplyEditsTool(map[string]Edit{"p1": {Text: "new wording"}}, nil, report), block)
			assert.Equal(t, []string{"p1"}, report.GuardFailed)
			assert.Empty(t, report.Applied)
			assert.Equal(t, []model.Run{source}, block.Source)
		})
	}
}

// A character reference is a character in the edit text, so an edit reads a
// block holding one as plain text and may keep, drop or move the character.
// The block remembers the source it was read with, which its writer compares
// with to encode the new wording.
func TestApplyEdits_CharacterReferencesAreText(t *testing.T) {
	ref := func(id, data string) model.Run {
		return model.Run{Ph: &model.PlaceholderRun{ID: id, Type: "code:entity", Data: data}}
	}
	newBlock := func() *model.Block {
		return &model.Block{
			ID:           "p1",
			Translatable: true,
			Source: []model.Run{
				{Text: &model.TextRun{Text: "Fish "}}, ref("1", "&amp;"), {Text: &model.TextRun{Text: " chips "}},
				ref("2", "&lt;"), {Text: &model.TextRun{Text: "3"}},
			},
		}
	}
	tests := []struct {
		name       string
		text       string
		wantBucket string
		wantData   string // the block rendered with its codes' data
	}{
		{name: "the text inspect shows is a no-op", text: "Fish & chips <3", wantBucket: "skipped", wantData: "Fish &amp; chips &lt;3"},
		{name: "the token form is a no-op too", text: `Fish <x id="1/"/> chips <x id="2/"/>3`, wantBucket: "skipped", wantData: "Fish &amp; chips &lt;3"},
		{name: "a kept character keeps its reference", text: "Fish & fries <3", wantBucket: "applied", wantData: "Fish &amp; fries &lt;3"},
		{name: "a rewrite may drop every reference", text: "Pay less today.", wantBucket: "applied", wantData: "Pay less today."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newBlock()
			read := b.Source
			report := &ApplyReport{}
			edits := map[string]Edit{"p1": {Text: tc.text, ContentHash: model.ComputeContentHash(b.SourceText())}}
			applyOne(t, NewApplyEditsTool(edits, nil, report), b)

			buckets := map[string][]string{"applied": report.Applied, "skipped": report.Skipped}
			assert.Equal(t, []string{"p1"}, buckets[tc.wantBucket], "%+v", report)
			assert.Empty(t, report.GuardFailed)
			assert.Equal(t, tc.wantData, model.RenderRunsWithData(b.Source))
			asRead, edited := b.SourceAsRead()
			assert.Equal(t, tc.wantBucket == "applied", edited)
			assert.Equal(t, read, asRead)
		})
	}
}
