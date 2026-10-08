package host

// What the server says about itself before any tool is called.
//
// MCP delivers an `instructions` string to every client at initialize, ahead
// of the tool list and whether or not the host loads a skill. It is the only
// text a client with no skill support ever reads about kapi, so it carries the
// habits the skill leads with: follow the rules the project's rules files
// hold, ask for the full answer only where they say nothing, record the names
// and corrections met while working, and check what changed before reporting
// the work done.
//
// It names only what the server serves. A sentence about a verb that does not
// exist here would be the most expensive kind of wrong, because a model acts
// on it before it can find out. So the text is for the writing set, and a
// server that does not serve that set sends none.

// MCPInstructions is the server's introduction when it serves the writing
// set, delivered to every client on initialize.
//
// The rules files come first. kapi writes the rules that hold in each folder
// into AGENTS.md and CLAUDE.md, which a host loads into every session by
// itself, so an agent told to call context_read before every change pays a
// round trip, and the tool schemas it loads, for an answer it already holds.
// context_read is for a project or folder with no rules section, and for the
// full answer (the whole voice, every rule past a capped list).
//
// It names the edit tools, and says when they are needed: a host that defers
// tool schemas loads a tool by the name it has read, and an agent told to edit
// every file through them loads the largest schemas the server has for a
// Markdown change its own editor makes.
//
// It points at context_read rather than the resource. The server offers the
// resource only as a template, and a client lists no resources from a
// template, so a model working from its tool list reaches the answer through
// the tool, which returns the same text.
func MCPInstructions() string {
	return "Follow the rules in kapi's section of AGENTS.md and CLAUDE.md. Only for a file no section covers, " +
		"or for the full answer, call context_read with its path: it says what applies, which old names stay " +
		"correct, or that nothing is recorded yet.\n\n" +
		"For Word, XLIFF and other files your tools cannot edit safely, use read_blocks and apply_edits; " +
		"describe_format says what a format accepts.\n\n" +
		"Record with context_note what the files keep to and the rules omit, even where your text does not need " +
		"it: a name, the spelling variety, a word preferred over a common one, with its path. Leave alone a word " +
		"the files write two ways. Record the person's edits with from, to, path; withdraw takes back a mistake. " +
		"A person decides what becomes a rule.\n\n" +
		"Finally, run check_file on each changed file; fix what it reports. " +
		"After a note, end with context_session_summary."
}

// MCPInstructionsFor is the introduction for a server serving the given tool
// sets: MCPInstructions when the writing set is among them, and nothing
// otherwise. A nil selection serves every set.
func MCPInstructionsFor(sets map[string]bool) string {
	if sets != nil && !sets[MCPSetWriting] {
		return ""
	}
	return MCPInstructions()
}
