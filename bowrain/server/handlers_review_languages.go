package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/locale"
)

// The languages a person follows in a project, and the project's own target
// languages.
//
// A followed list filters the project's shared queues for one person: the
// review inbox and the review plan count only the languages on it. Bowrain is
// a team tool, so following a language never copies work. Everyone who
// follows it shares its one queue, and a translation one person decides
// leaves it for everyone.
//
// A person who has saved no list follows every language they may review, so
// nothing changes for them until they choose.

// recipePathTargetLanguages is the recipe field a project-language change
// files.
const recipePathTargetLanguages = "defaults.target_languages"

// recipeChangeSourceProposal names a recipe change filed for the next pull to
// apply, in the recipe.changed event the feed records.
const recipeChangeSourceProposal = "proposal"

// reviewLanguagesResponse answers GET and PUT /:ws/:id/review/languages.
type reviewLanguagesResponse struct {
	// Followed is the languages the caller follows: the saved list, kept to
	// the ones still allowed, or every allowed language when none is saved.
	Followed []string `json:"followed"`
	// Saved is set once the caller has saved a list for the project.
	Saved bool `json:"saved"`
	// Allowed is the project's target languages the caller may review: all
	// of them with the review permission, narrowed to the languages the
	// caller's membership names when it names any.
	Allowed []string `json:"allowed"`
	// Source is the project's source language.
	Source string `json:"source"`
	// Project is the project's target languages as the last push declared
	// them.
	Project []string `json:"project"`
	// PendingProject is the target languages a recipe change waiting for the
	// next pull sets, when one waits.
	PendingProject []string `json:"pending_project,omitempty"`
	// CanManage is set when the caller may turn the project's languages on
	// and off.
	CanManage bool `json:"can_manage"`
}

// reviewLanguagesRequest saves the caller's followed languages.
type reviewLanguagesRequest struct {
	Followed []string `json:"followed"`
}

// projectLanguageRequest turns one of the project's target languages on or
// off.
type projectLanguageRequest struct {
	Locale  string `json:"locale"`
	Enabled bool   `json:"enabled"`
}

// followedLocales is the part of allowed the person follows in the project,
// in allowed's order, and whether they have saved a list. With no store, or
// no list saved, it is allowed whole.
func (s *Server) followedLocales(ctx context.Context, userID, projectID string, allowed []string) ([]string, bool, error) {
	fs, ok := platstore.FollowedLanguages(s.ContentStore)
	if !ok || userID == "" {
		return allowed, false, nil
	}
	saved, isSaved, err := fs.FollowedLanguages(ctx, userID, projectID)
	if err != nil || !isSaved {
		return allowed, false, err
	}
	return keepFollowed(allowed, saved), true, nil
}

// keepFollowed is the locales of allowed that followed names, in allowed's
// order.
func keepFollowed(allowed, followed []string) []string {
	out := []string{}
	for _, l := range allowed {
		if slices.Contains(followed, l) {
			out = append(out, l)
		}
	}
	return out
}

// reviewLanguagesOf builds the languages answer for the caller of a project
// route.
func (s *Server) reviewLanguagesOf(c echo.Context, proj *platstore.Project) (reviewLanguagesResponse, error) {
	ctx := c.Request().Context()
	userID, _ := c.Get("user_id").(string)
	perms, _ := c.Get("project_permissions").(platauth.Permission)
	langs, _ := c.Get("project_languages").([]string)

	out := reviewLanguagesResponse{
		Allowed:   reviewLocalesOf(proj, perms, langs),
		Source:    string(proj.DefaultSourceLanguage),
		Project:   []string{},
		CanManage: perms.Has(platauth.PermManageProject),
	}
	if out.Allowed == nil {
		out.Allowed = []string{}
	}
	for _, l := range proj.TargetLanguages {
		out.Project = append(out.Project, string(l))
	}
	var err error
	if out.Followed, out.Saved, err = s.followedLocales(ctx, userID, proj.ID, out.Allowed); err != nil {
		return out, err
	}
	if pending, ok, err := s.pendingTargetLanguages(ctx, proj.ID); err != nil {
		return out, err
	} else if ok {
		out.PendingProject = pending
	}
	return out, nil
}

// pendingTargetLanguages reads the target languages a pending recipe change
// sets for the project, if one waits.
func (s *Server) pendingTargetLanguages(ctx context.Context, projectID string) ([]string, bool, error) {
	if s.RecipeChangeStore == nil {
		return nil, false, nil
	}
	changes, err := s.RecipeChangeStore.PendingRecipeChanges(ctx, projectID)
	if err != nil {
		return nil, false, fmt.Errorf("list pending recipe changes: %w", err)
	}
	for _, ch := range changes {
		if ch.Path != recipePathTargetLanguages {
			continue
		}
		var locales []string
		if err := json.Unmarshal(ch.Value, &locales); err != nil {
			continue
		}
		if locales == nil {
			locales = []string{}
		}
		return locales, true, nil
	}
	return nil, false, nil
}

// HandleGetReviewLanguages answers GET /:ws/:id/review/languages: the
// languages the caller follows in the project, the ones they may follow, and
// the project's own.
func (s *Server) HandleGetReviewLanguages(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.ContentStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "store not configured")
	}
	proj, err := s.ContentStore.GetProject(c.Request().Context(), projectParam(c))
	if err != nil || proj == nil {
		return apiErr(c, http.StatusNotFound, errNoProject.Error())
	}
	out, err := s.reviewLanguagesOf(c, proj)
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// HandlePutReviewLanguages answers PUT /:ws/:id/review/languages
// {"followed": [...]}: it saves the caller's followed languages. A language
// the caller may not review is refused (400). An empty list is a saved choice
// to follow none.
func (s *Server) HandlePutReviewLanguages(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.ContentStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "store not configured")
	}
	fs, ok := platstore.FollowedLanguages(s.ContentStore)
	if !ok {
		return apiErr(c, http.StatusServiceUnavailable, "followed languages need a content store that keeps them")
	}
	var req reviewLanguagesRequest
	if err := c.Bind(&req); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	ctx := c.Request().Context()
	proj, err := s.ContentStore.GetProject(ctx, projectParam(c))
	if err != nil || proj == nil {
		return apiErr(c, http.StatusNotFound, errNoProject.Error())
	}
	perms, _ := c.Get("project_permissions").(platauth.Permission)
	langs, _ := c.Get("project_languages").([]string)
	allowed := reviewLocalesOf(proj, perms, langs)
	followed := []string{}
	for _, l := range req.Followed {
		l = strings.TrimSpace(l)
		if !slices.Contains(allowed, l) {
			return apiErr(c, http.StatusBadRequest, fmt.Sprintf("you can't review %q in this project", l))
		}
		if !slices.Contains(followed, l) {
			followed = append(followed, l)
		}
	}
	userID, _ := c.Get("user_id").(string)
	if err := fs.PutFollowedLanguages(ctx, userID, proj.ID, followed); err != nil {
		return serverErr(c, err)
	}
	out, err := s.reviewLanguagesOf(c, proj)
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// HandleSetProjectLanguage answers POST /:ws/:id/review/languages/project
// {"locale": "nb", "enabled": true}: it turns one of the project's target
// languages on or off. The change is filed as a pending recipe change, which
// the next pull writes into the recipe, and the feed records it. It needs the
// permission to edit the project's settings, and asks no second person.
func (s *Server) HandleSetProjectLanguage(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageProject); err != nil {
		return err
	}
	if s.ContentStore == nil || s.RecipeChangeStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "recipe changes not configured")
	}
	var req projectLanguageRequest
	if err := c.Bind(&req); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	loc, err := locale.Canonical(strings.TrimSpace(req.Locale))
	if err != nil || loc == "" {
		return apiErr(c, http.StatusBadRequest, fmt.Sprintf("%q is not a language code", req.Locale))
	}
	ctx := c.Request().Context()
	proj, err := s.ContentStore.GetProject(ctx, projectParam(c))
	if err != nil || proj == nil {
		return apiErr(c, http.StatusNotFound, errNoProject.Error())
	}
	if req.Enabled && loc == proj.DefaultSourceLanguage {
		return apiErr(c, http.StatusBadRequest, "the source language can't be a target language")
	}

	// The list the change starts from: a change already waiting, so two
	// toggles before a pull compose, or else the project's own.
	current, pending, err := s.pendingTargetLanguages(ctx, proj.ID)
	if err != nil {
		return serverErr(c, err)
	}
	if !pending {
		current = []string{}
		for _, l := range proj.TargetLanguages {
			current = append(current, string(l))
		}
	}
	next := slices.DeleteFunc(slices.Clone(current), func(l string) bool { return l == string(loc) })
	if req.Enabled {
		next = append(next, string(loc))
	}

	if !slices.Equal(next, current) {
		raw, err := json.Marshal(next)
		if err != nil {
			return serverErr(c, fmt.Errorf("encode target languages: %w", err))
		}
		wsID, _ := c.Get("workspace_id").(string)
		if err := s.RecipeChangeStore.ProposeRecipeChange(ctx, &bstore.PendingRecipeChange{
			WorkspaceID: wsID,
			ProjectID:   proj.ID,
			Path:        recipePathTargetLanguages,
			Value:       raw,
			CreatedBy:   recipeChangeActor(c),
		}); err != nil {
			return serverErr(c, fmt.Errorf("propose recipe change %s: %w", recipePathTargetLanguages, err))
		}
		recipe := platev.RecipeChange{
			WorkspaceSlug: c.Param("ws"),
			Source:        recipeChangeSourceProposal,
			Targets:       &platev.TargetsChange{Before: current, After: next},
		}
		s.emitAudit(c, auditEvent{
			Type:         platev.EventRecipeChanged,
			WorkspaceID:  proj.WorkspaceID,
			ProjectID:    proj.ID,
			ResourceType: "project",
			ResourceID:   proj.ID,
			Data:         recipe.Data(),
		})
	}

	out, err := s.reviewLanguagesOf(c, proj)
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, out)
}
