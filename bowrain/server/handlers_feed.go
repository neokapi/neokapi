package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// The client screens a feed entry can open: a closed list. A link names a
// screen and its arguments, never a URL, so web and mobile each route it.
const (
	FeedScreenStream      = "stream"
	FeedScreenRun         = "run"
	FeedScreenReviewDeck  = "review_deck"
	FeedScreenRecipeDiff  = "recipe_diff"
	FeedScreenContextDeck = "context_deck"
	FeedScreenChangeSet   = "change_set"
	FeedScreenLanguage    = "language"
)

// FeedActor is who did what a feed entry records, as recorded.
type FeedActor struct {
	ID string `json:"id"`
	// Kind is person, agent or tool.
	Kind    string `json:"kind"`
	Name    string `json:"name,omitempty"`
	Session string `json:"session,omitempty"`
	Host    string `json:"host,omitempty"`
}

// FeedMember is the signed-in member an agent or tool acted for.
type FeedMember struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// FeedLink names the client screen an entry opens and what to open there.
type FeedLink struct {
	Screen string `json:"screen"`
	// ID names the object on the screen: a push, a run, a change set, the
	// entry itself for a recipe diff.
	ID     string `json:"id,omitempty"`
	Stream string `json:"stream,omitempty"`
	Locale string `json:"locale,omitempty"`
	// Mode is the context deck's mode, pending or replay.
	Mode  string   `json:"mode,omitempty"`
	Items []string `json:"items,omitempty"`
}

// FeedEntry is one entry of a project's feed. It carries counts and names,
// never a sentence: a client renders each in its own language.
type FeedEntry struct {
	ID         string      `json:"id"`
	Kind       string      `json:"kind"`
	At         time.Time   `json:"at"`
	Actor      FeedActor   `json:"actor"`
	OnBehalfOf *FeedMember `json:"on_behalf_of,omitempty"`
	Locales    []string    `json:"locales,omitempty"`
	// Coordinates is the point the entry sits at (brand, product, channel,
	// market).
	Coordinates map[string]string `json:"coordinates,omitempty"`
	Counts      map[string]int    `json:"counts"`
	// Stream is the stream a push, run or review happened on.
	Stream string `json:"stream,omitempty"`
	// State is a run's outcome (converged, parked, failed, canceled) or a
	// change set's step (submitted, approved, rejected, merged, created,
	// abandoned, superseded).
	State string `json:"state,omitempty"`
	// Diff is a recipe change's values before and after.
	Diff *platev.RecipeChange `json:"diff,omitempty"`
	// Samples names the first few items a context change touched; counts
	// holds the full number.
	Samples []platev.ContextSample `json:"samples,omitempty"`
	// Standing is a run's result per language, one entry per target
	// language.
	Standing []FeedStanding `json:"standing,omitempty"`
	// Name is a change set's name.
	Name string   `json:"name,omitempty"`
	Link FeedLink `json:"link"`
}

// FeedStanding is one language's standing at the end of a run.
type FeedStanding struct {
	Locale string `json:"locale"`
	// State is shippable, parked or pending.
	State    string `json:"state"`
	Units    int    `json:"units,omitempty"`
	Produced int    `json:"produced,omitempty"`
}

// FeedResponse is one page of a project's feed, newest first. NextCursor is
// omitted on the last page.
type FeedResponse struct {
	Entries    []FeedEntry `json:"entries"`
	NextCursor string      `json:"next_cursor,omitempty"`
}

// HandleProjectFeed lists a project's feed: GET /:ws/:id/feed.
//
// The feed is the activities rows with a feed kind, filed for this project.
// Filters: kind (comma-separated), locale, coordinate (axis:value,
// repeatable), actor (a user id), actor_kind and since (RFC 3339). Paging is
// the (created_at, id) cursor the activity store issues.
func (s *Server) HandleProjectFeed(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.ActivityStore == nil {
		return c.JSON(http.StatusOK, FeedResponse{Entries: []FeedEntry{}})
	}
	ctx := c.Request().Context()
	ws := c.Param("ws")

	q := bstore.ActivityQuery{
		WorkspaceID:   ws,
		WorkspaceKeys: []string{s.activityWorkspaceID(ctx, c, ws)},
		ProjectID:     projectParam(c),
		FeedOnly:      true,
		Locale:        c.QueryParam("locale"),
		ActorID:       c.QueryParam("actor"),
		ActorKind:     c.QueryParam("actor_kind"),
		Cursor:        c.QueryParam("cursor"),
	}
	q.Limit, _ = pageParams(c, 50, maxListPageSize)

	for _, k := range csvParam(c, "kind") {
		kind := bstore.FeedKind(k)
		if !slices.Contains(bstore.FeedKinds, kind) {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "unknown feed kind " + k})
		}
		q.Kinds = append(q.Kinds, kind)
	}
	switch q.ActorKind {
	case "", bstore.ActorKindPerson, bstore.ActorKindAgent, bstore.ActorKindTool:
	default:
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "actor_kind is person, agent or tool"})
	}
	for _, raw := range c.QueryParams()["coordinate"] {
		axis, value, ok := strings.Cut(raw, ":")
		if !ok || axis == "" || value == "" {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "coordinate is axis:value"})
		}
		if q.Coordinates == nil {
			q.Coordinates = map[string]string{}
		}
		q.Coordinates[axis] = value
	}
	if raw := c.QueryParam("since"); raw != "" {
		since, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "since is an RFC 3339 time"})
		}
		q.Since = since
	}

	result, err := s.ActivityStore.List(ctx, q)
	if errors.Is(err, bstore.ErrInvalidActivityCursor) {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "cursor is not one this endpoint returned"})
	}
	if err != nil {
		return serverErr(c, err)
	}

	names := s.feedMemberNames(ctx)
	resp := FeedResponse{Entries: make([]FeedEntry, 0, len(result.Activities)), NextCursor: result.NextCursor}
	for i := range result.Activities {
		resp.Entries = append(resp.Entries, feedEntryOf(&result.Activities[i], names))
	}
	return c.JSON(http.StatusOK, resp)
}

// feedMemberNames returns a lookup of a user's display name that reads each
// user at most once per request.
func (s *Server) feedMemberNames(ctx context.Context) func(string) string {
	cache := map[string]string{}
	return func(userID string) string {
		if userID == "" || userID == "system" || s.AuthStore == nil {
			return ""
		}
		if name, ok := cache[userID]; ok {
			return name
		}
		name := ""
		if u, err := s.AuthStore.GetUser(ctx, userID); err == nil && u != nil {
			name = u.Name
			if name == "" {
				name = u.Email
			}
		}
		cache[userID] = name
		return name
	}
}

// feedEntryOf renders one activity row as a feed entry.
func feedEntryOf(a *bstore.Activity, memberName func(string) string) FeedEntry {
	e := FeedEntry{
		ID:          a.ID,
		Kind:        string(a.Kind),
		At:          a.CreatedAt,
		Locales:     a.Locales,
		Coordinates: a.Coordinates,
		Counts:      a.Counts,
		Stream:      a.Stream,
		Actor: FeedActor{
			ID:      a.ActorID,
			Kind:    a.ActorKind,
			Name:    a.ActorName,
			Session: a.Session,
			Host:    a.Data["host"],
		},
	}
	if e.Counts == nil {
		e.Counts = map[string]int{}
	}
	if e.Actor.Kind == "" {
		e.Actor.Kind = bstore.ActorKindPerson
	}
	if e.Actor.Kind == bstore.ActorKindPerson && e.Actor.Name == "" {
		e.Actor.Name = memberName(a.ActorID)
	}
	// An agent or a tool that ran under a member's login names the member.
	if e.Actor.Kind != bstore.ActorKindPerson {
		if name := memberName(a.ActorID); name != "" {
			e.OnBehalfOf = &FeedMember{ID: a.ActorID, Name: name}
		}
	}

	switch a.Kind {
	case bstore.FeedKindPush:
		e.Link = FeedLink{Screen: FeedScreenStream, ID: a.EntityID, Stream: a.Stream}
	case bstore.FeedKindConvergenceRun:
		e.State = a.Data["state"]
		if raw := a.Data["standing"]; raw != "" {
			_ = json.Unmarshal([]byte(raw), &e.Standing)
		}
		e.Link = FeedLink{Screen: FeedScreenRun, ID: a.EntityID}
	case bstore.FeedKindReviewSummary:
		locale := a.EntityID
		e.Link = FeedLink{Screen: FeedScreenReviewDeck, Locale: locale, Stream: a.Stream}
	case bstore.FeedKindRecipeChange:
		diff := platev.ParseRecipeChange(a.Data)
		e.Diff = &diff
		e.Link = FeedLink{Screen: FeedScreenRecipeDiff, ID: a.ID}
	case bstore.FeedKindContextChange:
		change := platev.ParseContextChange(a.Data)
		e.Samples = change.Samples
		link := FeedLink{Screen: FeedScreenContextDeck, Mode: contextDeckMode(change)}
		if change.ItemID != "" {
			link.Items = []string{change.ItemID}
		}
		e.Link = link
	case bstore.FeedKindChangeSet:
		e.State = a.Data["state"]
		e.Name = a.Data["changeset_name"]
		e.Link = FeedLink{Screen: FeedScreenChangeSet, ID: a.EntityID}
	case bstore.FeedKindCoverageMilestone:
		locale := a.EntityID
		if locale == "" && len(a.Locales) > 0 {
			locale = a.Locales[0]
		}
		e.Link = FeedLink{Screen: FeedScreenLanguage, Locale: locale}
	}
	return e
}

// contextDeckMode is the deck a context change opens: Pending for items a
// push added, which wait for a decision, and Replay for everything else.
func contextDeckMode(c platev.ContextChange) string {
	if c.Source == "push" && c.Operation == platev.ContextOpAdd {
		return "pending"
	}
	return "replay"
}
