package host

// What the server says about itself before any tool is called.
//
// MCP delivers an `instructions` string to every client at initialize, ahead
// of the tool list and whether or not the host loads a skill. It is the only
// text a client with no skill support ever reads about kapi, so it carries the
// same four habits the skill leads with: ask what applies before writing,
// record what you notice while you read, record the person's corrections, and
// check what you changed before reporting the work done.
//
// It names only what the server serves. A sentence about a verb that does not
// exist here would be the most expensive kind of wrong, because a model acts
// on it before it can find out.

// MCPInstructions is the server's own introduction, delivered to every client
// on initialize.
func MCPInstructions() string {
	return `kapi holds this project's content context: the voice in force at a location, the terms bound there, and wording it has already approved. Most projects have recorded little of it, and what you notice while you work is how it grows.

Before writing or editing prose here, read ` + "`context://<project-relative-path>`" + ` for the file you are about to change, and call context_search to ask what a word is called here. An empty answer is an answer: this project has recorded nothing at that point, and the answer says what is worth noticing while you work. Both list the candidates nobody has decided on, marked as candidates: build on them, report none as a rule in force.

While you read, call context_observe on what you notice about how the project writes, such as a product name as it spells it. Where it is consistent about a word and nothing records that, call context_propose with the file you saw it in. When the person changes your wording, call context_correct with both wordings and the place. One call records one thing and asks you nothing. What you record advises until a person confirms it, so none of it can fail a check.

After saving a change, call check_file on each file you changed and fix what it reports before you say the work is done. Then call context_session_summary and end your report with what it says.

A project's context lives in the person's workspace, and the files a checkout carries under ` + "`.kapi/`" + ` are artifacts of it. An answer carrying a ` + "`notice`" + ` names files this checkout holds that nothing has read in, which is why it is empty; ` + "`kapi context import`" + ` reads them, and a person runs it. Tell them, and work from what the store holds meanwhile.

Every project-scoped tool takes an optional project argument, and the context resource takes ?project=: the project's kapi.yaml, its root directory, or any path inside it. Every answer says which project answered, the workspace revision it was read at, and whether what kapi holds still matches the files on disk.`
}
