//go:build !js

package pluginhost

import (
	"context"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/plugin/commentproto"
	pb "github.com/neokapi/neokapi/core/plugin/proto/v2"
)

// locateCommentsTimeout bounds one LocateComments call, spawning the daemon
// included. comment.Provider carries no context, so the bound lives here.
const locateCommentsTimeout = 2 * time.Minute

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
	f, err := commentproto.FromProto(p.lang.Language, len(src), resp)
	if err != nil {
		return nil, fmt.Errorf("plugin %q located %s comments outside the file: %w", p.plugin.Name(), p.lang.Language, err)
	}
	return f, nil
}
