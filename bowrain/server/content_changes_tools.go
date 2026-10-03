package server

import (
	"context"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/changes"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// commitDrafts writes what a tool produced from a stream's blocks through the
// stream's change service, as the tool, and returns the row ids of the blocks
// that landed. A block a person changed since the tool read it, or whose
// source moved, keeps what it holds.
type commitDrafts func(ctx context.Context, tool string, drafts []changes.Draft) ([]string, error)

// toolPass is a tool's run over blocks a server action read: the state each
// block was read in, recorded before the tool changes it in place.
type toolPass struct {
	states map[string]changes.BlockState
	items  map[string]string
}

// beginToolPass records the state of each block an action read, before a tool
// runs over them. source is the project's source language.
func beginToolPass(stored []*venue.StoredBlock, source model.LocaleID) *toolPass {
	p := &toolPass{states: make(map[string]changes.BlockState, len(stored)), items: make(map[string]string, len(stored))}
	for _, sb := range stored {
		if sb == nil || sb.Block == nil {
			continue
		}
		p.states[sb.Block.ID] = changes.Snapshot(sb.Block, source)
		p.items[sb.Block.ID] = sb.ItemName
	}
	return p
}

// commit writes what the tool made of the blocks it ran over.
func (p *toolPass) commit(ctx context.Context, commit commitDrafts, tool string, after []*model.Block) ([]string, error) {
	drafts := make([]changes.Draft, 0, len(after))
	for _, b := range after {
		if b == nil {
			continue
		}
		state, ok := p.states[b.ID]
		if !ok {
			continue
		}
		drafts = append(drafts, changes.Draft{Doc: p.items[b.ID], Before: state, After: b})
	}
	if len(drafts) == 0 {
		return nil, nil
	}
	return commit(ctx, tool, drafts)
}

// commitTo returns the commitDrafts of one stream of proj. c is the request
// that started the action, which watchers' refresh events are sent on; nil
// sends none.
func (s *Server) commitTo(c echo.Context, proj *store.Project, stream, wsID string) commitDrafts {
	return func(ctx context.Context, tool string, drafts []changes.Draft) ([]string, error) {
		sc := s.newStreamChange(ctx, c, proj, stream, wsID, "", changeSender{})
		ctx, _ = changes.WithChange(ctx, "")
		res, _, landed, err := changes.CommitDrafts(ctx, sc.toolService(), tool, drafts)
		if err != nil {
			return nil, err
		}
		sc.landed(ctx, res)
		return landed, nil
	}
}

// requestCommit is the commitDrafts of the stream a request names.
func (s *Server) requestCommit(c echo.Context, projectID string) (commitDrafts, error) {
	proj, err := s.ContentStore.GetProject(c.Request().Context(), projectID)
	if err != nil {
		return nil, err
	}
	wsID, _ := c.Get("workspace_id").(string)
	return s.commitTo(c, proj, streamParam(c), wsID), nil
}

// toolService is the change service a tool the server runs writes through:
// the stream's home and recorder, with no request's permissions to hold it to
// and no commit check, since a tool's drafts land with their findings and meet
// the ship gates later.
func (sc *streamChange) toolService() *change.Service {
	return changes.NewService(sc.home, sc.s.FormatRegistry,
		change.WithPolicy(sc.policy),
		change.WithAssets(sc.decide),
		change.WithRecorder(sc.recorder()),
	)
}
