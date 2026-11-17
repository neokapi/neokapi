package server

import (
	"cmp"
	"context"
	"net/http"
	"slices"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/host/reviewdeck"
)

// reviewInboxLanguage is one language of one project with translations
// waiting for the caller's review.
// reviewInboxStream is the stream the inbox counts: the one a review client
// opens when it names none.
const reviewInboxStream = "main"

type reviewInboxLanguage struct {
	Workspace     string `json:"workspace"`
	WorkspaceName string `json:"workspace_name"`
	Project       string `json:"project"`
	ProjectName   string `json:"project_name"`
	Stream        string `json:"stream"`
	Locale        string `json:"locale"`
	deckWaiting
}

// deckWaiting is what a language's review deck holds, split by group. The
// inbox, the deck and the deck's collections all count with it, so one
// language shows one number wherever a client asks.
type deckWaiting struct {
	// Waiting is every translation the deck deals: the ones classified into
	// a group and the ones not classified yet. A translation set aside is
	// not counted.
	Waiting int `json:"waiting"`
	Fast    int `json:"fast"`
	Closer  int `json:"closer"`
	Desk    int `json:"desk"`
	// Unclassified is the part of Waiting the deck has not grouped yet. The
	// deck groups it when the language is opened.
	Unclassified int `json:"unclassified"`
}

// deckWaitingOf reads the store's deck counts.
func deckWaitingOf(n store.ReviewDeckCounts) deckWaiting {
	w := deckWaiting{
		Fast:         n.ByRank[deckRank(reviewdeck.DeckFast)],
		Closer:       n.ByRank[deckRank(reviewdeck.DeckCloser)],
		Desk:         n.ByRank[deckRank(reviewdeck.DeckDesk)],
		Unclassified: n.Unclassified,
	}
	w.Waiting = w.Fast + w.Closer + w.Desk + w.Unclassified
	return w
}

type reviewInboxResponse struct {
	// Context lists the projects whose context waits for the caller's
	// review, most waiting first. A client shows it before the languages:
	// the context a translation is checked against is reviewed first.
	Context   []contextInboxEntry   `json:"context"`
	Languages []reviewInboxLanguage `json:"languages"`
}

// HandleReviewInbox answers, for the signed-in person, every language they may
// review in every project of every workspace they belong to, where anything is
// waiting, most waiting first. A language counts as the review deck counts it,
// so the number here is the number the deck shows on opening.
//
// Access resolves as ProjectAccessMiddleware resolves it for a project route,
// and an API token's scopes narrow it as they narrow a project route. It
// counts from the deck's stored facts and classifies nothing, so the cost is
// one count per reviewable language.
//
// GET /api/v1/auth/me/review
func (s *Server) HandleReviewInbox(c echo.Context) error {
	if s.AuthStore == nil || s.ContentStore == nil || s.Services == nil {
		return apiErr(c, http.StatusServiceUnavailable, "store not configured")
	}
	decks, ok := store.ReviewDeckStores(s.ContentStore)
	if !ok {
		return apiErr(c, http.StatusServiceUnavailable, "the review inbox needs the PostgreSQL content store")
	}
	userID, _ := c.Get("user_id").(string)
	if userID == "" {
		return apiErr(c, http.StatusUnauthorized, "not authenticated")
	}
	ctx := c.Request().Context()

	workspaces, err := s.AuthStore.ListWorkspaces(ctx, userID)
	if err != nil {
		return serverErr(c, err)
	}
	projects, err := s.Services.Project.ListProjects(ctx)
	if err != nil {
		return serverErr(c, err)
	}
	scopes, _ := c.Get("api_token_scopes").(string)
	policy := deckPolicyKey(s.deckPolicy())

	out := reviewInboxResponse{Context: []contextInboxEntry{}, Languages: []reviewInboxLanguage{}}
	for _, ws := range workspaces {
		for _, p := range projects {
			if p.WorkspaceID != ws.ID || p.Archived {
				continue
			}
			perms, reach := s.inboxProjectAccess(ctx, userID, ws, p, scopes)
			entry, ok, err := s.contextInbox(ctx, ws, p, userID, perms, reach)
			if err != nil {
				return serverErr(c, err)
			}
			if ok {
				out.Context = append(out.Context, entry)
			}
			locales := s.reviewableLocales(ctx, userID, ws, p, scopes)
			scope, err := deckScopeOf(ctx, s.ContentStore, p.ID, reviewInboxStream)
			if err != nil {
				return serverErr(c, err)
			}
			for _, loc := range locales {
				q := store.ReviewDeckQuery{
					ProjectID: p.ID,
					Stream:    reviewInboxStream,
					Locale:    loc,
					Policy:    policy,
				}
				scope.apply(&q)
				counts, err := decks.CountReviewDeck(ctx, q)
				if err != nil {
					return serverErr(c, err)
				}
				lang := reviewInboxLanguage{
					Workspace:     ws.Slug,
					WorkspaceName: ws.Name,
					Project:       p.ID,
					ProjectName:   p.Name,
					Stream:        reviewInboxStream,
					Locale:        loc,
					deckWaiting:   deckWaitingOf(counts),
				}
				if lang.Waiting > 0 {
					out.Languages = append(out.Languages, lang)
				}
			}
		}
	}
	slices.SortStableFunc(out.Context, func(a, b contextInboxEntry) int {
		return cmp.Or(cmp.Compare(b.Waiting, a.Waiting), cmp.Compare(a.ProjectName, b.ProjectName))
	})
	slices.SortStableFunc(out.Languages, func(a, b reviewInboxLanguage) int {
		return cmp.Or(
			cmp.Compare(b.Waiting, a.Waiting),
			cmp.Compare(a.ProjectName, b.ProjectName),
			cmp.Compare(a.Locale, b.Locale),
		)
	})
	return c.JSON(http.StatusOK, out)
}

// inboxProjectAccess is what the user may do in the project and the
// coordinates their grants reach, as resolveProjectAccess resolves it and an
// API token's scopes narrow it.
func (s *Server) inboxProjectAccess(ctx context.Context, userID string, ws *platauth.Workspace, p *store.Project, scopes string) (platauth.Permission, platauth.CoordinateReach) {
	access := s.resolveProjectAccess(ctx, accessRequest{
		userID: userID, projectID: p.ID, workspaceID: ws.ID, role: ws.Role, plan: ws.Plan,
	})
	perms := access.permissions
	if scopes != "" {
		if resolved, err := platauth.ParseScopes(scopes); err == nil && !resolved.IsFullAccess {
			perms &= resolved.Permissions
		}
	}
	return perms, access.coordinates
}

// reviewableLocales is the project's target languages the user may review:
// the review permission as resolveProjectAccess resolves it, narrowed by an
// API token's scopes, then by the languages the permission reaches.
func (s *Server) reviewableLocales(ctx context.Context, userID string, ws *platauth.Workspace, p *store.Project, scopes string) []string {
	access := s.resolveProjectAccess(ctx, accessRequest{
		userID: userID, projectID: p.ID, workspaceID: ws.ID, role: ws.Role, plan: ws.Plan,
	})
	perms, languages := access.permissions, access.languages
	if scopes != "" {
		if resolved, err := platauth.ParseScopes(scopes); err == nil && !resolved.IsFullAccess {
			perms &= resolved.Permissions
			if len(resolved.Languages) > 0 {
				if len(languages) == 0 {
					languages = resolved.Languages
				} else {
					languages = intersectStrings(languages, resolved.Languages)
				}
			}
		}
	}
	if !perms.Has(platauth.PermReview) {
		return nil
	}
	var out []string
	for _, l := range p.TargetLanguages {
		if len(languages) == 0 || slices.Contains(languages, string(l)) {
			out = append(out, string(l))
		}
	}
	return out
}
