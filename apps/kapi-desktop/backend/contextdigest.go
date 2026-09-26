package backend

// The digest on a project's Context hub: what kapi learned about how the
// project writes since the person last looked. host.ContextDigest assembles
// it from the operation log, the same call `kapi context digest` makes, so
// the two surfaces show one thing.
//
// The "since you last looked" marker is the person's, kept in this machine
// account's config. The frontend reads the digest once with the marker,
// holds that instant for as long as the view is open, and moves the marker
// at once: refreshes while the view is open pass the held instant back, so
// what was new when the person arrived stays new until they leave.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/host"
)

// ProjectContextDigest reads the digest of the project a tab holds. since is
// an RFC3339 instant to read from in place of the marker, empty for the
// marker.
func (a *App) ProjectContextDigest(tabID, since string) (*host.ContextDigest, error) {
	key, err := a.tabWorkspaceKey(tabID)
	if err != nil {
		return nil, err
	}
	req := host.ContextDigestRequest{Key: key}
	if since != "" {
		at, perr := time.Parse(time.RFC3339Nano, since)
		if perr != nil {
			return nil, fmt.Errorf("read the digest from %q: %w", since, perr)
		}
		req.Since = at
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextFeedTimeout)
	defer cancel()
	digest, err := a.hostEngine().ContextDigest(ctx, req)
	if err != nil {
		return nil, err
	}
	return &digest, nil
}

// MarkProjectContextDigestSeen records that the person looked at the digest of
// the project a tab holds, now. It returns the marker it replaced, RFC3339,
// empty when the person had never looked.
func (a *App) MarkProjectContextDigestSeen(tabID string) (string, error) {
	key, err := a.tabWorkspaceKey(tabID)
	if err != nil {
		return "", err
	}
	previous, err := host.MarkContextDigestSeen(key, time.Now().UTC())
	if err != nil || previous.IsZero() {
		return "", err
	}
	return previous.UTC().Format(time.RFC3339Nano), nil
}

// KeepContextGroup keeps several suggestions in one step: a theme's
// suggestions in one collection, as the digest groups them. It returns how
// many were kept.
func (a *App) KeepContextGroup(projectKey string, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, errors.New("name the suggestions to keep")
	}
	recipe, err := a.contextRecipeFor(projectKey)
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextFeedTimeout)
	defer cancel()
	res, err := a.hostEngine().KeepContextOperations(ctx, host.ContextKeepRequest{
		Actor:   deskPerson(),
		Project: recipe,
		IDs:     ids,
	})
	a.emitEvent("workspace:changed", nil)
	return len(res.Kept), err
}

// ChooseContextSide settles a conflict for the rule the person chose: the
// rival rules are set aside and the chosen one is kept, with the edit the
// person made to it.
func (a *App) ChooseContextSide(req ContextDecisionRequest) (*ContextFeedEntry, error) {
	recipe, err := a.contextRecipeFor(req.Project)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextFeedTimeout)
	defer cancel()
	res, err := a.hostEngine().ChooseContextSide(ctx, host.ContextChooseRequest{
		Actor:       deskPerson(),
		Project:     recipe,
		ID:          req.ID,
		Replacement: req.Replacement,
		Note:        req.Note,
	})
	a.emitEvent("workspace:changed", nil)
	if err != nil {
		return nil, err
	}
	entry := contextFeedEntry(res.Kept.Record, "", recipe)
	return &entry, nil
}

// ContextNews reports, for each project whose digest holds something the
// person has not seen, how much: what the home screen shows beside a project
// in place of a count of work waiting. A project with nothing new is absent.
func (a *App) ContextNews() ([]host.ContextNews, error) {
	ctx, cancel := context.WithTimeout(context.Background(), contextFeedTimeout)
	defer cancel()
	return a.hostEngine().ContextNews(ctx)
}

// tabWorkspaceKey is the workspace key of the project a tab holds.
func (a *App) tabWorkspaceKey(tabID string) (workspace.ProjectKey, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return "", fmt.Errorf("project tab %q not found", tabID)
	}
	if op.workspaceKey == "" {
		return "", errors.New("this tab holds no project the workspace knows")
	}
	return op.workspaceKey, nil
}
