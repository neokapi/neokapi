package host

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host/pluginhost"
)

// commentProviderFor returns the provider that reads path's comments: the one an
// installed plugin declares for the file's extension, else the built-in
// provider for it. An installed plugin takes precedence over a built-in, as it
// does for a format. A plugin's provider is recorded in the ledger ctx carries,
// so the evaluation record of the operation names the plugin that located the
// comments. A context carrying no ledger records nothing.
func (a *App) commentProviderFor(ctx context.Context, path string) (comment.Provider, bool) {
	p, route := a.lookupCommentProvider(path)
	if route != nil {
		pluginLedgerFrom(ctx).served(route.Plugin.Name(), route.Plugin.Version(), "comments:"+route.Language.Language)
	}
	return p, p != nil
}

// lookupCommentProvider is the lookup behind commentProviderFor, recording
// nothing: for a caller that asks whether a file is a comment document, or
// edits one, rather than checking it. route is the installed plugin's route
// when a plugin's provider answers, and nil for a built-in provider.
func (a *App) lookupCommentProvider(path string) (comment.Provider, *pluginhost.CommentRoute) {
	if a.PluginHost != nil {
		ext := strings.ToLower(filepath.Ext(path))
		for _, r := range a.PluginHost.CommentRoutes() {
			if slices.ContainsFunc(r.Language.Extensions, func(e string) bool { return strings.ToLower(e) == ext }) {
				return pluginhost.CommentProvider(a.DaemonPool(), r), r
			}
		}
	}
	p, ok := commentProviders.For(path)
	if !ok {
		return nil, nil
	}
	return p, nil
}

// hasCommentProvider reports whether any provider, a plugin's or a built-in,
// reads path's comments. It records nothing: asking whether a plugin would
// read a file is not a plugin serving a read.
func (a *App) hasCommentProvider(path string) bool {
	p, _ := a.lookupCommentProvider(path)
	return p != nil
}

// commentPluginHint is what kapi knows, with no plugin installed, about a
// language whose comments a plugin reads: its extensions, its name and the
// plugin to install. A check over such a file names that plugin. Nothing is
// dispatched from here: an installed plugin's manifest decides what it reads,
// and TestCommentPluginHintsMatchTheManifest pins this table to the
// sourcecode plugin's manifest.
type commentPluginHint struct {
	Plugin      string
	Language    string
	DisplayName string
	Extensions  []string
}

var commentPluginHints = []commentPluginHint{
	{Plugin: "sourcecode", Language: "bash", DisplayName: "Bash", Extensions: []string{".sh", ".bash"}},
	{Plugin: "sourcecode", Language: "c", DisplayName: "C", Extensions: []string{".c", ".h"}},
	{Plugin: "sourcecode", Language: "cpp", DisplayName: "C++", Extensions: []string{".cpp", ".cc", ".cxx", ".c++", ".hpp", ".hh", ".hxx", ".h++", ".ipp", ".tpp"}},
	{Plugin: "sourcecode", Language: "csharp", DisplayName: "C#", Extensions: []string{".cs"}},
	{Plugin: "sourcecode", Language: "css", DisplayName: "CSS", Extensions: []string{".css"}},
	{Plugin: "sourcecode", Language: "java", DisplayName: "Java", Extensions: []string{".java"}},
	{Plugin: "sourcecode", Language: "javascript", DisplayName: "JavaScript", Extensions: []string{".js", ".jsx", ".mjs", ".cjs"}},
	{Plugin: "sourcecode", Language: "python", DisplayName: "Python", Extensions: []string{".py"}},
	{Plugin: "sourcecode", Language: "ruby", DisplayName: "Ruby", Extensions: []string{".rb", ".rake", ".gemspec", ".ru"}},
	{Plugin: "sourcecode", Language: "rust", DisplayName: "Rust", Extensions: []string{".rs"}},
	{Plugin: "sourcecode", Language: "tsx", DisplayName: "TSX", Extensions: []string{".tsx"}},
	{Plugin: "sourcecode", Language: "typescript", DisplayName: "TypeScript", Extensions: []string{".ts", ".mts", ".cts"}},
}

// noCommentReaderError reports a file whose comments only a plugin reads, when
// no installed plugin reads them. It wraps registry.ErrUnknownFormat: like a
// file in a format no installed reader opens, the file was never opened.
type noCommentReaderError struct {
	file string
	hint commentPluginHint
}

func (e *noCommentReaderError) Error() string {
	return fmt.Sprintf("no reader for %s comments is installed, so %s was not read; install the plugin that reads them (kapi plugins install %s)",
		e.hint.DisplayName, DisplayName(e.file), e.hint.Plugin)
}

func (e *noCommentReaderError) Unwrap() error { return registry.ErrUnknownFormat }

// missingCommentReader returns a noCommentReaderError for a file that only a
// plugin's comment reader could read: no format claims its extension, no
// installed provider reads it, and a plugin declares its language. Any other
// file returns nil.
func (a *App) missingCommentReader(path string) error {
	hint, ok := commentPluginHintFor(path)
	if !ok {
		return nil
	}
	if a.hasCommentProvider(path) {
		return nil
	}
	if _, err := a.FormatReg.Detect(path, registry.DetectOptions{ExtensionOnly: true}); err == nil {
		return nil
	}
	return &noCommentReaderError{file: path, hint: hint}
}

// commentPluginHintFor returns the hint for the language whose comments a
// plugin reads in files with path's extension.
func commentPluginHintFor(path string) (commentPluginHint, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	i := slices.IndexFunc(commentPluginHints, func(h commentPluginHint) bool { return slices.Contains(h.Extensions, ext) })
	if i < 0 {
		return commentPluginHint{}, false
	}
	return commentPluginHints[i], true
}
