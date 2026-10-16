package project

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/axis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const axesRecipe = `version: v1
name: market-app
defaults:
  source_language: en
  target_languages: [nb-NO, de-DE]
  coordinates:
    market: nordics
    audience: general
axes:
  market:
    kind: geo
    label: Market
    groups:
      nordics: [no, se, dk, fi, is]
  audience:
    values: [general, new-customers, specialists]
    labels:
      new-customers: New customers
  age:
    kind: range
    unit: years
  lifecycle:
    kind: ordered
    values: [preview, current, sunset]
collections:
  - name: teens
    coordinates:
      age: 13-17
      market: "no"
    content:
      - path: teens/**/*.md
`

func TestAxes_LoadAndType(t *testing.T) {
	p := loadRecipe(t, axesRecipe)
	require.Len(t, p.Axes, 4)
	assert.Equal(t, axis.KindGeo, p.Axes["market"].Kind)
	assert.Equal(t, []string{"no", "se", "dk", "fi", "is"}, p.Axes["market"].Groups["nordics"])
	assert.True(t, p.AxisSpace().Contains("market", "nordics", "no"))
	assert.Equal(t, []string{"age", "audience", "lifecycle", "market"}, p.DeclaredAxisNames())
}

// A recipe that types nothing loads as it did: every axis is an open
// category, whatever its values.
func TestAxes_UntypedRecipeLoadsUnchanged(t *testing.T) {
	p := loadRecipe(t, `version: v1
name: plain
defaults:
  coordinates:
    brand: Northsea Ltd
    market: anything at all
`)
	assert.Nil(t, p.Axes)
	assert.Equal(t, []string{"brand", "market"}, p.DeclaredAxisNames())
}

func TestAxes_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		recipe  string
		wantErr string
	}{
		{
			name:    "structural axis declared",
			recipe:  "version: v1\nname: x\naxes:\n  channel: {}\n",
			wantErr: `axes.channel: "channel" is derived from a collection's channel`,
		},
		{
			name:    "unknown kind",
			recipe:  "version: v1\nname: x\naxes:\n  market: {kind: map}\n",
			wantErr: `axes.market.kind: "map" is not an axis kind`,
		},
		{
			name:    "default outside a closed axis",
			recipe:  "version: v1\nname: x\naxes:\n  audience: {values: [general]}\ndefaults:\n  coordinates:\n    audience: children\n",
			wantErr: `defaults.coordinates.audience: axis audience: "children" is not one of its values (general)`,
		},
		{
			name:    "collection geo value unknown",
			recipe:  "version: v1\nname: x\naxes:\n  market: {kind: geo}\ncollections:\n  - name: docs\n    coordinates: {market: atlantis}\n    content:\n      - path: docs/*.md\n",
			wantErr: `collections[0]: collection "docs": coordinates.market: axis market: "atlantis" is not a country code`,
		},
		{
			name:    "range inverted",
			recipe:  "version: v1\nname: x\naxes:\n  age: {kind: range}\ndefaults:\n  coordinates: {age: 17-13}\n",
			wantErr: "lower bound is above its upper bound",
		},
		{
			name:    "geo group names an unknown code",
			recipe:  "version: v1\nname: x\naxes:\n  market:\n    kind: geo\n    groups: {nordics: [no, zz]}\n",
			wantErr: `axes.market.groups.nordics: "zz" is not a country code`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p KapiProject
			require.NoError(t, yaml.Unmarshal([]byte(tt.recipe), &p))
			err := p.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestAxes_SetFieldValidatesTheValue(t *testing.T) {
	p := loadRecipe(t, axesRecipe)
	_, err := SetField(p, "defaults.coordinates.audience", json.RawMessage(`"children"`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"children" is not one of its values`)

	changed, err := SetField(p, "defaults.coordinates.audience", json.RawMessage(`"specialists"`))
	require.NoError(t, err)
	assert.True(t, changed)
}

func TestAxes_RoundTrip(t *testing.T) {
	p := loadRecipe(t, axesRecipe)
	out, err := yaml.Marshal(p)
	require.NoError(t, err)
	var back KapiProject
	require.NoError(t, yaml.Unmarshal(out, &back))
	require.NoError(t, back.Validate())
	assert.Equal(t, p.Axes, back.Axes)
}
