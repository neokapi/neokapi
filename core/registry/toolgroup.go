package registry

import "github.com/neokapi/neokapi/core/schema"

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

// RegisterGroup registers a tool group. It composes the flat projection
// (Common + a discriminator select + per-member variant groups) for CLI/docs/MCP
// and the registry's Schema(id), stores each member's own schema for
// MemberSchema, and records the group metadata on ToolInfo.Group.
func (r *ToolRegistry) RegisterGroup(def ToolGroupDef) {
	variants := make([]schema.Variant, 0, len(def.Members))
	memberSchemas := make(map[string]*schema.ComponentSchema, len(def.Members))
	members := make([]ToolGroupMemberInfo, 0, len(def.Members))
	for _, m := range def.Members {
		variants = append(variants, schema.Variant{
			Name: m.Name, Label: m.Label, Description: m.Description, Params: m.Schema, When: m.When,
		})
		memberSchemas[m.Name] = m.Schema
		members = append(members, ToolGroupMemberInfo{
			Name: m.Name, Label: m.Label, Description: m.Description, HasSchema: m.Schema != nil,
		})
	}
	composed := schema.ComposeVariants(def.Common, def.Discriminator, def.Default, variants)

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

	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[def.Name] = &ToolRegistration{
		Schema:           composed,
		ConfigFactory:    def.ConfigFactory,
		ContractResolver: def.Resolver,
		MemberSchemas:    memberSchemas,
		Info:             info,
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
