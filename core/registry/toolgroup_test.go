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

// fakeTool is a minimal tool.Tool for asserting member-factory dispatch.
type fakeTool struct{ tool.BaseTool }

func TestAddGroupMember(t *testing.T) {
	reg := NewToolRegistry()

	groupBuilt := ""
	reg.RegisterGroup(ToolGroupDef{
		Name:          "segmentation",
		Discriminator: "engine",
		Default:       "srx",
		Common: &schema.ComponentSchema{
			Type:       "object",
			Title:      "Segmentation",
			Properties: map[string]schema.PropertySchema{"engine": {Type: "string"}},
			Groups:     []schema.ParameterGroup{{ID: "common", Label: "Common", Fields: []string{"engine"}}},
		},
		Members: []ToolGroupMember{{Name: "srx", Label: "SRX"}},
		ConfigFactory: func(config map[string]any, _ string) (tool.Tool, error) {
			groupBuilt, _ = config["engine"].(string)
			return nil, nil
		},
	})

	// A plugin contributes a member with its own factory + schema.
	memberTool := &fakeTool{}
	memberBuilt := false
	err := reg.AddGroupMember("segmentation", ToolGroupMember{
		Name:        "sat",
		Label:       "SaT (ML)",
		Description: "wtpsplit ML segmenter",
		Schema: &schema.ComponentSchema{
			Type:       "object",
			Properties: map[string]schema.PropertySchema{"threshold": {Type: "number"}},
		},
		Factory: func(config map[string]any, _ string) (tool.Tool, error) {
			memberBuilt = true
			return memberTool, nil
		},
	})
	require.NoError(t, err)

	// Group metadata + member schema now include the contributed member.
	info := reg.ToolInfo("segmentation")
	require.Len(t, info.Group.Members, 2)
	assert.Equal(t, "sat", info.Group.Members[1].Name)
	assert.True(t, info.Group.Members[1].HasSchema)
	require.NotNil(t, reg.MemberSchema("segmentation", "sat"))
	assert.Contains(t, reg.MemberSchema("segmentation", "sat").Properties, "threshold")

	// The flat schema recomposed: engine selector now offers the new member, and
	// its gated params appear.
	s := reg.Schema("segmentation")
	var hasSat bool
	for _, o := range s.Properties["engine"].Options {
		if o.Value == "sat" {
			hasSat = true
		}
	}
	assert.True(t, hasSat, "recomposed selector offers the plugin member")
	assert.Contains(t, s.Properties, "threshold")

	// Dispatch: the member's own factory wins for its discriminator value...
	got, err := reg.NewToolWithConfig("segmentation", map[string]any{"engine": "sat"}, "")
	require.NoError(t, err)
	assert.True(t, memberBuilt, "member factory invoked")
	assert.Equal(t, tool.Tool(memberTool), got)

	// ...while built-in members still fall through to the group factory.
	_, err = reg.NewToolWithConfig("segmentation", map[string]any{"engine": "srx"}, "")
	require.NoError(t, err)
	assert.Equal(t, "srx", groupBuilt)

	// Adding a member to a non-group is an error.
	reg.RegisterWithSchema("plain", func() tool.Tool { return &fakeTool{} }, &schema.ComponentSchema{Type: "object"})
	require.Error(t, reg.AddGroupMember("plain", ToolGroupMember{Name: "x"}))
	require.Error(t, reg.AddGroupMember("missing", ToolGroupMember{Name: "x"}))
}

// TestAddGroupMember_ReplacesByName verifies re-adding a member name replaces it.
func TestAddGroupMember_ReplacesByName(t *testing.T) {
	reg := NewToolRegistry()
	reg.RegisterGroup(ToolGroupDef{
		Name:          "g",
		Discriminator: "engine",
		Default:       "a",
		Common: &schema.ComponentSchema{
			Type:       "object",
			Title:      "G",
			Properties: map[string]schema.PropertySchema{"engine": {Type: "string"}},
			Groups:     []schema.ParameterGroup{{ID: "c", Label: "C", Fields: []string{"engine"}}},
		},
		Members:       []ToolGroupMember{{Name: "a", Label: "A"}},
		ConfigFactory: func(map[string]any, string) (tool.Tool, error) { return nil, nil },
	})

	first := &fakeTool{}
	second := &fakeTool{}
	require.NoError(t, reg.AddGroupMember("g", ToolGroupMember{Name: "p", Label: "First", Factory: func(map[string]any, string) (tool.Tool, error) { return first, nil }}))
	require.NoError(t, reg.AddGroupMember("g", ToolGroupMember{Name: "p", Label: "Second", Factory: func(map[string]any, string) (tool.Tool, error) { return second, nil }}))

	info := reg.ToolInfo("g")
	require.Len(t, info.Group.Members, 2, "replaced, not appended")
	assert.Equal(t, "Second", info.Group.Members[1].Label)

	got, err := reg.NewToolWithConfig("g", map[string]any{"engine": "p"}, "")
	require.NoError(t, err)
	assert.Equal(t, tool.Tool(second), got)
}

// groupCommon builds a minimal valid Common schema for a discriminated group.
func groupCommon(id string) *schema.ComponentSchema {
	return &schema.ComponentSchema{
		ID:    id,
		Title: id,
		Type:  "object",
		ToolMeta: &schema.ToolMeta{
			ID:       id,
			Category: schema.CategoryTextProcessing,
		},
		Properties: map[string]schema.PropertySchema{"engine": {Type: "string", Title: "Engine"}},
		Groups:     []schema.ParameterGroup{{ID: "common", Label: "Common", Fields: []string{"engine"}}},
	}
}

// TestRegisterGroup_NewToolBuildsDefaultMember locks the contract that a group is
// instantiable via the flat NewTool(name): it must build the group's Default
// member, so flat consumers that instantiate by name — the CLI flow runner, the
// bowrain gRPC flow builder, and the desktop/server tool listings — keep working.
// Regression for the bowrain TestListTools + flow-build failures that appeared
// once segmentation moved from a flat RegisterWithSchema to a tool group.
func TestRegisterGroup_NewToolBuildsDefaultMember(t *testing.T) {
	reg := NewToolRegistry()
	gotEngine := ""
	reg.RegisterGroup(ToolGroupDef{
		Name:          "segmentation",
		Discriminator: "engine",
		Default:       "srx",
		Common:        groupCommon("segmentation"),
		Members:       []ToolGroupMember{{Name: "srx", Label: "Default"}, {Name: "uax29", Label: "Unicode"}},
		ConfigFactory: func(config map[string]any, _ string) (tool.Tool, error) {
			gotEngine, _ = config["engine"].(string)
			return &fakeTool{}, nil
		},
	})

	// The group is listed by name (Names backs the desktop/server tool listings)...
	assert.Contains(t, reg.Names(), ToolID("segmentation"))
	// ...and NewTool(name) builds its Default member instead of erroring.
	tl, err := reg.NewTool("segmentation")
	require.NoError(t, err)
	require.NotNil(t, tl)
	assert.Equal(t, "srx", gotEngine, "NewTool must instantiate the group's Default member")
}

// TestRegisterGroup_NewToolDefaultFails surfaces an error rather than a (nil,nil)
// result when a group's default member cannot be built locally, so callers never
// receive a nil tool with a nil error.
func TestRegisterGroup_NewToolDefaultFails(t *testing.T) {
	reg := NewToolRegistry()
	reg.RegisterGroup(ToolGroupDef{
		Name:          "broken",
		Discriminator: "engine",
		Default:       "missing",
		Common:        groupCommon("broken"),
		Members:       []ToolGroupMember{{Name: "missing"}},
		ConfigFactory: func(map[string]any, string) (tool.Tool, error) {
			return nil, assert.AnError
		},
	})
	tl, err := reg.NewTool("broken")
	require.Error(t, err)
	assert.Nil(t, tl)
}
