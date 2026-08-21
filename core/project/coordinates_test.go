package project_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
)

// A project states its brand once and every collection inherits it.
//
// This is the half that decides whether a declared axis is useful or becomes
// per-entry boilerplate. Repeated on every collection it drifts; stated once it
// is a fact about the project.
func TestMergeCoordinates_DefaultsReachEveryCollection(t *testing.T) {
	got := project.MergeCoordinates(
		map[string]string{project.BrandAxis: "acme"},
		map[string]string{project.ProductAxis: "acme-app", project.ChannelAxis: "ui"},
		nil,
	)
	assert.Equal(t, map[string]string{
		project.BrandAxis:   "acme",
		project.ProductAxis: "acme-app",
		project.ChannelAxis: "ui",
	}, got)
}

// A collection that genuinely sits elsewhere moves on that axis alone, and
// keeps everything else it inherited.
func TestMergeCoordinates_ACollectionOverridesOneAxis(t *testing.T) {
	got := project.MergeCoordinates(
		map[string]string{project.BrandAxis: "acme", "market": "eu"},
		map[string]string{project.ProductAxis: "other", project.ChannelAxis: "docs"},
		map[string]string{project.BrandAxis: "beta"},
	)
	assert.Equal(t, map[string]string{
		project.BrandAxis:   "beta",
		"market":            "eu",
		project.ProductAxis: "other",
		project.ChannelAxis: "docs",
	}, got, "the brand moves; the market it inherited does not")
}

// Most specific wins, per axis, in one order.
func TestMergeCoordinates_MostSpecificWins(t *testing.T) {
	got := project.MergeCoordinates(
		map[string]string{"a": "from-defaults", "b": "from-defaults", "c": "from-defaults"},
		map[string]string{"b": "from-channel", "c": "from-channel"},
		map[string]string{"c": "from-collection"},
	)
	assert.Equal(t, map[string]string{
		"a": "from-defaults",
		"b": "from-channel",
		"c": "from-collection",
	}, got)
}

// A collection that claims no point at all sits at the project's default one,
// which the graph treats as a real place rather than an absence.
func TestMergeCoordinates_NothingDeclaredIsTheDefaultPoint(t *testing.T) {
	assert.Nil(t, project.MergeCoordinates(nil, nil, nil),
		"no axes is the default point, and puts no coordinates on the wire")
	assert.Nil(t, project.MergeCoordinates(
		map[string]string{"": "no axis"},
		map[string]string{"axis": ""},
		nil,
	), "an empty axis or an empty value declares nothing")
}

// An axis set to nothing does not erase what a broader layer said. Blanking a
// value is not a way to unset an axis — it is an incomplete edit, and reading
// it as an erasure would let a typo silently move content off its point.
func TestMergeCoordinates_AnEmptyValueDoesNotErase(t *testing.T) {
	got := project.MergeCoordinates(
		map[string]string{project.BrandAxis: "acme"},
		nil,
		map[string]string{project.BrandAxis: ""},
	)
	assert.Equal(t, map[string]string{project.BrandAxis: "acme"}, got)
}
