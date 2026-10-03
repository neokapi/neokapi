package changes_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/changes"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// A job's tool drafts translations; the producer commits them as the tool's
// drafts, each with the status and origin the tool gave it.
func TestProducer_CommitsWhatAToolProduced(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	rows, err := f.store.ItemBlocks(ctx, f.project.ID, "main", "a.json", nil)
	require.NoError(t, err)

	p := changes.NewProducer(f.store, f.project, "main", nil, nil)
	p.Read(rows)
	blocks := make([]*model.Block, 0, len(rows))
	for _, sb := range rows {
		sb.Block.SetTargetText("de", "Entwurf "+sb.Block.SourceText())
		sb.Block.StampTargetProvenance("de", model.TargetStatusDraft, model.Origin{Kind: model.OriginAI, Engine: "demo"})
		blocks = append(blocks, sb.Block)
	}
	landed, err := p.Commit(ctx, "translate", blocks)
	require.NoError(t, err)
	assert.Len(t, landed, len(rows))

	after, err := f.store.ItemBlocks(ctx, f.project.ID, "main", "a.json", nil)
	require.NoError(t, err)
	for _, sb := range after {
		target := sb.Block.Target("de")
		require.NotNil(t, target, sb.SourceID)
		assert.Equal(t, "Entwurf "+sb.Block.SourceText(), model.RunsText(target.Runs))
		assert.Equal(t, model.TargetStatusDraft, target.Status)
		assert.Equal(t, model.OriginAI, target.Origin.Kind)
		assert.Equal(t, "demo", target.Origin.Engine)
	}
}

// A block a person changed after the job read it keeps the person's wording,
// and the job's other drafts land.
func TestProducer_LeavesABlockAPersonChangedSinceTheRead(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	rows, err := f.store.ItemBlocks(ctx, f.project.ID, "main", "a.json", nil)
	require.NoError(t, err)
	p := changes.NewProducer(f.store, f.project, "main", nil, nil)
	p.Read(rows)

	one := read(t, f.svc, "a.json", "one")
	at := one.Ref
	at.Edition = model.EditionKey{Locale: "fr"}
	res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{setText(at, one.Editions["fr"].Rev, "Le premier, relu")}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	for _, sb := range rows {
		sb.Block.SetTargetText("fr", "Brouillon "+sb.Block.SourceText())
	}
	landed, err := p.Commit(ctx, "translate", []*model.Block{rows[0].Block, rows[1].Block})
	require.NoError(t, err)
	require.Len(t, landed, 1, "only the block nobody changed since the read takes the draft")

	after, err := f.store.ItemBlocks(ctx, f.project.ID, "main", "a.json", nil)
	require.NoError(t, err)
	for _, sb := range after {
		switch sb.SourceID {
		case "one":
			assert.Equal(t, "Le premier, relu", sb.Block.TargetText("fr"), "the person's wording stays")
		case "two":
			assert.Equal(t, "Brouillon Second", sb.Block.TargetText("fr"))
		}
	}
}

// An extraction tool marks a span on the source it read. A person rewrites the
// source before the job commits, so the span would mark other words: it does
// not land, and an overlay on a source nobody touched does.
func TestProducer_LeavesAnOverlayOnContentThatMovedSinceTheRead(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	rows, err := f.store.ItemBlocks(ctx, f.project.ID, "main", "a.json", nil)
	require.NoError(t, err)
	p := changes.NewProducer(f.store, f.project, "main", nil, nil)
	p.Read(rows)

	two := read(t, f.svc, "a.json", "two")
	res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{setText(two.Ref, two.Rev, "Totally different wording")}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	for _, sb := range rows {
		b := sb.Block
		b.AddOverlaySpan(model.OverlayEntity, model.Span{ID: "entity:0", Range: model.RangeAnchor(b.SourceRuns(), 0, 5),
			Value: &model.EntityAnnotation{Text: b.SourceText()[:5]}})
	}
	landed, err := p.Commit(ctx, "extract", []*model.Block{rows[0].Block, rows[1].Block})
	require.NoError(t, err)
	require.Len(t, landed, 1, "only the block whose source the tool read takes its span")

	after, err := f.store.ItemBlocks(ctx, f.project.ID, "main", "a.json", nil)
	require.NoError(t, err)
	for _, sb := range after {
		span := sb.Block.OverlaySpan(model.OverlayEntity, "entity:0")
		switch sb.SourceID {
		case "one":
			require.NotNil(t, span, "a span on an unchanged source lands")
		case "two":
			assert.Equal(t, "Totally different wording", sb.Block.SourceText())
			assert.Nil(t, span, "a span on a source that moved does not land")
		}
	}
}

// A tool writes block annotations and properties beside the content: a draft
// with its candidates, and a block it only analysed. Both land on the rows. A
// block a person changed since the read keeps what it holds, and what the tool
// found on its old content stays off it.
func TestProducer_KeepsTheAnnotationsAndPropertiesAToolWrote(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	rows, err := f.store.ItemBlocks(ctx, f.project.ID, "main", "a.json", nil)
	require.NoError(t, err)
	more, err := f.store.ItemBlocks(ctx, f.project.ID, "main", "b.json", nil)
	require.NoError(t, err)
	rows = append(rows, more...)
	p := changes.NewProducer(f.store, f.project, "main", nil, nil)
	p.Read(rows)

	three := read(t, f.svc, "b.json", "three")
	res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{setText(three.Ref, three.Rev, "Third, rewritten")}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	byKey := map[string]*model.Block{}
	for _, sb := range rows {
		byKey[sb.SourceID] = sb.Block
		sb.Block.SetAnno(model.AnnoAltTranslation, &model.AltTranslations{Items: []*model.AltTranslation{
			{Locale: "de", Target: []model.Run{{Text: &model.TextRun{Text: "Kandidat " + sb.SourceID}}}, Score: 0.8}}})
		if sb.Block.Properties == nil {
			sb.Block.Properties = map[string]string{}
		}
		sb.Block.Properties["terminology"] = `[{"term":"` + sb.SourceID + `"}]`
	}
	byKey["one"].SetTargetText("de", "Erster")
	landed, err := p.Commit(ctx, "memory", []*model.Block{byKey["one"], byKey["two"], byKey["three"]})
	require.NoError(t, err)
	assert.Len(t, landed, 2, "the drafts on content nobody moved land")

	stored := func(item, key string) *model.Block {
		got, err := f.store.ItemBlocks(ctx, f.project.ID, "main", item, []string{key})
		require.NoError(t, err)
		require.Len(t, got, 1)
		return got[0].Block
	}
	for _, key := range []string{"one", "two"} {
		b := stored("a.json", key)
		alts := b.AltTranslations()
		require.Len(t, alts, 1, key)
		assert.Equal(t, "Kandidat "+key, model.RunsText(alts[0].Target), key)
		assert.Equal(t, `[{"term":"`+key+`"}]`, b.Properties["terminology"], key)
	}
	assert.Equal(t, "Erster", stored("a.json", "one").TargetText("de"))
	moved := stored("b.json", "three")
	assert.Empty(t, moved.AltTranslations(), "what the tool found on content that moved stays off the row")
	assert.Empty(t, moved.Properties["terminology"])
}
