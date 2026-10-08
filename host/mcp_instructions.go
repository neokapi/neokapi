package host

// What the server says about itself before any tool is called.
//
// MCP delivers an `instructions` string to every client at initialize, ahead
// of the tool list and whether or not the host loads a skill. It is the only
// text a client with no skill support ever reads about kapi, so it carries the
// habits the skill leads with: read what applies before writing, record the
// names and corrections met while working, and check what changed before
// reporting the work done.
//
// It names only what the server serves. A sentence about a verb that does not
// exist here would be the most expensive kind of wrong, because a model acts
// on it before it can find out. So the text is for the writing set, and a
// server that does not serve that set sends none.

// MCPInstructions is the server's introduction when it serves the writing
// set, delivered to every client on initialize.
//
// It names the edit tools: a host that defers tool schemas loads a tool by
// the name it has read, and the instructions are what it reads first.
//
// It points at context_read rather than the resource. The server offers the
// resource only as a template, and a client lists no resources from a
// template, so a model working from its tool list reaches the answer through
// the tool, which returns the same text.
func MCPInstructions() string {
	return "This project's writing rules are kept by kapi. Before you change a file, call context_read " +
		"with its project-relative path; it says what applies, or that nothing is recorded yet.\n\n" +
		"To change text inside a file, read it with read_blocks and send the change to apply_edits; " +
		"describe_format says what a format accepts.\n\n" +
		"As you read, record with context_note what the files do every time, even where your text " +
		"does not need it: product, feature and plan names, the spelling variety, a word chosen over a " +
		"common alternative. Leave alone a word the files write two ways. Record the person's edits " +
		"with from, to, path; take back a mistake with withdraw. A person decides what becomes a rule.\n\n" +
		"Before finishing, run check_file on each changed file, fix what it reports, and end with what " +
		"context_session_summary says."
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
