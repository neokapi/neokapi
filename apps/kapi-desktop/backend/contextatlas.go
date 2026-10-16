package backend

// The atlas on a project's Context hub: every rule, note, term and content
// cell the project holds, with the axes they sit on and the history of each,
// for the Explore section. host.ContextAtlas assembles it, the same read
// `kapi context atlas --json` prints, so the two surfaces show one thing.

import (
	"context"

	"github.com/neokapi/neokapi/host"
)

// ProjectContextAtlas reads the atlas of the project a tab holds. A tab with a
// checkout includes its content cells; a project known only to the workspace
// is read from its log and stores alone.
func (a *App) ProjectContextAtlas(tabID string) (*host.ContextAtlas, error) {
	op := a.getOpenProject(tabID)
	req := host.ContextAtlasRequest{}
	if op != nil && op.Path != "" {
		req.Project, req.Content = op.Path, true
	} else {
		key, err := a.tabWorkspaceKey(tabID)
		if err != nil {
			return nil, err
		}
		req.Project = string(key)
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextFeedTimeout)
	defer cancel()
	atlas, err := a.hostEngine().ContextAtlas(ctx, req)
	if err != nil {
		return nil, err
	}
	return &atlas, nil
}
