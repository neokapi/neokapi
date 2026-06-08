package schema

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponentSchema_Validate(t *testing.T) {
	s := &ComponentSchema{
		Properties: map[string]PropertySchema{
			"enabled":   {Type: "boolean"},
			"name":      {Type: "string"},
			"count":     {Type: "integer"},
			"threshold": {Type: "number"},
			"mode":      {Type: "string", Options: []OptionItem{{Value: "fast", Label: "Fast"}, {Value: "slow", Label: "Slow"}}},
		},
	}

	// Valid params
	errs := s.Validate(map[string]any{
		"enabled":   true,
		"name":      "test",
		"count":     42,
		"threshold": 0.5,
		"mode":      "fast",
	})
	assert.Empty(t, errs)

	// Type mismatches
	errs = s.Validate(map[string]any{
		"enabled": "not-a-bool",
		"name":    123,
		"count":   "not-a-number",
	})
	assert.Len(t, errs, 3)

	// Unknown parameter
	errs = s.Validate(map[string]any{
		"unknown": "value",
	})
	require.Len(t, errs, 1)
	assert.Equal(t, "unknown", errs[0].Field)
	assert.Contains(t, errs[0].Message, "unknown parameter")

	// Enum violation
	errs = s.Validate(map[string]any{
		"mode": "invalid",
	})
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Message, "options")
}

func TestComponentSchema_Validate_NilSchema(t *testing.T) {
	var s *ComponentSchema
	errs := s.Validate(map[string]any{"foo": "bar"})
	assert.Empty(t, errs)
}

func TestComponentSchema_JSON_Roundtrip(t *testing.T) {
	s := &ComponentSchema{
		ID:          "pseudo-translate",
		Title:       "Pseudo Translate",
		Description: "Generate pseudo-translations",
		Type:        "object",
		ToolMeta: &ToolMeta{
			ID:          "pseudo-translate",
			Category:    "transform",
			DisplayName: "Pseudo Translate",
			Produces:    []IOPort{{Type: PortTarget, Side: model.SideTarget}},
			Tags:        []string{"translation"},
			Requires:    []string{RequiresTargetLanguage},
		},
		Groups: []ParameterGroup{
			{ID: "output", Label: "Output", Fields: []string{"prefix", "suffix"}},
		},
		Properties: map[string]PropertySchema{
			"expansionPercent": {Type: "integer", Description: "Extra padding", Default: 0},
			"prefix":           {Type: "string", Default: "["},
			"suffix":           {Type: "string", Default: "]"},
		},
	}

	data, err := json.Marshal(s)
	require.NoError(t, err)

	var decoded ComponentSchema
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, s.ID, decoded.ID)
	assert.Equal(t, s.Title, decoded.Title)
	assert.Equal(t, s.ToolMeta.Category, decoded.ToolMeta.Category)
	assert.Equal(t, []IOPort{{Type: PortTarget, Side: model.SideTarget}}, decoded.ToolMeta.Produces)
	assert.Equal(t, []string{"translation"}, decoded.ToolMeta.Tags)
	assert.Equal(t, []string{"target-language"}, decoded.ToolMeta.Requires)
	assert.Len(t, decoded.Properties, 3)
	assert.Equal(t, "integer", decoded.Properties["expansionPercent"].Type)
}

func TestValidationError_Error(t *testing.T) {
	e := ValidationError{Field: "name", Message: "expected string"}
	assert.Equal(t, "name: expected string", e.Error())
}

// ── ConditionExpr JSON Roundtrip Tests ──────────────────────────────────

func TestConditionExpr_Simple_Eq(t *testing.T) {
	expr := &ConditionExpr{Field: "mode", Eq: "advanced"}
	data, err := json.Marshal(expr)
	require.NoError(t, err)

	var decoded ConditionExpr
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "mode", decoded.Field)
	assert.Equal(t, "advanced", decoded.Eq)
}

func TestConditionExpr_Simple_Empty(t *testing.T) {
	boolTrue := true
	expr := &ConditionExpr{Field: "path", Empty: &boolTrue}
	data, err := json.Marshal(expr)
	require.NoError(t, err)

	var decoded ConditionExpr
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "path", decoded.Field)
	require.NotNil(t, decoded.Empty)
	assert.True(t, *decoded.Empty)
}

func TestConditionExpr_And(t *testing.T) {
	expr := &ConditionExpr{
		All: []*ConditionExpr{
			{Field: "mode", Eq: "advanced"},
			{Field: "enabled", Eq: true},
		},
	}
	data, err := json.Marshal(expr)
	require.NoError(t, err)

	var decoded ConditionExpr
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Len(t, decoded.All, 2)
	assert.Equal(t, "mode", decoded.All[0].Field)
	assert.Equal(t, "enabled", decoded.All[1].Field)
}

func TestConditionExpr_Or(t *testing.T) {
	expr := &ConditionExpr{
		Any: []*ConditionExpr{
			{Field: "mode", Eq: "a"},
			{Field: "mode", Eq: "b"},
		},
	}
	data, err := json.Marshal(expr)
	require.NoError(t, err)

	var decoded ConditionExpr
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Len(t, decoded.Any, 2)
}

func TestConditionExpr_Not(t *testing.T) {
	expr := &ConditionExpr{
		Not: &ConditionExpr{Field: "mode", Eq: "simple"},
	}
	data, err := json.Marshal(expr)
	require.NoError(t, err)

	var decoded ConditionExpr
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.NotNil(t, decoded.Not)
	assert.Equal(t, "mode", decoded.Not.Field)
	assert.Equal(t, "simple", decoded.Not.Eq)
}

func TestConditionExpr_Nested_Compound(t *testing.T) {
	// { all: [{ field: "a", eq: true }, { any: [{ field: "b", eq: 1 }, { not: { field: "c", eq: "x" } }] }] }
	expr := &ConditionExpr{
		All: []*ConditionExpr{
			{Field: "a", Eq: true},
			{Any: []*ConditionExpr{
				{Field: "b", Eq: float64(1)},
				{Not: &ConditionExpr{Field: "c", Eq: "x"}},
			}},
		},
	}
	data, err := json.Marshal(expr)
	require.NoError(t, err)

	var decoded ConditionExpr
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Len(t, decoded.All, 2)
	require.Len(t, decoded.All[1].Any, 2)
	require.NotNil(t, decoded.All[1].Any[1].Not)
	assert.Equal(t, "c", decoded.All[1].Any[1].Not.Field)
}

// ── PropertySchema UI Extension Tests ──────────────────────────────────

func TestPropertySchema_UIExtensions_Roundtrip(t *testing.T) {
	order := 3
	prop := PropertySchema{
		Type:             "string",
		Title:            "Output Format",
		Description:      "Choose format",
		Widget:           "segmented",
		Placeholder:      "select...",
		Order:            &order,
		Options:          []OptionItem{{Value: "json", Label: "JSON"}, {Value: "yaml", Label: "YAML"}},
		EnumDescriptions: map[string]string{"json": "Standard JSON"},
		Layout:           &LayoutHints{HideLabel: true, Columns: 2},
		Visible:          &ConditionExpr{Field: "mode", Eq: "advanced"},
		Enabled:          &ConditionExpr{Not: &ConditionExpr{Field: "locked", Eq: true}},
	}

	data, err := json.Marshal(prop)
	require.NoError(t, err)

	// Verify ui: prefix in JSON
	raw := string(data)
	assert.Contains(t, raw, `"ui:widget"`)
	assert.Contains(t, raw, `"ui:placeholder"`)
	assert.Contains(t, raw, `"ui:order"`)
	assert.Contains(t, raw, `"options"`)
	assert.Contains(t, raw, `"ui:enum-descriptions"`)
	assert.Contains(t, raw, `"ui:layout"`)
	assert.Contains(t, raw, `"ui:visible"`)
	assert.Contains(t, raw, `"ui:enabled"`)

	// Roundtrip
	var decoded PropertySchema
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "segmented", decoded.Widget)
	assert.Equal(t, "select...", decoded.Placeholder)
	require.NotNil(t, decoded.Order)
	assert.Equal(t, 3, *decoded.Order)
	require.Len(t, decoded.Options, 2)
	assert.Equal(t, "JSON", decoded.Options[0].Label)
	require.NotNil(t, decoded.Layout)
	assert.True(t, decoded.Layout.HideLabel)
	assert.Equal(t, 2, decoded.Layout.Columns)
	require.NotNil(t, decoded.Visible)
	assert.Equal(t, "mode", decoded.Visible.Field)
	require.NotNil(t, decoded.Enabled)
	require.NotNil(t, decoded.Enabled.Not)
}

// ── ParameterGroup Tests ───────────────────────────────────────────────

func TestParameterGroup_Collapsible_Roundtrip(t *testing.T) {
	boolTrue := true
	g := ParameterGroup{
		ID:          "advanced",
		Label:       "Advanced Settings",
		Description: "Fine-tune behavior",
		Collapsible: &boolTrue,
		Collapsed:   true,
		Icon:        "settings",
		Fields:      []string{"a", "b", "c"},
	}

	data, err := json.Marshal(g)
	require.NoError(t, err)

	var decoded ParameterGroup
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "advanced", decoded.ID)
	assert.Equal(t, "Fine-tune behavior", decoded.Description)
	require.NotNil(t, decoded.Collapsible)
	assert.True(t, *decoded.Collapsible)
	assert.True(t, decoded.Collapsed)
	assert.Equal(t, "settings", decoded.Icon)
	assert.Equal(t, []string{"a", "b", "c"}, decoded.Fields)
}

// ── LayoutHints Tests ──────────────────────────────────────────────────

func TestLayoutHints_Roundtrip(t *testing.T) {
	h := &LayoutHints{HideLabel: true, Vertical: true, Columns: 3}
	data, err := json.Marshal(h)
	require.NoError(t, err)

	var decoded LayoutHints
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.True(t, decoded.HideLabel)
	assert.True(t, decoded.Vertical)
	assert.Equal(t, 3, decoded.Columns)
}

// ── ToolMeta Tests ─────────────────────────────────────────────────────

func TestToolMeta_JSON_Tag(t *testing.T) {
	s := &ComponentSchema{
		Title: "Test",
		Type:  "object",
		ToolMeta: &ToolMeta{
			ID:       "test-tool",
			Category: "validate",
			Requires: []string{"target-language"},
		},
	}
	data, err := json.Marshal(s)
	require.NoError(t, err)

	raw := string(data)
	assert.Contains(t, raw, `"toolMeta"`)
	assert.NotContains(t, raw, `"x-component"`)

	var decoded ComponentSchema
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.NotNil(t, decoded.ToolMeta)
	assert.Equal(t, "test-tool", decoded.ToolMeta.ID)
	assert.Equal(t, "validate", decoded.ToolMeta.Category)
}

func TestComponentSchema_UIGroups_JSON_Tag(t *testing.T) {
	s := &ComponentSchema{
		Title: "Test",
		Type:  "object",
		Groups: []ParameterGroup{
			{ID: "g1", Label: "Group 1", Fields: []string{"a"}},
		},
	}
	data, err := json.Marshal(s)
	require.NoError(t, err)

	raw := string(data)
	assert.Contains(t, raw, `"ui:groups"`)
	assert.NotContains(t, raw, `"x-groups"`)
}
