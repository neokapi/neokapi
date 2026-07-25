package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/model"
)

// targetPendingReview is the review queue's presence predicate, and it asked
// "is there content here?" through TargetText(), the flattening that drops inline
// code. A target whose only run is a placeholder flattened to "" and read as
// absent, so the unit never entered the review queue: no reviewer could see it,
// no one could approve it, and — because the loop's completion condition is
// block-derived from this same predicate — the project read as "nothing left to
// review" while it sat there.
func TestTargetPendingReview_PlaceholderOnlyTargetIsPending(t *testing.T) {
	phRun := model.Run{Ph: &model.PlaceholderRun{
		ID: "1", Type: "jsx:element", Data: "{p.price}", Equiv: "p.price",
	}}

	b := &model.Block{ID: "price", Translatable: true, Source: []model.Run{phRun}}
	b.SetTargetRuns("fr", []model.Run{phRun})
	assert.True(t, targetPendingReview(b, "fr"),
		"a placeholder-only target is produced content and must reach the review queue")

	// A reviewed target is not pending, placeholder-only or not.
	b.Target("fr").Status = model.TargetStatusReviewed
	assert.False(t, targetPendingReview(b, "fr"))

	// The boundary holds: no target, and a whitespace-only target, are not pending.
	assert.False(t, targetPendingReview(&model.Block{ID: "x", Translatable: true}, "fr"))
	ws := &model.Block{ID: "w", Translatable: true}
	ws.SetSourceText("Hello")
	ws.SetTargetText("fr", "   ")
	assert.False(t, targetPendingReview(ws, "fr"))
}
