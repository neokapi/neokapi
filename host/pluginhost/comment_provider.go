package pluginhost

import (
	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/plugin/manifest"
)

// CommentProvider returns the comment provider for a comment language a plugin
// declares (manifest capabilities.comments), dispatched to the plugin's
// LocateComments RPC over the Mode-C daemon. A language whose manifest declares
// a rewrite gets a provider that also writes its comments (comment.Rewriter).
func CommentProvider(pool *DaemonPool, route *CommentRoute) comment.Provider {
	p := &daemonCommentProvider{pool: pool, plugin: route.Plugin, lang: route.Language}
	if route.Language.Rewrite != nil {
		return &daemonCommentRewriter{daemonCommentProvider: p}
	}
	return p
}

// daemonCommentProvider locates the comments of one language through a plugin.
// Its canary is the one the plugin's manifest declares: the host holds the
// canary's bytes and asks the plugin to read them the way it reads every real
// file.
type daemonCommentProvider struct {
	pool   *DaemonPool
	plugin *Plugin
	lang   manifest.CommentLanguage
}

// Language implements comment.Provider.
func (p *daemonCommentProvider) Language() string { return p.lang.Language }

// Extensions implements comment.Provider.
func (p *daemonCommentProvider) Extensions() []string { return p.lang.Extensions }

// LineText implements comment.Provider from the comment markers the manifest
// declares, so reading one comment line needs no call to the plugin.
func (p *daemonCommentProvider) LineText(line []byte) (int, string, bool) {
	return p.markers().LineText(line)
}

// markers are the comment markers the manifest declares for the language.
func (p *daemonCommentProvider) markers() comment.Markers {
	m := comment.Markers{Line: p.lang.Markers.Line, Splice: p.lang.Markers.Splice}
	for _, b := range p.lang.Markers.Block {
		m.Block = append(m.Block, comment.BlockMarker{Open: b.Open, Close: b.Close, Nested: b.Nested})
	}
	return m
}

// Canary implements comment.Provider.
func (p *daemonCommentProvider) Canary() comment.Canary {
	return comment.Canary{Name: p.lang.Canary.Name, Source: []byte(p.lang.Canary.Source), Block: p.lang.Canary.Block}
}
