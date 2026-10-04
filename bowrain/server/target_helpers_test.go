package server

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// targetStatusOf reads the status of the target b holds in loc, failing the
// test when b holds none.
func targetStatusOf(t *testing.T, b *model.Block, loc model.LocaleID) model.TargetStatus {
	t.Helper()
	e, ok := b.TargetEdition(loc)
	require.True(t, ok, "block %s holds no %s target", b.ID, loc)
	return model.TargetStatus(e.Status)
}

// holdsTarget reports whether b holds a target in loc.
func holdsTarget(b *model.Block, loc model.LocaleID) bool {
	_, ok := b.TargetEdition(loc)
	return ok
}

// setTargetOrigin records how the target b holds in loc was produced, failing
// the test when b holds none.
func setTargetOrigin(t *testing.T, b *model.Block, loc model.LocaleID, o model.Origin) {
	t.Helper()
	e, ok := b.TargetEdition(loc)
	require.True(t, ok, "block %s holds no %s target", b.ID, loc)
	e.Origin = o
	b.SetTargetEdition(model.Variant(loc), e)
}
