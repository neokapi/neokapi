package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

// ApproveAxisRequest approves one axis of a project's default point.
//
// Approving an axis is a separate decision from approving an artefact that sits
// on it: an axis changes the shape of the context space, an artefact changes
// what governs one point in it. Only the first edits the recipe.
type ApproveAxisRequest struct {
	Axis  string `json:"axis"`
	Value string `json:"value"`
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

	// product and channel are DERIVED from a collection's channel:, so a
	// default coordinate for one would state a point the recipe also computes.
	// `kapi apply` refuses them too; refusing here as well means the error
	// names the reason now rather than surfacing as a puzzling apply failure
	// days later, on a machine that is not this one.
	if axis == project.ProductAxis || axis == project.ChannelAxis {
		return c.JSON(http.StatusConflict, ErrorResponse{
			Error: axis + " is derived from a collection's channel, not declared: set the collection's channel instead of approving it as a default coordinate",
		})
	}

	projectID := c.Param("id")
	if projectID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "project id is required"})
	}
	wsID, _ := c.Get("workspace_id").(string)

	raw, err := json.Marshal(value)
	if err != nil {
		return serverErr(c, fmt.Errorf("encode axis value: %w", err))
	}

	ch := &bstore.PendingRecipeChange{
		WorkspaceID: wsID,
		ProjectID:   projectID,
		Path:        "defaults.coordinates." + axis,
		Value:       raw,
		CreatedBy:   recipeChangeActor(c),
	}
	if err := s.RecipeChangeStore.ProposeRecipeChange(c.Request().Context(), ch); err != nil {
		return serverErr(c, fmt.Errorf("propose recipe change %s: %w", ch.Path, err))
	}
	return c.JSON(http.StatusOK, ch)
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
