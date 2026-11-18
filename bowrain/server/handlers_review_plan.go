package server

import (
	"context"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/host/reviewdeck"
	"github.com/neokapi/neokapi/terms"
)

// The review plan: what a project asks of its reviewers, in the order that
// gives each decision the most reach (docs/internals/review-by-leverage.md).
//
//  1. Coordinates: the products, channels and languages, and the voice each
//     point binds. A few decisions, each governing everything under it.
//  2. Key language: the terms and voice rules, source first, then each
//     term's translation in every target language.
//  3. Exceptions: what fails a check against the key language.
//  4. Interfaces: the pages a reviewer reads whole, in context.
//  5. Policy: what the involvement in force leaves to the checks.
//
// The plan counts; it decides nothing. Each level names the surface that
// already decides it.

// Project properties the plan reads and writes.
const (
	// propReviewInvolvement is how much the project's reviewers can be
	// involved: context, interfaces or every.
	propReviewInvolvement = "review_involvement"
	// propCoordinatesConfirmedBy and propCoordinatesConfirmedAt record the
	// person who confirmed the coordinates, and when.
	propCoordinatesConfirmedBy = "review_coordinates_confirmed_by"
	propCoordinatesConfirmedAt = "review_coordinates_confirmed_at"
)

// Review involvement levels.
const (
	involvementContext    = "context"
	involvementInterfaces = "interfaces"
	involvementEvery      = "every"
)

// Plan levels, in the order a reviewer works them.
const (
	planCoordinates = "coordinates"
	planKeyLanguage = "key_language"
	planExceptions  = "exceptions"
	planInterfaces  = "interfaces"
	planPolicy      = "policy"
)

// reviewPagesCountLimit bounds the pages the plan counts per language.
const reviewPagesCountLimit = 5000

type reviewPlanResponse struct {
	Stream string `json:"stream"`
	// Involvement is the project's: context (the default), interfaces or
	// every.
	Involvement string `json:"involvement"`
	// Next is the first level with something waiting for the caller, or ""
	// when nothing does.
	Next        string             `json:"next"`
	Coordinates planCoordinatesDTO `json:"coordinates"`
	KeyLanguage planKeyLanguageDTO `json:"key_language"`
	Exceptions  planExceptionsDTO  `json:"exceptions"`
	Languages   []planLanguageDTO  `json:"languages"`
}

// planPoint is one place in the project's context space: a collection's
// coordinates and the voice bound there.
type planPoint struct {
	Collection  string            `json:"collection"`
	Coordinates map[string]string `json:"coordinates"`
	Voice       string            `json:"voice,omitempty"`
	SourceOnly  bool              `json:"source_only,omitempty"`
	Priority    string            `json:"priority,omitempty"`
}

type planCoordinatesDTO struct {
	SourceLanguage  string      `json:"source_language"`
	TargetLanguages []string    `json:"target_languages"`
	Points          []planPoint `json:"points"`
	// Confirmed is set once a person has confirmed the coordinates.
	Confirmed   bool   `json:"confirmed"`
	ConfirmedBy string `json:"confirmed_by,omitempty"`
	ConfirmedAt string `json:"confirmed_at,omitempty"`
}

type planKeyLanguageDTO struct {
	// Waiting is the context cards the caller may keep that wait for
	// review, by kind; zero when the caller keeps no context.
	Waiting int            `json:"waiting"`
	ByKind  map[string]int `json:"by_kind"`
	// Concepts is the workspace's concepts with a term in the source
	// language.
	Concepts int `json:"concepts"`
	// Untranslated counts, per target language, the concepts with a source
	// term and no usable term in that language, leaving out the ones marked
	// do-not-translate. Each is a decision to make.
	Untranslated map[string]int `json:"untranslated"`
}

type planExceptionsDTO struct {
	// SourceFindings is the findings the last project-wide check recorded in
	// the source; nil before any check.
	SourceFindings *int   `json:"source_findings,omitempty"`
	CheckedAt      string `json:"checked_at,omitempty"`
}

// planLanguageDTO is the plan's part for one target language the caller
// may review.
type planLanguageDTO struct {
	Locale string `json:"locale"`
	// Waiting is the translations the deck deals, by group.
	deckWaiting
	// Failing is the translations held back by a failing check or a stale
	// approval: the deck's desk group.
	Failing int `json:"failing"`
	// Pages is the pages with a translation waiting.
	Pages     int  `json:"pages"`
	PagesMore bool `json:"pages_more,omitempty"`
	// Settleable is the translations the involvement in force leaves to the
	// checks: the ones that clear every bar (the deck's fast group). Zero
	// under every.
	Settleable int `json:"settleable"`
}

// HandleGetReviewPlan answers GET /:ws/:id/review/plan[?stream=] with the
// project's review plan for the caller.
func (s *Server) HandleGetReviewPlan(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.ContentStore == nil || s.AuthStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "store not configured")
	}
	ctx := c.Request().Context()
	proj, err := s.ContentStore.GetProject(ctx, projectParam(c))
	if err != nil || proj == nil {
		return apiErr(c, http.StatusNotFound, errNoProject.Error())
	}
	stream := strings.TrimSpace(c.QueryParam("stream"))
	if stream == "" {
		stream = reviewInboxStream
	}
	wsID, _ := c.Get("workspace_id").(string)
	userID, _ := c.Get("user_id").(string)
	ws, err := s.AuthStore.GetWorkspace(ctx, wsID)
	if err != nil || ws == nil {
		return apiErr(c, http.StatusNotFound, "workspace not found")
	}
	perms, _ := c.Get("project_permissions").(platauth.Permission)
	reach, _ := c.Get("project_coordinates").(platauth.CoordinateReach)
	langs, _ := c.Get("project_languages").([]string)

	out := reviewPlanResponse{
		Stream:      stream,
		Involvement: involvementOf(proj),
		Languages:   []planLanguageDTO{},
	}
	if out.Coordinates, err = s.planCoordinates(ctx, proj, stream); err != nil {
		return serverErr(c, err)
	}
	if out.KeyLanguage, err = s.planKeyLanguage(ctx, ws, proj, userID, perms, reach); err != nil {
		return serverErr(c, err)
	}
	if out.Exceptions, err = s.planExceptions(ctx, proj, stream); err != nil {
		return serverErr(c, err)
	}
	if out.Languages, err = s.planLanguages(ctx, proj, stream, reviewLocalesOf(proj, perms, langs), out.Involvement); err != nil {
		return serverErr(c, err)
	}
	out.Next = nextPlanLevel(out)
	return c.JSON(http.StatusOK, out)
}

// involvementOf is the project's review involvement, context when unset.
func involvementOf(p *platstore.Project) string {
	switch v := p.Properties[propReviewInvolvement]; v {
	case involvementInterfaces, involvementEvery:
		return v
	}
	return involvementContext
}

// nextPlanLevel is the first level with something waiting.
func nextPlanLevel(p reviewPlanResponse) string {
	if !p.Coordinates.Confirmed {
		return planCoordinates
	}
	if p.KeyLanguage.Waiting > 0 {
		return planKeyLanguage
	}
	for _, n := range p.KeyLanguage.Untranslated {
		if n > 0 {
			return planKeyLanguage
		}
	}
	if p.Exceptions.SourceFindings != nil && *p.Exceptions.SourceFindings > 0 {
		return planExceptions
	}
	for _, l := range p.Languages {
		if l.Failing > 0 {
			return planExceptions
		}
	}
	for _, l := range p.Languages {
		if p.Involvement != involvementContext && l.Pages > 0 {
			return planInterfaces
		}
		if l.Settleable > 0 {
			return planPolicy
		}
	}
	return ""
}

// planCoordinates reads the project's points from its collections.
func (s *Server) planCoordinates(ctx context.Context, proj *platstore.Project, stream string) (planCoordinatesDTO, error) {
	out := planCoordinatesDTO{
		SourceLanguage:  string(proj.DefaultSourceLanguage),
		TargetLanguages: []string{},
		Points:          []planPoint{},
		ConfirmedBy:     proj.Properties[propCoordinatesConfirmedBy],
		ConfirmedAt:     proj.Properties[propCoordinatesConfirmedAt],
	}
	out.Confirmed = out.ConfirmedBy != ""
	for _, l := range proj.TargetLanguages {
		out.TargetLanguages = append(out.TargetLanguages, string(l))
	}
	colls, err := s.ContentStore.ListCollections(ctx, proj.ID, stream)
	if err != nil {
		return out, err
	}
	voiceNames := map[string]string{}
	for _, c := range colls {
		pt := planPoint{
			Collection:  c.Name,
			Coordinates: map[string]string{},
			SourceOnly:  c.SourceOnly(),
			Priority:    c.Priority(),
		}
		maps.Copy(pt.Coordinates, c.Context)
		if id := c.ConnectorConfig[coreprofile.PropertyProfileID]; id != "" {
			name, seen := voiceNames[id]
			if !seen {
				name = id
				if s.VoiceStore != nil {
					if p, err := s.VoiceStore.GetProfile(ctx, id); err == nil && p != nil && p.Name != "" {
						name = p.Name
					}
				}
				voiceNames[id] = name
			}
			pt.Voice = name
		}
		out.Points = append(out.Points, pt)
	}
	slices.SortFunc(out.Points, func(a, b planPoint) int { return strings.Compare(a.Collection, b.Collection) })
	return out, nil
}

// planKeyLanguage counts the context waiting for the caller and the
// concepts that lack a term in a target language.
func (s *Server) planKeyLanguage(ctx context.Context, ws *platauth.Workspace, proj *platstore.Project, userID string, perms platauth.Permission, reach platauth.CoordinateReach) (planKeyLanguageDTO, error) {
	out := planKeyLanguageDTO{ByKind: map[string]int{}, Untranslated: map[string]int{}}
	entry, ok, err := s.contextInbox(ctx, ws, proj, userID, perms, reach)
	if err != nil {
		return out, err
	}
	if ok {
		out.Waiting, out.ByKind = entry.Waiting, entry.ByKind
	}
	tb, err := s.workspaceTermsByID(ctx, ws.ID)
	if err != nil || tb == nil {
		return out, nil //nolint:nilerr // a workspace with no terms store has no concepts to count
	}
	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return out, err
	}
	src := proj.DefaultSourceLanguage
	for _, loc := range proj.TargetLanguages {
		out.Untranslated[string(loc)] = 0
	}
	for i := range concepts {
		c := concepts[i]
		if c.HeadTerm(src) == nil {
			continue
		}
		out.Concepts++
		if c.DoNotTranslate {
			continue
		}
		for _, loc := range proj.TargetLanguages {
			if _, ok := terms.RuleForConcept(c, src, loc); !ok {
				out.Untranslated[string(loc)]++
			}
		}
	}
	return out, nil
}

// planExceptions sums the source findings the last project-wide check
// recorded.
func (s *Server) planExceptions(ctx context.Context, proj *platstore.Project, stream string) (planExceptionsDTO, error) {
	var out planExceptionsDTO
	ts, ok := platstore.ItemCheckTallies(s.ContentStore)
	if !ok {
		return out, nil
	}
	tallies, err := ts.ListItemCheckTallies(ctx, proj.ID, stream)
	if err != nil || len(tallies) == 0 {
		return out, err
	}
	findings := 0
	var at time.Time
	for _, t := range tallies {
		findings += t.Findings
		if t.CheckedAt.After(at) {
			at = t.CheckedAt
		}
	}
	out.SourceFindings = &findings
	out.CheckedAt = at.UTC().Format(time.RFC3339)
	return out, nil
}

// reviewLocalesOf is the project's target languages the caller may review:
// all of them with the review permission, narrowed to the languages the
// caller's grant names when it names any.
func reviewLocalesOf(proj *platstore.Project, perms platauth.Permission, langs []string) []string {
	if perms&platauth.PermReview == 0 {
		return nil
	}
	var out []string
	for _, l := range proj.TargetLanguages {
		if len(langs) == 0 || slices.Contains(langs, string(l)) {
			out = append(out, string(l))
		}
	}
	return out
}

// planLanguages counts, for each language, the deck's groups and pages.
func (s *Server) planLanguages(ctx context.Context, proj *platstore.Project, stream string, locales []string, involvement string) ([]planLanguageDTO, error) {
	out := []planLanguageDTO{}
	decks, ok := platstore.ReviewDeckStores(s.ContentStore)
	if !ok {
		return out, nil
	}
	scope, err := deckScopeOf(ctx, s.ContentStore, proj.ID, stream)
	if err != nil {
		return nil, err
	}
	policy := deckPolicyKey(s.deckPolicy())
	for _, loc := range locales {
		q := platstore.ReviewDeckQuery{ProjectID: proj.ID, Stream: stream, Locale: loc, Policy: policy}
		scope.apply(&q)
		counts, err := decks.CountReviewDeck(ctx, q)
		if err != nil {
			return nil, err
		}
		lang := planLanguageDTO{Locale: loc, deckWaiting: deckWaitingOf(counts)}
		lang.Failing = counts.ByRank[deckRank(reviewdeck.DeckDesk)]
		if involvement != involvementEvery {
			lang.Settleable = lang.Fast
		}
		pages, err := decks.ListReviewDeckPages(ctx, q, reviewPagesCountLimit+1)
		if err != nil {
			return nil, err
		}
		if len(pages) > reviewPagesCountLimit {
			lang.Pages, lang.PagesMore = reviewPagesCountLimit, true
		} else {
			lang.Pages = len(pages)
		}
		out = append(out, lang)
	}
	return out, nil
}

// reviewPlanInvolvementRequest sets the project's involvement.
type reviewPlanInvolvementRequest struct {
	Involvement string `json:"involvement"`
}

// HandlePutReviewInvolvement answers PUT /:ws/:id/review/plan/involvement
// {"involvement": "context"|"interfaces"|"every"}. It needs the permission to
// edit the project's settings.
func (s *Server) HandlePutReviewInvolvement(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageProject); err != nil {
		return err
	}
	var req reviewPlanInvolvementRequest
	if err := c.Bind(&req); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	switch req.Involvement {
	case involvementContext, involvementInterfaces, involvementEvery:
	default:
		return apiErr(c, http.StatusBadRequest, "involvement is context, interfaces or every")
	}
	return s.setPlanProperties(c, map[string]string{propReviewInvolvement: req.Involvement})
}

// HandleConfirmReviewCoordinates answers POST
// /:ws/:id/review/plan/coordinates: the caller confirms the project's
// coordinates as the plan lists them. It needs the permission to edit the
// project's settings.
func (s *Server) HandleConfirmReviewCoordinates(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageProject); err != nil {
		return err
	}
	userID, _ := c.Get("user_id").(string)
	return s.setPlanProperties(c, map[string]string{
		propCoordinatesConfirmedBy: userID,
		propCoordinatesConfirmedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

// setPlanProperties writes plan properties on the project and answers 204.
func (s *Server) setPlanProperties(c echo.Context, props map[string]string) error {
	if s.ContentStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "store not configured")
	}
	ctx := c.Request().Context()
	proj, err := s.ContentStore.GetProject(ctx, projectParam(c))
	if err != nil || proj == nil {
		return apiErr(c, http.StatusNotFound, errNoProject.Error())
	}
	if proj.Properties == nil {
		proj.Properties = map[string]string{}
	}
	maps.Copy(proj.Properties, props)
	if err := s.ContentStore.UpdateProject(ctx, proj); err != nil {
		return serverErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
