package store_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// A target's revision is the edition revision of its content: the token a
// project file gives the same content, moved by a change to the target's
// wording or codes and by nothing else.
func TestTargetRevision_IsTheTargetsContentRevision(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.SourceLocale = "en"
	b.SetTarget("fr", &model.Target{Runs: []model.Run{model.TextR("Bonjour")}, Status: model.TargetStatusDraft})
	sb := &venue.StoredBlock{Block: b, ContentHash: model.ComputeContentHash("Hello")}
	rev := store.TargetRevision(sb, "fr")

	assert.Equal(t, model.EditionRevision(b, model.Variant("fr")), rev, "one token for the same content wherever it is held")
	assert.Equal(t, model.AbsentRevision, store.TargetRevision(sb, "de"))
	assert.Equal(t, model.AbsentRevision, store.TargetRevision(nil, "fr"))

	b.Target("fr").Status = model.TargetStatusEstablished
	assert.Equal(t, rev, store.TargetRevision(sb, "fr"), "a review decision does not move it")

	b.SetSourceText("Hello there")
	sb.ContentHash = model.ComputeContentHash("Hello there")
	assert.Equal(t, rev, store.TargetRevision(sb, "fr"), "a source edit does not refuse a translator's save")

	b.SetTargetText("fr", "Salut")
	assert.NotEqual(t, rev, store.TargetRevision(sb, "fr"), "a change to the wording moves it")
}
