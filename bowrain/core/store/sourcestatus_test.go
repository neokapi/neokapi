package store

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
)

func TestPropsForStore_RoundTrip(t *testing.T) {
	b := &model.Block{SourceStatus: model.SourceStatusChecked, Properties: map[string]string{"k": "v"}}
	props := PropsForStore(b)
	assert.Equal(t, "v", props["k"], "existing properties are preserved")
	assert.Equal(t, "checked", props[PropSourceStatus], "the status is folded in")
	// The caller's own map is never mutated.
	_, leaked := b.Properties[PropSourceStatus]
	assert.False(t, leaked, "copy-on-write: the block's map is untouched")

	// Read side: lift it back onto the block and strip the reserved key.
	scanned := &model.Block{Properties: map[string]string{"k": "v", PropSourceStatus: "approved"}}
	ApplySourceStatusFromProps(scanned)
	assert.Equal(t, model.SourceStatusApproved, scanned.SourceStatus)
	_, stillThere := scanned.Properties[PropSourceStatus]
	assert.False(t, stillThere, "the reserved key is stripped on read")
	assert.Equal(t, "v", scanned.Properties["k"])
}

func TestPropsForStore_NoStatus(t *testing.T) {
	b := &model.Block{Properties: map[string]string{"k": "v"}}
	// No status → properties returned unchanged (same map).
	assert.Equal(t, b.Properties, PropsForStore(b))
}

func TestSourceGateFor(t *testing.T) {
	assert.Equal(t, model.SourceGateChecked, SourceGateFor(nil), "nil project → default")
	assert.Equal(t, model.SourceGateChecked, SourceGateFor(&Project{}), "unset → default")
	assert.Equal(t, model.SourceGateNone,
		SourceGateFor(&Project{Properties: map[string]string{SourceGateProperty: "none"}}))
	assert.Equal(t, model.SourceGateApproved,
		SourceGateFor(&Project{Properties: map[string]string{SourceGateProperty: "approved"}}))
}
