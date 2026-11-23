package host

// How each string argument of the MCP tools kapi's host registers with a
// typed input names a location, or that it names none (mcp_args.go). A
// framework tool declares its own through its config schema (path= in a
// field's schema tag). TestEveryMCPPathArgumentIsDeclared fails on an
// argument declared neither way.
func init() {
	DeclareMCPPathArgs("check_text", MCPArgPath, "profile_file")
	DeclareMCPPathArgs("check_text", MCPArgProjectRelative, "context_path")
	DeclareMCPPathArgs("check_file", MCPArgPath, "file", "target", "profile_file")
	DeclareMCPPathArgs("voice_check", MCPArgPath, "profile_file")
	DeclareMCPPathArgs("voice_rewrite", MCPArgPath, "profile_file")
	DeclareMCPPathArgs("stats", MCPArgPath, "files")
	DeclareMCPPathArgs("context_search", MCPArgPath, "terms", "memory")
	DeclareMCPPathArgs("context_read", MCPArgProjectRelative, "path")
	DeclareMCPPathArgs("context_note", MCPArgProjectRelative, "path")
	DeclareMCPPathArgs("read_blocks", MCPArgProjectRelative, "doc")
	DeclareMCPPathArgs("describe_format", MCPArgProjectRelative, "doc")
	// apply_edits names its documents inside each operation: at.doc for an
	// operation on a block or an edition, doc for one on a whole document.
	// A native operation's arguments are described by the format's own
	// schema, which the input schema does not carry, so every string in them
	// is checked as a location. The evidence behind a change may name the
	// file the wording was seen in.
	DeclareMCPPathArgs("apply_edits", MCPArgProjectRelative, "ops.at.doc", "ops.doc", "ops.args", "evidence.path")

	// Wording, patterns, git revisions, pack and profile names, locales,
	// formats, keys and cursors.
	DeclareMCPPathArgs("check_file", MCPArgNotLocation,
		"diff", "diff_against", "diff_range", "dnt", "forbid", "require", "profile_pack", "target_lang", "validate")
	DeclareMCPPathArgs("check_text", MCPArgNotLocation, "text", "forbid", "require", "profile_pack")
	DeclareMCPPathArgs("voice_check", MCPArgNotLocation, "text", "profile_pack")
	DeclareMCPPathArgs("voice_rewrite", MCPArgNotLocation, "text", "profile_pack")
	DeclareMCPPathArgs("context_note", MCPArgNotLocation,
		"quote", "block", "text", "term", "instead_of", "from", "to", "why", "withdraw", "valid_from", "valid_to", "point")
	DeclareMCPPathArgs("context_read", MCPArgNotLocation, "format")
	DeclareMCPPathArgs("context_search", MCPArgNotLocation, "query", "locale")
	DeclareMCPPathArgs("context_session_summary", MCPArgNotLocation, "session")
	DeclareMCPPathArgs("read_blocks", MCPArgNotLocation, "editions", "blocks", "cursor")
	DeclareMCPPathArgs("describe_format", MCPArgNotLocation, "format")
	DeclareMCPPathArgs("stats", MCPArgNotLocation, "format")
	// The rest of a change set: its wording, revisions, block keys, edition
	// keys, review outcomes, term entries and the content model's runs. A
	// recipe field (ops.path) is a dotted name, and a plural or select path
	// (ops.path, ops.anchor.path, ops.edits.path, ops.range.path) a walk into
	// a block's runs; neither is a file.
	DeclareMCPPathArgs("apply_edits", MCPArgNotLocation,
		"schema", "mode", "gate", "note",
		"evidence.block", "evidence.quote", "evidence.url",
		"ops.op", "ops.at.block", "ops.at.edition", "ops.if_match", "ops.basis",
		"ops.text", "ops.runs", "ops.editions", "ops.edits", "ops.range", "ops.path", "ops.anchor",
		"ops.from", "ops.to", "ops.after", "ops.before", "ops.action", "ops.code", "ops.attrs",
		"ops.id", "ops.type", "ops.value", "ops.name", "ops.origin", "ops.outcome", "ops.reasons",
		"ops.term", "ops.locale", "ops.status", "ops.replacement", "ops.replaces")
}
