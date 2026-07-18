package jobs

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
)

func blk(id string, translatable bool, status model.SourceStatus) *store.StoredBlock {
	b := &model.Block{ID: id, Translatable: translatable, SourceStatus: status}
	b.SetSourceText("text")
	return &store.StoredBlock{Block: b}
}

// TestGateBlocksBySource: the worker translates only blocks whose source clears
// the gate; a partially-settled item keeps its ready blocks and drops the rest.
func TestGateBlocksBySource(t *testing.T) {
	blocks := []*store.StoredBlock{
		blk("checked", true, model.SourceStatusChecked),
		blk("approved", true, model.SourceStatusApproved),
		blk("authored", true, model.SourceStatusAuthored),
		blk("new", true, model.SourceStatusNew),
		blk("nontrans", false, model.SourceStatusNew), // no source to gate → passes
	}

	// checked gate: a committed authored status is held; checked/approved,
	// non-translatable, AND never-settled (New) blocks pass through — the worker
	// is not the settle phase and must not strand unstamped blocks.
	kept := gateBlocksBySource(blocks, model.SourceGateChecked)
	got := map[string]bool{}
	for _, sb := range kept {
		got[sb.Block.ID] = true
	}
	assert.True(t, got["checked"])
	assert.True(t, got["approved"])
	assert.True(t, got["nontrans"])
	assert.True(t, got["new"], "never-settled (New) blocks pass through the worker gate")
	assert.False(t, got["authored"], "a committed authored status is below the checked gate")

	// none gate: every block passes (opt-out), slice returned unchanged.
	all := gateBlocksBySource(blocks, model.SourceGateNone)
	assert.Len(t, all, len(blocks))
}

// TestProjectDNTTerms parses the comma-separated do-not-translate list from
// project settings.
func TestProjectDNTTerms(t *testing.T) {
	assert.Nil(t, ProjectDNTTerms(&store.Project{}))
	assert.Equal(t, []string{"Kapi", "Bowrain"},
		ProjectDNTTerms(&store.Project{Properties: map[string]string{"dnt_terms": " Kapi , Bowrain , "}}))
}
