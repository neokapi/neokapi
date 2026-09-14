package host

import (
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
// does for a format.
func (a *App) commentProviderFor(path string) (comment.Provider, bool) {
	if a.PluginHost != nil {
		ext := strings.ToLower(filepath.Ext(path))
		for _, r := range a.PluginHost.CommentRoutes() {
			if slices.ContainsFunc(r.Language.Extensions, func(e string) bool { return strings.ToLower(e) == ext }) {
				return pluginhost.CommentProvider(a.DaemonPool(), r), true
			}
		}
	}
	return commentProviders.For(path)
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
	{Plugin: "sourcecode", Language: "javascript", DisplayName: "JavaScript", Extensions: []string{".js", ".jsx", ".mjs", ".cjs"}},
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
	ext := strings.ToLower(filepath.Ext(path))
	i := slices.IndexFunc(commentPluginHints, func(h commentPluginHint) bool { return slices.Contains(h.Extensions, ext) })
	if i < 0 {
		return nil
	}
	if _, ok := a.commentProviderFor(path); ok {
		return nil
	}
	if _, err := a.FormatReg.Detect(path, registry.DetectOptions{ExtensionOnly: true}); err == nil {
		return nil
	}
	return &noCommentReaderError{file: path, hint: commentPluginHints[i]}
}
