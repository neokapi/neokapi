package registry

import (
	"fmt"

	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// ToolGroupMember defines one selectable backend of a tool group: a
// self-describing member (a segmentation engine, an AI/MT provider, a QA mode)
// with its own parameter schema. Members are never merged into one another —
// the group selects exactly one at a time.
type ToolGroupMember struct {
	Name        string // discriminator value that selects this member ("srx")
	Label       string // human label for the selector
	Description string // selector help text

	// Schema is the member's own parameter schema (nil = no parameters). Built
	// with schema.FromStruct over the member's config struct, or loaded from a
	// plugin manifest.
	Schema *schema.ComponentSchema

	// When overrides the visibility condition for this member's config when the
	// group is projected to a flat schema — for a member whose config is shared
	// across several discriminator values (e.g. entity-extract's LLM params apply
	// to both "llm" and "hybrid"). Defaults to discriminator == Name.
	When *schema.ConditionExpr

	// Factory, when set, builds this member's tool directly instead of going
	// through the group's ConfigFactory. This is the seam for pluggable members:
	// a member contributed at runtime (e.g. by a plugin via AddGroupMember)
	// carries its own factory, while the built-in members leave it nil and are
	// served by the group's ConfigFactory. The group's stored factory dispatches
	// on the discriminator: a member with a Factory wins; otherwise the group
	// ConfigFactory handles it.
	Factory ToolConfigFactory
}

// ToolGroupDef defines a tool group for registration. A group is a tool whose
// behaviour is provided by one of several members selected at runtime by the
// Discriminator field; the group owns Common config + a Default member, and each
// member owns its config schema.
//
// One registry entry is created (so flat consumers — CLI, docs, MCP — keep
// working via the composed schema), but it carries the member metadata and
// per-member schemas that group-aware consumers (the config UI) render
// master-detail.
type ToolGroupDef struct {
	Name          ToolID                  // group id ("segmentation")
	Discriminator string                  // selector field present in Common ("engine")
	Default       string                  // default member name
	Common        *schema.ComponentSchema // common config incl the discriminator field + ToolMeta
	Members       []ToolGroupMember
	ConfigFactory ToolConfigFactory                                   // builds the tool from the unified config map
	Resolver      func(config map[string]any, base ToolInfo) ToolInfo // optional IO-contract refinement per member
}

// ComposeGroupSchema returns a group's flat projection — the composed schema
// (Common + a discriminator select + per-member variant groups) that flat
// consumers (CLI flags, docs, MCP) and the registry's Schema(id) use. It is the
// single composition path, shared by RegisterGroup and any tool that exposes its
// own flat-schema accessor.
func ComposeGroupSchema(def ToolGroupDef) *schema.ComponentSchema {
	variants := make([]schema.Variant, 0, len(def.Members))
	for _, m := range def.Members {
		variants = append(variants, schema.Variant{
			Name: m.Name, Label: m.Label, Description: m.Description, Params: m.Schema, When: m.When,
		})
	}
	return schema.ComposeVariants(def.Common, def.Discriminator, def.Default, variants)
}

// RegisterGroup registers a tool group. It composes the flat projection
// (see ComposeGroupSchema) for CLI/docs/MCP and the registry's Schema(id),
// stores each member's own schema for MemberSchema, and records the group
// metadata on ToolInfo.Group. The registered ConfigFactory dispatches on the
// discriminator so members with their own Factory are served directly and the
// rest fall through to def.ConfigFactory.
func (r *ToolRegistry) RegisterGroup(def ToolGroupDef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.storeGroup(def)
}

// AddGroupMember appends (or replaces, by name) a member of an already-registered
// group and recomposes everything derived from the member set — the flat schema,
// MemberSchema, the Group metadata, and the dispatching ConfigFactory. This is
// how a runtime source (e.g. a plugin) contributes a member to a built-in group
// without the group's own package knowing about it. The member should carry a
// Factory (a member with no Factory relies on the group's ConfigFactory
// recognizing its discriminator value). Returns an error if the tool is not a
// registered group.
func (r *ToolRegistry) AddGroupMember(group ToolID, m ToolGroupMember) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	reg, ok := r.tools[group]
	if !ok || reg.GroupDef == nil {
		return fmt.Errorf("tool %q is not a registered group", group)
	}
	def := *reg.GroupDef
	members := append([]ToolGroupMember(nil), def.Members...)
	replaced := false
	for i := range members {
		if members[i].Name == m.Name {
			members[i] = m
			replaced = true
			break
		}
	}
	if !replaced {
		members = append(members, m)
	}
	def.Members = members
	r.storeGroup(def)
	return nil
}

// storeGroup composes a group definition and stores it as a ToolRegistration.
// Shared by RegisterGroup and AddGroupMember. The caller must hold r.mu.
func (r *ToolRegistry) storeGroup(def ToolGroupDef) {
	memberSchemas := make(map[string]*schema.ComponentSchema, len(def.Members))
	memberFactories := make(map[string]ToolConfigFactory)
	members := make([]ToolGroupMemberInfo, 0, len(def.Members))
	for _, m := range def.Members {
		memberSchemas[m.Name] = m.Schema
		if m.Factory != nil {
			memberFactories[m.Name] = m.Factory
		}
		members = append(members, ToolGroupMemberInfo{
			Name: m.Name, Label: m.Label, Description: m.Description, HasSchema: m.Schema != nil,
		})
	}
	composed := ComposeGroupSchema(def)

	// Dispatching factory: a member's own Factory wins; otherwise the group's
	// ConfigFactory handles the discriminator value.
	groupFactory := def.ConfigFactory
	discriminator := def.Discriminator
	groupName := def.Name
	dispatch := func(config map[string]any, targetLang string) (tool.Tool, error) {
		value, _ := config[discriminator].(string)
		if f, ok := memberFactories[value]; ok {
			return f(config, targetLang)
		}
		if groupFactory != nil {
			return groupFactory(config, targetLang)
		}
		return nil, fmt.Errorf("tool group %q: no factory for %s=%q", groupName, discriminator, value)
	}

	info := ToolInfo{
		Name:      def.Name,
		Source:    SourceBuiltIn,
		HasSchema: true,
		Group: &ToolGroupInfo{
			Discriminator: def.Discriminator,
			Default:       def.Default,
			Members:       members,
		},
	}
	info.DisplayName = composed.Title
	info.Description = composed.Description
	if composed.ToolMeta != nil {
		copyToolMeta(&info, composed.ToolMeta)
	}

	defCopy := def
	r.tools[def.Name] = &ToolRegistration{
		Schema:           composed,
		ConfigFactory:    dispatch,
		ContractResolver: def.Resolver,
		MemberSchemas:    memberSchemas,
		Info:             info,
		GroupDef:         &defCopy,
	}
}

// MemberSchema returns the own schema of a group member (the master-detail
// section the UI renders for the selected member), or nil when the tool is not a
// group, the member is unknown, or the member takes no parameters.
func (r *ToolRegistry) MemberSchema(group ToolID, member string) *schema.ComponentSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reg, ok := r.tools[group]
	if !ok || reg.MemberSchemas == nil {
		return nil
	}
	return reg.MemberSchemas[member]
}
