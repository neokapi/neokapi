package server

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/venue"
)

// Ownership of a collection row: the second read-only gate on the collections
// API, beside the connector-sourced one in project_origin.go.
//
// A collection a project's recipe declares under `context:` is recipe-owned —
// store.Collection.Owner, written by the push reconcile in bowrain/jobs. The
// recipe is authoritative for it, so renaming, reconfiguring or deleting the row
// over the API would diverge it from the kapi.yaml that declares it: the next
// push reconciles the edit away, or recreates what was deleted. The API refuses
// instead, naming the recipe as the authority and the way to make the change
// stick.
//
// The gate is on the row, not on its content: reviewing, approving and
// governing a recipe-owned collection stay allowed, exactly as they do for
// connector-sourced content.

// guardRecipeOwned refuses a mutation of a recipe-owned collection with a 409,
// the same code the connector-sourced gate answers with: the request conflicts
// with a state the server does not decide.
//
// refused reports that the request has been answered and the caller must return
// immediately — err carries only the response write, which is nil on success.
func guardRecipeOwned(c echo.Context, coll *store.Collection) (refused bool, err error) {
	if coll == nil || !venue.IsRecipeOwned(coll.Owner) {
		return false, nil
	}
	return true, c.JSON(http.StatusConflict, ErrorResponse{Error: recipeOwnedMessage(coll)})
}

// recipeOwnedMessage explains why the mutation was refused, naming the recipe
// that owns the collection and the path to changing it.
func recipeOwnedMessage(coll *store.Collection) string {
	subject := "this collection"
	if coll != nil && coll.Name != "" {
		subject = fmt.Sprintf("collection %q", coll.Name)
	}
	return fmt.Sprintf(
		"%s is declared by the project's kapi.yaml recipe, which is authoritative for it; "+
			"edit the recipe's context entry and push, and the change arrives here.",
		subject)
}
