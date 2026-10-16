package axis_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/axis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testSpace() axis.Space {
	return axis.Space{
		"market": {
			Kind:   axis.KindGeo,
			Label:  "Market",
			Groups: map[string][]string{"nordics": {"no", "se", "dk", "fi", "is"}, "scandinavia": {"no", "se", "dk"}, "north": {"nordics", "gb"}},
		},
		"audience":  {Values: []string{"general", "new-customers", "specialists"}},
		"age":       {Kind: axis.KindRange, Unit: "years"},
		"lifecycle": {Kind: axis.KindOrdered, Values: []string{"preview", "current", "sunset"}},
	}
}

func TestSpace_Contains(t *testing.T) {
	s := testSpace()
	tests := []struct {
		name               string
		axis, outer, inner string
		want               bool
	}{
		{"open category is equality", "brand", "northsea", "northsea", true},
		{"open category differs", "brand", "northsea", "acme", false},
		{"closed category is equality", "audience", "general", "general", true},
		{"closed category differs", "audience", "general", "specialists", false},
		{"ordered is equality", "lifecycle", "current", "current", true},
		{"ordered never contains another value", "lifecycle", "current", "preview", false},
		{"geo country in group", "market", "nordics", "no", true},
		{"geo country outside group", "market", "nordics", "de", false},
		{"geo country in subregion", "market", "northern-europe", "no", true},
		{"geo country in region", "market", "europe", "no", true},
		{"geo country in world", "market", "world", "no", true},
		{"geo subregion in region", "market", "europe", "northern-europe", true},
		{"geo intermediate region chain", "market", "africa", "ke", true},
		{"geo region chain through latin america", "market", "americas", "br", true},
		{"geo region does not contain a sibling", "market", "western-europe", "no", false},
		{"geo country does not contain its region", "market", "no", "northern-europe", false},
		{"geo group in region", "market", "europe", "nordics", true},
		{"geo group within group", "market", "nordics", "scandinavia", true},
		{"geo group not within a narrower group", "market", "scandinavia", "nordics", false},
		{"geo nested group holds members of members", "market", "north", "fi", true},
		{"geo nested group holds its own members", "market", "north", "gb", true},
		{"geo group in world", "market", "world", "nordics", true},
		{"range inside", "age", "13-17", "14-16", true},
		{"range equal", "age", "13-17", "13-17", true},
		{"range outside", "age", "13-17", "18-", false},
		{"range open upper contains bounded", "age", "13-", "13-17", true},
		{"range bounded does not contain open", "age", "13-17", "13-", false},
		{"range single number", "age", "13-17", "15", true},
		{"empty never contains", "brand", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, s.Contains(tt.axis, tt.outer, tt.inner))
		})
	}
}

func TestSpace_Overlaps(t *testing.T) {
	s := testSpace()
	assert.True(t, s.Overlaps("market", "nordics", "northern-europe"), "the two share Norway")
	assert.True(t, s.Overlaps("market", "no", "nordics"))
	assert.False(t, s.Overlaps("market", "nordics", "western-europe"))
	assert.True(t, s.Overlaps("age", "13-17", "16-20"))
	assert.False(t, s.Overlaps("age", "13-17", "18-"))
	assert.True(t, s.Overlaps("brand", "a", "a"))
	assert.False(t, s.Overlaps("brand", "a", "b"))
}

func TestSpace_Validate(t *testing.T) {
	s := testSpace()
	tests := []struct {
		axis, value string
		wantErr     string
	}{
		{"brand", "anything", ""},
		{"audience", "general", ""},
		{"audience", "children", "is not one of its values (general, new-customers, specialists)"},
		{"lifecycle", "retired", "is not one of its values"},
		{"market", "no", ""},
		{"market", "northern-europe", ""},
		{"market", "world", ""},
		{"market", "nordics", ""},
		{"market", "NO", "Write it in lower case: no"},
		{"market", "atlantis", "ISO 3166-1 alpha-2"},
		{"age", "13-17", ""},
		{"age", "65-", ""},
		{"age", "18", ""},
		{"age", "17-13", "lower bound is above its upper bound"},
		{"age", "teen", "is not a number"},
		{"brand", "", "empty value"},
	}
	for _, tt := range tests {
		t.Run(tt.axis+"="+tt.value, func(t *testing.T) {
			err := s.Validate(tt.axis, tt.value)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestSpace_Check(t *testing.T) {
	require.NoError(t, testSpace().Check("axes"))
	tests := []struct {
		name    string
		space   axis.Space
		wantErr string
	}{
		{"name not a slug", axis.Space{"Market": {}}, `axis name "Market" is not a slug`},
		{"unknown kind", axis.Space{"m": {Kind: "map"}}, `axes.m.kind: "map" is not an axis kind`},
		{"geo with values", axis.Space{"m": {Kind: axis.KindGeo, Values: []string{"no"}}}, "takes no declared values"},
		{"value not a slug", axis.Space{"a": {Values: []string{"New Customers"}}}, "is not a slug"},
		{"value twice", axis.Space{"a": {Values: []string{"x", "x"}}}, "declared twice"},
		{"ordered without values", axis.Space{"l": {Kind: axis.KindOrdered}}, "declares its values in order"},
		{"groups off geo", axis.Space{"a": {Groups: map[string][]string{"g": {"x"}}}}, "only a geo axis takes groups"},
		{"unit off range", axis.Space{"a": {Unit: "years"}}, "only a range axis takes a unit"},
		{"label for undeclared value", axis.Space{"a": {Values: []string{"x"}, Labels: map[string]string{"y": "Y"}}}, `"y" is not one of its values`},
		{"group member unknown", axis.Space{"m": {Kind: axis.KindGeo, Groups: map[string][]string{"g": {"no", "xx"}}}}, `"xx" is not a country code`},
		{"group shadows region", axis.Space{"m": {Kind: axis.KindGeo, Groups: map[string][]string{"europe": {"no"}}}}, "already a country or region"},
		{"empty group", axis.Space{"m": {Kind: axis.KindGeo, Groups: map[string][]string{"g": {}}}}, "at least one"},
		{"group cycle", axis.Space{"m": {Kind: axis.KindGeo, Groups: map[string][]string{"a": {"b"}, "b": {"a"}}}}, "contains itself"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.space.Check("axes")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestSpace_LoopingGroupsEnd(t *testing.T) {
	// A space nobody checked can name its groups in a loop; containment ends.
	s := axis.Space{"m": {Kind: axis.KindGeo, Groups: map[string][]string{"a": {"b"}, "b": {"a"}}}}
	assert.False(t, s.Contains("m", "a", "no"))
	assert.False(t, s.Contains("m", "world", "a"))
}

func TestSpace_Label(t *testing.T) {
	s := testSpace()
	s["audience"] = axis.Spec{Values: []string{"general"}, Labels: map[string]string{"general": "Everyone"}}
	assert.Equal(t, "Everyone", s.Label("audience", "general"))
	assert.Equal(t, "Norway", s.Label("market", "no"))
	assert.Equal(t, "Northern Europe", s.Label("market", "northern-europe"))
	assert.Empty(t, s.Label("brand", "northsea"))
}

func TestParseRange(t *testing.T) {
	r, err := axis.ParseRange("13-17")
	require.NoError(t, err)
	assert.Equal(t, axis.Range{Min: 13, Max: 17}, r)
	r, err = axis.ParseRange("65-")
	require.NoError(t, err)
	assert.True(t, r.Open())
	_, err = axis.ParseRange("-12")
	require.Error(t, err)
	_, err = axis.ParseRange("")
	assert.Error(t, err)
}

func TestSpec_Closed(t *testing.T) {
	assert.False(t, axis.Spec{}.Closed())
	assert.True(t, axis.Spec{Values: []string{"a"}}.Closed())
	assert.True(t, axis.Spec{Kind: axis.KindOrdered, Values: []string{"a"}}.Closed())
	assert.False(t, axis.Spec{Kind: axis.KindRange}.Closed())
	assert.Equal(t, axis.KindCategory, axis.Spec{}.EffectiveKind())
}
