package host

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/host/pluginhost"
)

// The directories a plugin daemon may be sent paths in.
//
// A run declares them where it begins, from what a person chose: the root of
// the project the run acts on, and the directories named for this invocation
// (explicit inputs and their -o/--out-dir outputs). The format client sends a
// daemon a path only beneath them, and the content inline otherwise
// (pluginhost.EnvPluginRoots).

// declarePathRoots adds dirs to the roots cmd's context declares, for the
// rest of the command, and returns that context.
func declarePathRoots(cmd Command, dirs ...string) context.Context {
	ctx := pluginhost.AddPathRoots(CmdContext(cmd), dirs...)
	if cmd != nil {
		cmd.SetContext(ctx)
	}
	return ctx
}

// projectRootDir is the root of the project whose recipe is at projectPath:
// the directory that holds the recipe. Empty for no project.
func projectRootDir(projectPath string) string {
	if projectPath == "" {
		return ""
	}
	abs, err := filepath.Abs(projectPath)
	if err != nil {
		return ""
	}
	return filepath.Dir(abs)
}

// namedInputRoots returns the roots to declare for the inputs a person named,
// as they typed them: the project root of each, or else the directory it
// names or the directory that holds the file. A glob names the directory
// before its first wildcard, a `container!entry` locator names its archive,
// and standard input names no directory.
func namedInputRoots(inputs []string) []string {
	paths := make([]string, 0, len(inputs))
	for _, in := range inputs {
		if in == "" || in == StdinName {
			continue
		}
		if loc, ok := parseEntryLocator(in); ok {
			in = loc.Archive
		} else if looksLikeGlobArg(in) {
			in, _ = splitGlobRoot(in)
		}
		paths = append(paths, in)
	}
	return pluginhost.PathRootsFor(paths...)
}

// outputRoot returns the directory an -o/--output value names for a run's
// outputs: the directory itself for a directory-style value, the directory
// before the first template token or wildcard for a template, and the
// directory that holds the file otherwise. Empty when the value names none of
// its own (no value, standard output, or a template that starts from the
// input's own directory).
func outputRoot(output string) string {
	if output == "" || output == StdinName {
		return ""
	}
	if i := strings.IndexAny(output, "{*?["); i >= 0 {
		prefix := output[:i]
		if prefix == "" {
			return ""
		}
		if strings.HasSuffix(prefix, "/") || strings.HasSuffix(prefix, string(filepath.Separator)) {
			return filepath.Clean(prefix)
		}
		return filepath.Dir(prefix)
	}
	if isDirTemplate(output) {
		return filepath.Clean(output)
	}
	return filepath.Dir(output)
}
