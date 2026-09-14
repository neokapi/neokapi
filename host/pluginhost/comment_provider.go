package pluginhost

import (
	"context"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/plugin/manifest"
	pb "github.com/neokapi/neokapi/core/plugin/proto/v2"
	"github.com/neokapi/neokapi/core/plugin/protoconvert"
)

// locateCommentsTimeout bounds one LocateComments call, spawning the daemon
// included. comment.Provider carries no context, so the bound lives here.
const locateCommentsTimeout = 2 * time.Minute

// CommentProvider returns the comment provider for a comment language a plugin
// declares (manifest capabilities.comments), dispatched to the plugin's
// LocateComments RPC over the Mode-C daemon.
func CommentProvider(pool *DaemonPool, route *CommentRoute) comment.Provider {
	return &daemonCommentProvider{pool: pool, plugin: route.Plugin, lang: route.Language}
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

// Canary implements comment.Provider.
func (p *daemonCommentProvider) Canary() comment.Canary {
	return comment.Canary{Name: p.lang.Canary.Name, Source: []byte(p.lang.Canary.Source), Block: p.lang.Canary.Block}
}

// Locate implements comment.Provider. A file the plugin read and could not
// place the comments of exactly wraps comment.ErrUnlocated.
func (p *daemonCommentProvider) Locate(name string, src []byte) (*comment.File, error) {
	ctx, cancel := context.WithTimeout(context.Background(), locateCommentsTimeout)
	defer cancel()
	client, err := p.pool.Acquire(ctx, p.plugin)
	if err != nil {
		return nil, fmt.Errorf("acquire daemon for plugin %q: %w", p.plugin.Name(), err)
	}
	resp, err := pb.NewBridgeServiceClient(client.Conn).LocateComments(ctx, &pb.LocateCommentsRequest{
		Language: p.lang.Language,
		Name:     name,
		Source:   src,
	})
	if err != nil {
		return nil, fmt.Errorf("locate %s comments (plugin %q): %w", p.lang.Language, p.plugin.Name(), err)
	}
	if resp.GetUnlocated() {
		return nil, fmt.Errorf("%w: %s", comment.ErrUnlocated, resp.GetError())
	}
	if e := resp.GetError(); e != "" {
		return nil, fmt.Errorf("locate %s comments (plugin %q): %s", p.lang.Language, p.plugin.Name(), e)
	}
	f, err := protoconvert.ProtoToCommentFile(p.lang.Language, len(src), resp)
	if err != nil {
		return nil, fmt.Errorf("plugin %q located %s comments outside the file: %w", p.plugin.Name(), p.lang.Language, err)
	}
	return f, nil
}
