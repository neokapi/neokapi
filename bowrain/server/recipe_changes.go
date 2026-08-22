package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/core/project"

	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// RecipeChangeStore is the three calls the recipe-change handlers make. A
// narrow interface rather than the whole store, so what these endpoints touch
// is legible from here.
type RecipeChangeStore interface {
	ProposeRecipeChange(ctx context.Context, ch *bstore.PendingRecipeChange) error
	PendingRecipeChanges(ctx context.Context, projectID string) ([]*bstore.PendingRecipeChange, error)
	MarkRecipeChangeApplied(ctx context.Context, changeID string) error
}

// ApproveAxisRequest approves one axis of a project's context space.
//
// Approving an axis is a separate decision from approving an artefact that sits
// on it: an axis changes the shape of the context space, an artefact changes
// what governs one point in it. Only the first edits the recipe.
//
// Collection is required for the STRUCTURAL axes and refused for the rest.
// product and channel are derived from a collection's `channel:`, so approving
// one is a statement about a particular collection, and which one is not
// derivable: a scan reads a corpus of pasted text, links and uploads, never the
// project's collections. The person approving says which.
type ApproveAxisRequest struct {
	Axis       string `json:"axis"`
	Value      string `json:"value"`
	Collection string `json:"collection,omitempty"`
}

// HandleApproveAxis records an approved axis as a pending recipe change.
//
// It does not declare the axis. Nothing here makes a coordinate real: the row
// waits for a pull to write `defaults.coordinates.<axis>` into kapi.yaml, where
// git reviews it, and the axis exists once that lands and a push carries
// content at it. The recipe remains the only thing that mints a coordinate.
func (s *Server) HandleApproveAxis(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageVoice); err != nil {
		return err
	}
	if s.RecipeChangeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "recipe changes not configured"})
	}

	var req ApproveAxisRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	axis := strings.ToLower(strings.TrimSpace(req.Axis))
	value := strings.ToLower(strings.TrimSpace(req.Value))
	if axis == "" || value == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "axis and value are required"})
	}

	projectID := c.Param("id")
	if projectID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "project id is required"})
	}
	wsID, _ := c.Get("workspace_id").(string)

	structural := axis == project.ProductAxis || axis == project.ChannelAxis
	collection := strings.TrimSpace(req.Collection)
	if !structural && collection != "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: axis + " is a declared axis: it is set on the project's default point and inherited by every collection, so naming one is a narrower claim than the axis makes",
		})
	}

	// The two paths a coordinate can be written at. A declared axis is one
	// value on the project's default point; a structural one is a collection
	// sitting somewhere, spelled as the `channel:` both axes derive from.
	path := "defaults.coordinates." + axis
	if structural {
		if collection == "" {
			return c.JSON(http.StatusConflict, ErrorResponse{
				Error: axis + " is derived from a collection's channel, not declared: name the collection this applies to",
			})
		}
		ref, cerr := s.channelRefFor(c.Request().Context(), projectID, collection, axis, value)
		if cerr != nil {
			return c.JSON(http.StatusConflict, ErrorResponse{Error: cerr.Error()})
		}
		path, value = "collections."+collection+".channel", ref
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return serverErr(c, fmt.Errorf("encode axis value: %w", err))
	}

	ch := &bstore.PendingRecipeChange{
		WorkspaceID: wsID,
		ProjectID:   projectID,
		Path:        path,
		Value:       raw,
		CreatedBy:   recipeChangeActor(c),
	}
	if err := s.RecipeChangeStore.ProposeRecipeChange(c.Request().Context(), ch); err != nil {
		return serverErr(c, fmt.Errorf("propose recipe change %s: %w", ch.Path, err))
	}
	return c.JSON(http.StatusOK, ch)
}

// channelRefFor composes the `product/channel` a collection would move to when
// one of the two structural axes is approved for it.
//
// Both halves are needed and only one is being approved, so the other comes
// from where the collection sits TODAY — the point the server holds because a
// push reconciled it (store.Collection.Context), which is the same index the
// context-profiles board is built from. Approving a channel for a collection
// that has no product yet is refused rather than guessed: `channel:` cannot
// express a channel alone, and inventing a product would put the collection
// somewhere nobody chose.
func (s *Server) channelRefFor(ctx context.Context, projectID, collection, axis, value string) (string, error) {
	at, err := s.collectionPoint(ctx, projectID, collection)
	if err != nil {
		return "", err
	}
	product, channel := at[project.ProductAxis], at[project.ChannelAxis]
	switch axis {
	case project.ProductAxis:
		product = value
	case project.ChannelAxis:
		channel = value
	}
	if product == "" {
		return "", fmt.Errorf("collection %q sits at no product, and a channel is a surface OF a product: approve the product for it first", collection)
	}
	if channel == "" {
		// A product with no channel is a coherent point — the collection sits
		// at the product's default surface — but `channel:` has no spelling for
		// it, so the axis is written with the collection's own name as the
		// surface it ships on. It is the one name that is already the person's.
		channel = collection
	}
	return product + "/" + channel, nil
}

// collectionPoint reads where a named collection currently sits, across the
// project's streams. A collection the venue does not hold is refused: a recipe
// change naming one would not apply, and finding that out on someone's laptop
// days later is the failure this check exists to move forward.
func (s *Server) collectionPoint(ctx context.Context, projectID, collection string) (map[string]string, error) {
	if s.ContentStore == nil {
		return nil, errors.New("collections are not readable here, so the point cannot be resolved")
	}
	streams := []string{"main"}
	if list, err := s.ContentStore.ListStreams(ctx, projectID, false); err == nil {
		for _, st := range list {
			if st != nil && st.Name != "" && st.Name != "main" {
				streams = append(streams, st.Name)
			}
		}
	}
	var known []string
	for _, stream := range streams {
		cols, err := s.ContentStore.ListCollections(ctx, projectID, stream)
		if err != nil {
			continue
		}
		for _, col := range cols {
			if col == nil || col.Name == "" {
				continue
			}
			if col.Name == collection {
				return col.Context, nil
			}
			known = append(known, col.Name)
		}
	}
	sort.Strings(known)
	if len(known) == 0 {
		return nil, errors.New("this project declares no collections yet: push one before placing it")
	}
	return nil, fmt.Errorf("no collection named %q (declared: %s)", collection, strings.Join(slices.Compact(known), ", "))
}

// recipeChangeActor is the signed-in user recorded on a proposal, so a recipe
// line that lands in someone's working tree names who decided it.
func recipeChangeActor(c echo.Context) string {
	actor, _ := c.Get("user_id").(string)
	return actor
}

// HandleListPendingRecipeChanges answers what a project's working tree has not
// yet taken. The pull reads this, applies each through the same setRecipeField
// the local fix loop uses, and marks them applied.
func (s *Server) HandleListPendingRecipeChanges(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.RecipeChangeStore == nil {
		return c.JSON(http.StatusOK, []*bstore.PendingRecipeChange{})
	}
	changes, err := s.RecipeChangeStore.PendingRecipeChanges(c.Request().Context(), c.Param("id"))
	if err != nil {
		return serverErr(c, fmt.Errorf("list pending recipe changes: %w", err))
	}
	if changes == nil {
		changes = []*bstore.PendingRecipeChange{}
	}
	return c.JSON(http.StatusOK, changes)
}

// HandleMarkRecipeChangeApplied settles one change once a working tree has
// taken it. Marking an already-applied change is a no-op: a retried pull must
// converge rather than fail.
func (s *Server) HandleMarkRecipeChangeApplied(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageVoice); err != nil {
		return err
	}
	if s.RecipeChangeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "recipe changes not configured"})
	}
	if err := s.RecipeChangeStore.MarkRecipeChangeApplied(c.Request().Context(), c.Param("changeID")); err != nil {
		return serverErr(c, fmt.Errorf("mark recipe change applied: %w", err))
	}
	return c.NoContent(http.StatusNoContent)
}
