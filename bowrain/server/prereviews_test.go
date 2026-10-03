package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// advise is a pre-review of a block's translation in locale, on revision rev.
func advise(item, bid, locale, rev string, score *int, reasons ...string) change.Op {
	return change.Op{Kind: change.KindDecide,
		At:      change.Ref{Doc: item, Block: bid, Edition: model.EditionKey{Locale: model.LocaleID(locale)}},
		IfMatch: rev,
		Body:    &change.Decide{Outcome: change.OutcomeAdvise, Score: score, Reasons: reasons}}
}

// An agent pre-reviews a translation through the server MCP's change service.
// The advice moves no status; the review context and the review queue show it
// beside the translation while the translation stands at the revision the
// agent read. Once a person rewrites the translation, neither shows it.
func TestPreReview_AnAgentsAdviceShowsBesideTheTranslationItJudged(t *testing.T) {
	s, wsID, owner := newRecheckHarness(t)
	ctx := t.Context()
	projID, byText := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("a", "Open the app", "Ouvrir l'application"),
		pendingFrBlock("b", "Close the app", "Fermer l'application"),
	})
	judged, other := byText["Open the app"], byText["Close the app"]

	svc, _, err := s.mcpChangeService(ctx, owner, projID, "main")
	require.NoError(t, err)
	sb, err := s.ContentStore.GetBlock(ctx, projID, "main", judged)
	require.NoError(t, err)
	read := platstore.TargetRevision(sb, "fr")

	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{
		advise("greetings.txt", judged, "fr", read, new(72), "reads as machine output"),
	}}, change.Actor{Kind: change.ActorAgent, Name: "claude-code"})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Equal(t, change.OpApplied, res.Ops[0].Status)

	sb, err = s.ContentStore.GetBlock(ctx, projID, "main", judged)
	require.NoError(t, err)
	assert.Equal(t, model.TargetStatusDraft, sb.Block.Target("fr").Status, "a pre-review moves no status")

	_, got := getReviewContext(t, s, wsID, projID, judged, "fr")
	require.NotNil(t, got.Judgement.AIScore, "the review context shows the advice")
	assert.Equal(t, 72, *got.Judgement.AIScore)
	assert.Equal(t, "agent/claude-code", got.Judgement.AIModel)
	require.Len(t, got.Judgement.AIFindings, 1)
	assert.Equal(t, "reads as machine output", got.Judgement.AIFindings[0].Message)

	queue := listPendingReview(t, s, wsID, projID)
	entry := entryFor(t, queue, judged)
	require.NotNil(t, entry.PreReview, "the queue shows the advice")
	assert.Equal(t, 72, entry.PreReview.Score)
	assert.Equal(t, "agent/claude-code", entry.PreReview.Reviewer)
	assert.Equal(t, []string{"reads as machine output"}, entry.PreReview.Reasons)
	assert.Nil(t, entryFor(t, queue, other).PreReview, "a translation nobody advised on carries none")

	// A person rewrites the translation: the advice judged other wording.
	text := "Lancer l'application"
	res, err = svc.Apply(ctx, change.Set{Ops: []change.Op{{Kind: change.KindSetContent,
		At:      change.Ref{Doc: "greetings.txt", Block: judged, Edition: model.EditionKey{Locale: "fr"}},
		IfMatch: read, Body: &change.SetContent{Text: &text}}}},
		change.Actor{Kind: change.ActorPerson, Name: owner})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	_, got = getReviewContext(t, s, wsID, projID, judged, "fr")
	assert.Nil(t, got.Judgement.AIScore, "advice on wording that changed is not shown")
	assert.Nil(t, entryFor(t, listPendingReview(t, s, wsID, projID), judged).PreReview)
}

// A pre-review carries a score and judges a translation that exists, and an
// agent records nothing but a pre-review.
func TestPreReview_IsRefusedWithoutAScoreOrATranslation(t *testing.T) {
	s, wsID, owner := newRecheckHarness(t)
	ctx := t.Context()
	untranslated := &model.Block{ID: "c", Translatable: true}
	untranslated.SetSourceText("Sign in")
	projID, byText := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("a", "Open the app", "Ouvrir l'application"),
		untranslated,
	})
	svc, _, err := s.mcpChangeService(ctx, owner, projID, "main")
	require.NoError(t, err)
	agent := change.Actor{Kind: change.ActorAgent, Name: "claude-code"}
	sb, err := s.ContentStore.GetBlock(ctx, projID, "main", byText["Open the app"])
	require.NoError(t, err)
	read := platstore.TargetRevision(sb, "fr")

	cases := []struct {
		name string
		op   change.Op
		code change.Code
	}{
		{"advice with no score", advise("greetings.txt", byText["Open the app"], "fr", read, nil), change.CodeInvalid},
		{"advice on a translation that does not exist", advise("greetings.txt", byText["Sign in"], "fr", model.AbsentRevision, new(50)), change.CodeNotFound},
		{"an agent's approval", change.Op{Kind: change.KindDecide,
			At:      change.Ref{Doc: "greetings.txt", Block: byText["Open the app"], Edition: model.EditionKey{Locale: "fr"}},
			IfMatch: read, Body: &change.Decide{Outcome: change.OutcomeEstablish}}, change.CodeNotPermitted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{tc.op}}, agent)
			require.NoError(t, err)
			require.Equal(t, change.SetRefused, res.Status, "%+v", res.Ops)
			require.NotNil(t, res.Ops[0].Error, "%+v", res.Ops)
			assert.Equal(t, tc.code, res.Ops[0].Error.Code, res.Ops[0].Error.Message)
		})
	}
	reviews, err := s.ContentStore.PreReviews(ctx, projID, "main", []string{byText["Open the app"], byText["Sign in"]})
	require.NoError(t, err)
	assert.Empty(t, reviews, "a refused change set records nothing")
}
