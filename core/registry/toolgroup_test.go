package registry

import (
	"testing"

	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterGroup(t *testing.T) {
	reg := NewToolRegistry()

	common := &schema.ComponentSchema{
		ID:    "seg",
		Title: "Segmentation",
		Type:  "object",
		ToolMeta: &schema.ToolMeta{
			ID:       "segmentation",
			Category: schema.CategoryTextProcessing,
		},
		Properties: map[string]schema.PropertySchema{
			"engine": {Type: "string", Title: "Engine"},
			"scope":  {Type: "boolean", Title: "Scope"},
		},
		Groups: []schema.ParameterGroup{{ID: "common", Label: "Common", Fields: []string{"engine", "scope"}}},
	}
	srxParams := &schema.ComponentSchema{
		Type:       "object",
		Properties: map[string]schema.PropertySchema{"rulesPath": {Type: "string", Title: "Rules"}},
	}

	built := ""
	reg.RegisterGroup(ToolGroupDef{
		Name:          "segmentation",
		Discriminator: "engine",
		Default:       "srx",
		Common:        common,
		Members: []ToolGroupMember{
			{Name: "srx", Label: "Default", Description: "rule-based", Schema: srxParams},
			{Name: "uax29", Label: "Unicode"}, // no params
		},
		ConfigFactory: func(config map[string]any, _ string) (tool.Tool, error) {
			built, _ = config["engine"].(string)
			return nil, nil
		},
	})

	// ToolInfo carries group metadata.
	info := reg.ToolInfo("segmentation")
	require.NotNil(t, info)
	require.NotNil(t, info.Group)
	assert.Equal(t, "engine", info.Group.Discriminator)
	assert.Equal(t, "srx", info.Group.Default)
	require.Len(t, info.Group.Members, 2)
	assert.Equal(t, "srx", info.Group.Members[0].Name)
	assert.True(t, info.Group.Members[0].HasSchema)
	assert.False(t, info.Group.Members[1].HasSchema)
	assert.Equal(t, schema.CategoryTextProcessing, info.Category)

	// The composed (flat) schema is the registry's Schema — discriminator select
	// + member field gated.
	s := reg.Schema("segmentation")
	require.NotNil(t, s)
	eng := s.Properties["engine"]
	assert.Equal(t, "select", eng.Widget)
	assert.Equal(t, "srx", eng.Default)
	require.Contains(t, s.Properties, "rulesPath")

	// Per-member schema is the master-detail section the UI renders.
	ms := reg.MemberSchema("segmentation", "srx")
	require.NotNil(t, ms)
	assert.Contains(t, ms.Properties, "rulesPath")
	assert.Nil(t, reg.MemberSchema("segmentation", "uax29"), "parameterless member has no schema")
	assert.Nil(t, reg.MemberSchema("segmentation", "nope"), "unknown member")

	// CLITools sees the group (it has a schema + config factory).
	require.Len(t, reg.CLITools(), 1)

	// The config factory dispatches via the discriminator.
	_, err := reg.NewToolWithConfig("segmentation", map[string]any{"engine": "uax29"}, "")
	require.NoError(t, err)
	assert.Equal(t, "uax29", built)
}
