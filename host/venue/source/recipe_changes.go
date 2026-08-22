package source

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/neokapi/neokapi/core/project"
)

// RecipeChangeOutcome is what a pull did with one recipe change an approval
// made on the venue.
type RecipeChangeOutcome struct {
	Path string
	// Applied is true when the working tree's recipe now says this.
	Applied bool
	// Conflict names the value the recipe already had, when it differed from
	// the one proposed. A conflict is reported, never resolved: a coordinate is
	// a decision, and overwriting one silently would make the venue the author
	// of a choice somebody made here.
	Conflict string
	// Err is a proposal that could not be applied at all — an unknown path, a
	// derived axis, an unreadable value.
	Err error
}

// applyPendingRecipeChanges writes the recipe fields an approval decided on the
// venue into this working tree, and settles each on the server once it lands.
//
// This runs BEFORE content in a pull. A coordinate is minted by the recipe, so
// content arriving at a point the recipe does not yet declare is the same
// divergence the venue refuses on the other side — the recipe has to be able to
// account for what the pull is about to write.
//
// A failure here never fails the pull. The recipe is a decision waiting to be
// reviewed in git; content is the work. Losing the first for a moment is a
// nuisance, losing the second is the pull not doing its job.
func (c *BowrainSourceConnector) applyPendingRecipeChanges(ctx context.Context) []RecipeChangeOutcome {
	if c.client == nil || c.project == nil {
		return nil
	}

	pending, err := c.client.PendingRecipeChanges(ctx)
	if err != nil {
		slog.WarnContext(ctx, "could not read pending recipe changes; pulling content only", "error", err)
		return nil
	}
	if len(pending) == 0 {
		return nil
	}

	recipePath := c.project.Layout.RecipePath
	proj, err := project.LoadWithOptions(recipePath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		slog.WarnContext(ctx, "could not load the recipe to apply pending changes", "error", err)
		return nil
	}

	var out []RecipeChangeOutcome
	dirty := false
	for _, ch := range pending {
		res := RecipeChangeOutcome{Path: ch.Path}

		// What the recipe says now, so a genuine disagreement can be named
		// rather than overwritten.
		if existing, ok := currentRecipeValue(proj, ch.Path); ok {
			var proposed string
			if err := json.Unmarshal(ch.Value, &proposed); err == nil && existing != "" && existing != proposed {
				res.Conflict = existing
				out = append(out, res)
				continue
			}
		}

		changed, err := project.SetField(proj, ch.Path, ch.Value)
		if err != nil {
			res.Err = err
			out = append(out, res)
			continue
		}
		dirty = dirty || changed
		res.Applied = true
		out = append(out, res)

		// Settle it whether or not the value moved: a change that matched what
		// the recipe already said is taken, not pending.
		if err := c.client.MarkRecipeChangeApplied(ctx, ch.ID); err != nil {
			slog.WarnContext(ctx, "recipe change applied locally but not settled on the venue",
				"path", ch.Path, "error", err)
		}
	}

	if dirty {
		if err := project.Save(recipePath, proj); err != nil {
			slog.WarnContext(ctx, "could not write the recipe", "error", err)
			for i := range out {
				if out[i].Applied {
					out[i].Applied = false
					out[i].Err = fmt.Errorf("write recipe: %w", err)
				}
			}
		}
	}
	return out
}

// currentRecipeValue reads what the recipe says at a settable path today, for
// the paths where a disagreement is possible. It answers false for a path with
// nothing set, which is not a conflict — it is the ordinary case.
func currentRecipeValue(proj *project.KapiProject, path string) (string, bool) {
	const coordPrefix = "defaults.coordinates."
	if len(path) > len(coordPrefix) && path[:len(coordPrefix)] == coordPrefix {
		axis := path[len(coordPrefix):]
		v, ok := proj.Defaults.Coordinates[axis]
		return v, ok
	}
	return "", false
}
