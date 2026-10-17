package axis_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/axis"
	"github.com/neokapi/neokapi/core/place"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func testPlaces(t *testing.T) *place.Gazetteer {
	t.Helper()
	g, err := place.New(map[string]place.Place{
		"nordics":     {Name: "Nordics", Of: []string{"no", "se", "dk", "fi", "is"}},
		"scandinavia": {Name: "Scandinavia", Of: []string{"no", "se", "dk"}},
		"north":       {Name: "The north", Of: []string{"nordics", "gb"}},
		"oslo":        {Name: "Oslo", Type: place.TypeLocality, Within: []string{"no-03"}},
	})
	require.NoError(t, err)
	return g
}

func testSpace(t *testing.T) axis.Space {
	return axis.NewSpace(map[string]axis.Spec{
		"market":    {Kind: axis.KindGeo, Label: "Market"},
		"audience":  {Values: []string{"general", "new-customers", "specialists"}, Labels: map[string]string{"general": "Everyone"}},
		"age":       {Kind: axis.KindRange, Unit: "years"},
		"lifecycle": {Kind: axis.KindOrdered, Values: []string{"preview", "current", "sunset"}},
	}, testPlaces(t))
}

func TestSpace_Contains(t *testing.T) {
	s := testSpace(t)
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
		{"geo country in a made-of place", "market", "nordics", "no", true},
		{"geo country outside a made-of place", "market", "nordics", "de", false},
		{"geo country in subregion", "market", "northern-europe", "no", true},
		{"geo country in world", "market", "world", "no", true},
		{"geo subdivision in country", "market", "no", "no-03", true},
		{"geo declared locality in region", "market", "europe", "oslo", true},
		{"geo declared locality in a made-of place", "market", "nordics", "oslo", true},
		{"geo country does not contain its region", "market", "no", "northern-europe", false},
		{"geo made-of place in region", "market", "europe", "nordics", true},
		{"geo made-of place within a wider one", "market", "nordics", "scandinavia", true},
		{"geo made-of place not within a narrower one", "market", "scandinavia", "nordics", false},
		{"geo nested made-of place holds members of members", "market", "north", "fi", true},
		{"geo an unknown place holds only itself", "market", "atlantis", "atlantis", true},
		{"geo world holds no unknown place", "market", "world", "atlantis", false},
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
	s := testSpace(t)
	assert.True(t, s.Overlaps("market", "nordics", "northern-europe"), "the two share Norway")
	assert.True(t, s.Overlaps("market", "no", "nordics"))
	assert.True(t, s.Overlaps("market", "scandinavia", "oslo"))
	assert.False(t, s.Overlaps("market", "nordics", "western-europe"))
	assert.False(t, s.Overlaps("market", "no-03", "no-46"))
	assert.True(t, s.Overlaps("age", "13-17", "16-20"))
	assert.False(t, s.Overlaps("age", "13-17", "18-"))
	assert.True(t, s.Overlaps("brand", "a", "a"))
	assert.False(t, s.Overlaps("brand", "a", "b"))
}

func TestSpace_Validate(t *testing.T) {
	s := testSpace(t)
	tests := []struct {
		axis, value string
		wantErr     string
	}{
		{"brand", "anything", ""},
		{"audience", "general", ""},
		{"audience", "children", "is not one of its values (general, new-customers, specialists)"},
		{"lifecycle", "retired", "is not one of its values"},
		{"market", "no", ""},
		{"market", "no-03", ""},
		{"market", "northern-europe", ""},
		{"market", "world", ""},
		{"market", "nordics", ""},
		{"market", "oslo", ""},
		{"market", "NO", "Write it in lower case: no"},
		{"market", "no03", "Write it with a hyphen: no-03"},
		{"market", "atlantis", "`kapi context places atlantis`"},
		{"market", "Vestland", "Did you mean no-46 (Vestland, Norway)?"},
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
	require.NoError(t, testSpace(t).Check("axes"))
	tests := []struct {
		name    string
		axes    map[string]axis.Spec
		wantErr string
	}{
		{"name not a slug", map[string]axis.Spec{"Market": {}}, `axis name "Market" is not a slug`},
		{"unknown kind", map[string]axis.Spec{"m": {Kind: "map"}}, `axes.m.kind: "map" is not an axis kind`},
		{"geo with values", map[string]axis.Spec{"m": {Kind: axis.KindGeo, Values: []string{"no"}}}, "takes no declared values"},
		{"geo with labels", map[string]axis.Spec{"m": {Kind: axis.KindGeo, Labels: map[string]string{"no": "Norge"}}}, "axes.m.labels: a geo axis takes no labels; name a place under `places:`"},
		{"range with values", map[string]axis.Spec{"a": {Kind: axis.KindRange, Values: []string{"13-17"}}}, "takes no declared values"},
		{"value not a slug", map[string]axis.Spec{"a": {Values: []string{"New Customers"}}}, "is not a slug"},
		{"value twice", map[string]axis.Spec{"a": {Values: []string{"x", "x"}}}, "declared twice"},
		{"ordered without values", map[string]axis.Spec{"l": {Kind: axis.KindOrdered}}, "declares its values in order"},
		{"unit off range", map[string]axis.Spec{"a": {Unit: "years"}}, "only a range axis takes a unit"},
		{"label for undeclared value", map[string]axis.Spec{"a": {Values: []string{"x"}, Labels: map[string]string{"y": "Y"}}}, `"y" is not one of its values`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := axis.NewSpace(tt.axes, nil).Check("axes")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestSpace_Label(t *testing.T) {
	s := testSpace(t)
	assert.Equal(t, "Everyone", s.Label("audience", "general"))
	assert.Equal(t, "Norway", s.Label("market", "no"))
	assert.Equal(t, "Oslo", s.Label("market", "no-03"))
	assert.Equal(t, "Nordics", s.Label("market", "nordics"))
	assert.Equal(t, "Northern Europe", s.Label("market", "northern-europe"))
	assert.Empty(t, s.Label("brand", "northsea"))
}

func TestSpace_Zero(t *testing.T) {
	var s axis.Space
	assert.Same(t, place.Builtin(), s.Places(), "the zero space reads the built-in places")
	assert.Equal(t, axis.Spec{}, s.Spec("market"))
	assert.False(t, s.Typed("market"))
	assert.Empty(t, s.Names())
	assert.True(t, s.Contains("market", "no", "no"))
	assert.False(t, s.Contains("market", "europe", "no"), "an undeclared axis compares by equality")
	assert.NoError(t, s.Validate("market", "anything"))
	assert.NoError(t, s.Check("axes"))

	typed := axis.NewSpace(map[string]axis.Spec{"market": {Kind: axis.KindGeo}, "age": {Kind: axis.KindRange}}, nil)
	assert.Equal(t, []string{"age", "market"}, typed.Names())
	assert.True(t, typed.Contains("market", "europe", "no-03"), "a nil gazetteer is the built-in one")
}

func TestSpec_YAML(t *testing.T) {
	var spec axis.Spec
	require.NoError(t, yaml.Unmarshal([]byte("kind: range\nlabel: Age\nunit: years\n"), &spec))
	assert.Equal(t, axis.Spec{Kind: axis.KindRange, Label: "Age", Unit: "years"}, spec)
	err := yaml.Unmarshal([]byte("kind: geo\nlable: Market\n"), &spec)
	assert.ErrorContains(t, err, `an axis takes no key "lable" (it takes kind, label, values, labels, unit)`)
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
