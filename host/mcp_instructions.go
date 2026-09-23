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
// It points at context_read rather than the resource. The server offers the
// resource only as a template, and a client lists no resources from a
// template, so a model working from its tool list reaches the answer through
// the tool, which returns the same text.
func MCPInstructions() string {
	return "This project's writing rules are kept by kapi. Before you change a file, call context_read " +
		"with its project-relative path. It gives the voice and the words to use there, or says nothing " +
		"is recorded yet.\n\n" +
		"While you read, record the names and spellings the project keeps to with context_observe. When the " +
		"person changes your wording, record it with context_correct. If you recorded something wrongly, " +
		"take it back with context_withdraw. A person decides what becomes a rule.\n\n" +
		"Before you finish, run check_file on each file you changed, fix what it reports, and end with what " +
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
