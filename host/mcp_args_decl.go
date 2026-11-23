package host

// The location arguments of the MCP tools kapi's host registers with a typed
// input. A framework tool declares its own through x-path annotations on its
// config schema. TestEveryMCPPathArgumentIsDeclared fails on one missing here.
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
	DeclareMCPPathArgs("apply_edits", MCPArgProjectRelative, "doc")
	DeclareMCPPathArgs("read_blocks", MCPArgProjectRelative, "doc")
	DeclareMCPPathArgs("describe_format", MCPArgProjectRelative, "doc")

	// Arguments whose description mentions a file and that name none: git
	// revisions, pack and profile names, a locale, a format, quoted wording.
	DeclareMCPPathArgs("check_file", MCPArgNotLocation, "diff_against", "diff_range", "profile_pack", "target_lang")
	DeclareMCPPathArgs("check_text", MCPArgNotLocation, "profile_pack")
	DeclareMCPPathArgs("voice_check", MCPArgNotLocation, "profile_pack")
	DeclareMCPPathArgs("voice_rewrite", MCPArgNotLocation, "profile_pack")
	DeclareMCPPathArgs("context_note", MCPArgNotLocation, "quote")
	DeclareMCPPathArgs("read_blocks", MCPArgNotLocation, "editions")
	DeclareMCPPathArgs("stats", MCPArgNotLocation, "format")
	DeclareMCPPathArgs("voice-check", MCPArgNotLocation, "profile_id")
	DeclareMCPPathArgs("voice-infer", MCPArgNotLocation, "profile_name")
}
