package host

import "fmt"

// The verbs `kapi context` no longer has.
//
// `kapi context <path>` takes a path that need not exist yet: a missing file
// whose folder exists is a file about to be written. A verb that moved
// elsewhere would therefore be read as a path, and `kapi context import
// voice.yaml` would answer for a file called `import` instead of failing. So a
// bare retired verb is refused with the command that does the job now. A file
// that really is called `import` is still answered for as `./import`.

// retiredContextVerb is where one retired verb went: the command a person or
// an assistant on the command line runs now, and the MCP tool that does the
// same, empty when no tool does.
type retiredContextVerb struct {
	cli string
	mcp string
}

var retiredContextVerbs = map[string]retiredContextVerb{
	"import":   {cli: "kapi store import"},
	"export":   {cli: "kapi store export"},
	"rebuild":  {cli: "kapi store rebuild"},
	"locales":  {cli: "kapi store locales"},
	"observe":  {cli: "kapi context note", mcp: "context_note"},
	"correct":  {cli: "kapi context note --from <yours> --to <theirs>", mcp: "context_note with from and to"},
	"withdraw": {cli: "kapi context note --withdraw <id>", mcp: "context_note with withdraw"},
	"keep":     {cli: "kapi context review --keep <id>"},
	"drop":     {cli: "kapi context review --drop <id>"},
	"widen":    {cli: "kapi context review --keep <id> --widen-to <where>"},
	"digest":   {cli: "kapi context review"},
	"revert":   {cli: "kapi context reset --before <session|date|id>"},
	"pull":     {cli: "kapi context sync --no-push"},
	"push":     {cli: "kapi context sync"},
	"settle":   {cli: "kapi context sync --merged <range>"},
	"backend":  {cli: "kapi context sync --status"},
}

// RetiredContextVerbError reports the error for a `kapi context` argument that
// names a retired verb, or nil when the argument is a path. Only the bare verb
// is refused: `./import` and `docs/import` are paths.
func RetiredContextVerbError(arg string) error {
	v, ok := retiredContextVerbs[arg]
	if !ok {
		return nil
	}
	return WithExitCode(ExitUsage, fmt.Errorf("kapi context %s moved: use %s (a file named %q is ./%s)", arg, v.cli, arg, arg))
}

// retiredContextReadError is RetiredContextVerbError for the context_read
// tool, naming the tool that does the job when there is one.
func retiredContextReadError(path string) error {
	v, ok := retiredContextVerbs[path]
	if !ok {
		return nil
	}
	if v.mcp != "" {
		return fmt.Errorf("context_read: %q is not a file; context %s moved to the %s tool (a file named %q is ./%s)", path, path, v.mcp, path, path)
	}
	return fmt.Errorf("context_read: %q is not a file; kapi context %s moved to %s, which a person runs (a file named %q is ./%s)", path, path, v.cli, path, path)
}
