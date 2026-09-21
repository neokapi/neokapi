package host

// What the server says about itself before any tool is called.
//
// MCP delivers an `instructions` string to every client at initialize, ahead
// of the tool list and whether or not the host loads a skill. It is the only
// text a client with no skill support ever reads about kapi, so it says the
// two things that change what an assistant does: ask what applies before
// writing, and run the check before reporting the work done.
//
// It names only what the server serves. A sentence about a verb that does not
// exist here would be the most expensive kind of wrong, because a model acts
// on it before it can find out.

// MCPInstructions is the server's own introduction, delivered to every client
// on initialize.
func MCPInstructions() string {
	return `kapi holds this project's content context: the voice profile in force at a location, the terms bound there, and wording the project has already approved.

Before writing or editing prose in this project, read the context resource for the file you are about to change: ` + "`context://<project-relative-path>`" + `. Call context_search to ask what a word or phrase is called here. An empty answer is an answer: it says this project has recorded nothing at that point, and it says what is worth noticing while you work.

After saving a change, call check_file on each file you changed, and fix what it reports before you say the work is done. It reports against the context in force at each file's own location.

Every project-scoped tool takes an optional project argument, and the context resource takes ?project=, naming the project's kapi.yaml, its root directory, or any path inside it. Every answer says which project it came from, the workspace revision it was read at, and whether the content kapi holds still matches the files on disk.`
}
