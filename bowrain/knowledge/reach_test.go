package knowledge

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"

	"github.com/neokapi/neokapi/bowrain/core/store"
)

// withTarget attaches a committed translation to a block so the reach split can
// count the work a change would pull back or invalidate.
func withTarget(b *store.StoredBlock, locale model.LocaleID, text string, status model.TargetStatus) *store.StoredBlock {
	if b.Targets == nil {
		b.Targets = map[model.VariantKey]*model.Target{}
	}
	b.Targets[model.Variant(locale)] = model.NewTarget([]model.Run{model.TextR(text)}, status)
	return b
}

// A ban with no successor is an annotation: it changes what the checks flag and
// nothing else, so the cost is a re-check that pulls the affected translations
// back into review.
func TestReach_BanWithoutAReplacementIsAnnotate(t *testing.T) {
	ctx := context.Background()

	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(ctx, concept("c1", term("foobar", "en-US", model.TermAdmitted))))

	bs := newFakeBlockSource()
	bs.addProject(&store.Project{ID: "proj1", Name: "Docs", WorkspaceID: "ws"})
	bs.addBlocks("proj1", "main",
		withTarget(withTarget(
			srcBlock("b1", "guide.md", "en-US", "Please use foobar here"),
			"nb", "Bruk foobar her", model.TargetStatusReviewed),
			"de", "Nutze foobar hier", model.TargetStatusDraft),
	)

	e := NewEngine(bs, tb, newFakeProfileStore(), nil)
	imp, err := e.EvaluateChangeSet(ctx, "ws", ChangeSet{}, []ChangeSetOp{
		mustOp(t, 0, OpTermStatus, TermStatusPayload{
			ConceptID: "c1", Locale: "en-US", Text: "foobar",
			From: model.TermAdmitted, To: model.TermForbidden,
		}),
	}, EvalOptions{})
	require.NoError(t, err)

	require.NotNil(t, imp.Reach)
	assert.Equal(t, 1, imp.Reach.Annotate.Blocks)
	assert.Equal(t, 0, imp.Reach.Transform.Blocks)
	assert.Equal(t, 1, imp.Reach.Annotate.Collections)
	assert.Equal(t, 1, imp.Reach.Annotate.Projects)
	assert.Equal(t, 2, imp.Reach.Annotate.Targets, "both committed translations are re-checked")
	assert.Equal(t, 1, imp.Reach.Annotate.Approved, "only the reviewed one leaves a state someone signed off")
	assert.Equal(t, []string{"de", "nb"}, imp.Reach.Annotate.Locales)
	assert.Empty(t, imp.Reach.TransformProjects)
}

// The same ban, once the graph can resolve a successor, is an instruction to
// rewrite the source — and rewriting source invalidates every translation of it.
func TestReach_BanWithAReplacementIsTransform(t *testing.T) {
	ctx := context.Background()

	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(ctx, concept("old", term("foobar", "en-US", model.TermAdmitted))))
	require.NoError(t, tb.AddConcept(ctx, concept("new", term("widget", "en-US", model.TermPreferred))))
	require.NoError(t, tb.AddRelation(ctx, terms.ConceptRelation{
		ID: "r1", SourceID: "old", TargetID: "new", RelationType: graph.LabelUseInstead,
	}))

	bs := newFakeBlockSource()
	bs.addProject(&store.Project{ID: "proj1", Name: "Docs", WorkspaceID: "ws"})
	bs.addBlocks("proj1", "main",
		withTarget(srcBlock("b1", "guide.md", "en-US", "Please use foobar here"),
			"nb", "Bruk foobar her", model.TargetStatusSignedOff),
	)

	e := NewEngine(bs, tb, newFakeProfileStore(), nil)
	imp, err := e.EvaluateChangeSet(ctx, "ws", ChangeSet{}, []ChangeSetOp{
		mustOp(t, 0, OpTermStatus, TermStatusPayload{
			ConceptID: "old", Locale: "en-US", Text: "foobar",
			From: model.TermAdmitted, To: model.TermForbidden,
		}),
	}, EvalOptions{})
	require.NoError(t, err)

	require.NotNil(t, imp.Reach)
	assert.Equal(t, 0, imp.Reach.Annotate.Blocks)
	assert.Equal(t, 1, imp.Reach.Transform.Blocks)
	assert.Equal(t, 1, imp.Reach.Transform.Targets)
	assert.Equal(t, 1, imp.Reach.Transform.Approved)
	assert.Equal(t, []string{"nb"}, imp.Reach.Transform.Locales)
	require.Len(t, imp.Reach.TransformProjects, 1)
	assert.Equal(t, "proj1", imp.Reach.TransformProjects[0].ProjectID)
	assert.Equal(t, "Docs", imp.Reach.TransformProjects[0].ProjectName)
}

// A voice rule that names what to write instead is the same instruction from the
// other side of the graph, and classifies the same way.
func TestReach_VoiceRuleWithReplacementIsTransform(t *testing.T) {
	ctx := context.Background()

	bs := newFakeBlockSource()
	bs.addProject(&store.Project{ID: "proj1", Name: "Site", WorkspaceID: "ws"})
	bs.addBlocks("proj1", "main",
		srcBlock("b1", "home.json", "en-US", "Embrace synergy across teams"),
		srcBlock("b2", "home.json", "en-US", "We deliver leverage daily"),
	)

	profile := &coreprofile.VoiceProfile{ID: "p1", Name: "Acme", Scope: "ws"}
	e := NewEngine(bs, terms.NewInMemoryStore(), newFakeProfileStore(profile), nil)

	imp, err := e.EvaluateChangeSet(ctx, "ws", ChangeSet{}, []ChangeSetOp{
		mustOp(t, 0, OpVoiceRuleAdd, VoiceRuleAddPayload{
			ProfileID: "p1", List: VoiceListForbidden,
			Rule: coreprofile.TermRule{Term: "synergy", Replacement: "teamwork"},
		}),
		mustOp(t, 1, OpVoiceRuleAdd, VoiceRuleAddPayload{
			ProfileID: "p1", List: VoiceListForbidden,
			Rule: coreprofile.TermRule{Term: "leverage"},
		}),
	}, EvalOptions{})
	require.NoError(t, err)

	require.NotNil(t, imp.Reach)
	assert.Equal(t, 1, imp.Reach.Transform.Blocks, "the rule naming a successor prescribes a rewrite")
	assert.Equal(t, 1, imp.Reach.Annotate.Blocks, "the rule that only bans a word flags it")
	assert.Equal(t, 2, imp.AffectedBlocks, "the split partitions the affected blocks exactly")
}

// A block reached in several locales is one block to act on, not several, and
// its translations must be counted once. Otherwise the transform cost — the
// number that decides whether a draft is affordable — is multiplied by the
// number of locales it was evaluated in.
func TestReach_CountsEachBlockOnceAcrossLocales(t *testing.T) {
	ctx := context.Background()

	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(ctx, concept("c1",
		term("foobar", "en-US", model.TermAdmitted),
		term("foobar", "nb", model.TermAdmitted),
	)))

	bs := newFakeBlockSource()
	bs.addProject(&store.Project{ID: "proj1", Name: "Docs", WorkspaceID: "ws"})
	bs.addBlocks("proj1", "main",
		withTarget(srcBlock("b1", "guide.md", "en-US", "Please use foobar here"),
			"nb", "Bruk foobar her", model.TargetStatusReviewed),
	)

	e := NewEngine(bs, tb, newFakeProfileStore(), nil)
	imp, err := e.EvaluateChangeSet(ctx, "ws", ChangeSet{}, []ChangeSetOp{
		mustOp(t, 0, OpTermStatus, TermStatusPayload{
			ConceptID: "c1", Locale: "en-US", Text: "foobar",
			From: model.TermAdmitted, To: model.TermForbidden,
		}),
		mustOp(t, 1, OpTermStatus, TermStatusPayload{
			ConceptID: "c1", Locale: "nb", Text: "foobar",
			From: model.TermAdmitted, To: model.TermForbidden,
		}),
	}, EvalOptions{Locales: []model.LocaleID{"en-US", "nb"}})
	require.NoError(t, err)

	assert.Equal(t, 2, imp.AffectedBlocks, "two (block, locale) rows are affected")
	require.NotNil(t, imp.Reach)
	assert.Equal(t, 1, imp.Reach.Annotate.Blocks, "but one block to act on")
	assert.Equal(t, 1, imp.Reach.Annotate.Targets, "and its one translation counted once")
}

// The split survives the round trip through the stored summary, because the
// stored summary is what a reader sees by default.
func TestReach_SurvivesTheStoredSummary(t *testing.T) {
	imp := ChangeSetImpact{
		TotalBlocks:    10,
		AffectedBlocks: 4,
		Reach: &Reach{
			Annotate:  ReachClass{Blocks: 3, Targets: 5, Approved: 2, Locales: []string{"de", "nb"}},
			Transform: ReachClass{Blocks: 1, Targets: 2, Approved: 1, Projects: 1, Locales: []string{"nb"}},
			TransformProjects: []ProjectRef{
				{ProjectID: "proj1", ProjectName: "Docs"},
			},
		},
	}

	summary := imp.Summarize(time.Now().UTC())
	require.NotNil(t, summary.Reach)
	assert.Equal(t, 3, summary.Reach.AnnotateBlocks)
	assert.Equal(t, 1, summary.Reach.TransformBlocks)
	assert.Equal(t, 2, summary.Reach.TransformTargets)
	assert.Equal(t, 1, summary.Reach.TransformProjects)

	back := summary.Report()
	require.NotNil(t, back.Reach)
	assert.Equal(t, 3, back.Reach.Annotate.Blocks)
	assert.Equal(t, 5, back.Reach.Annotate.Targets)
	assert.Equal(t, 1, back.Reach.Transform.Blocks)
	assert.Equal(t, 1, back.Reach.Transform.Projects)
	assert.Empty(t, back.Reach.Annotate.Locales, "the summary never carried the locale lists")
	assert.NotNil(t, back.Reach.TransformProjects, "and marshals its project list as an array")
}

// A translation row with no committed runs is a queued target, not a
// translation, so nothing counts it as work a change would invalidate.
func TestBlockTargetLocales_SkipsEmptyAndCollapsesVariants(t *testing.T) {
	b := srcBlock("b1", "g.md", "en-US", "text")
	b.Targets = map[model.VariantKey]*model.Target{
		model.Variant("nb"):                            model.NewTarget([]model.Run{model.TextR("t")}, model.TargetStatusDraft),
		{Locale: "nb", Channel: "email"}:               model.NewTarget([]model.Run{model.TextR("t")}, model.TargetStatusReviewed),
		model.Variant("de"):                            model.NewTarget(nil, model.TargetStatusDraft),
		{Locale: "fr", Tone: "formal"}:                 model.NewTarget([]model.Run{model.TextR("t")}, model.TargetStatusSignedOff),
	}

	locales, approved := blockTargetLocales(b)
	assert.Equal(t, []string{"fr", "nb"}, locales, "de has no runs; nb's two variants are one language")
	assert.Equal(t, []string{"fr", "nb"}, approved, "the highest rung any variant reached decides the language")
}
