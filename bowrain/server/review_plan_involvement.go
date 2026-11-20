package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/sync/errgroup"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/host/reviewdeck"
)

// Involvement per coordinate. The project's involvement is the default, and
// a manager may override it at a point of the project's context space:
// product=docs at context, product=app with channel=ui at every. Each
// collection sits at one point (its coordinates), so involvement resolves
// once per collection. The plan's counts, the language decks' holds and the
// settle pass read it per collection, and hand the store collection ids.

// Resolution: an override applies to a collection when every axis it names
// has the value the collection holds on that axis. Of the overrides that
// apply, the one naming the most axes wins. Between two that name as many,
// the deeper involvement wins (every, then interfaces, then context), so a
// tie never leaves people less to review than either override asks for. A
// collection no override reaches, and a block in no collection, takes the
// project's involvement.

// propInvolvementOverrides holds the project's involvement overrides, as a
// JSON list of involvementOverride.
const propInvolvementOverrides = "review_involvement_overrides"

// involvementOverrideLimit bounds the overrides a project holds.
const involvementOverrideLimit = 50

// involvementOverride is the involvement at one point: the axes it names,
// each with one value.
type involvementOverride struct {
	Coordinates map[string]string `json:"coordinates"`
	Involvement string            `json:"involvement"`
}

// validInvolvement reports one of the three involvement levels.
func validInvolvement(v string) bool {
	switch v {
	case involvementContext, involvementInterfaces, involvementEvery:
		return true
	}
	return false
}

// involvementDepth orders the levels by how much they ask of people.
func involvementDepth(v string) int {
	switch v {
	case involvementEvery:
		return 2
	case involvementInterfaces:
		return 1
	}
	return 0
}

// involvementOverridesOf reads the project's overrides. An entry that does
// not parse, or names no axis or no valid level, is left out.
func involvementOverridesOf(p *platstore.Project) []involvementOverride {
	raw := p.Properties[propInvolvementOverrides]
	if raw == "" {
		return nil
	}
	var list []involvementOverride
	if json.Unmarshal([]byte(raw), &list) != nil {
		return nil
	}
	out := list[:0]
	for _, o := range list {
		if len(o.Coordinates) > 0 && validInvolvement(o.Involvement) {
			out = append(out, o)
		}
	}
	return out
}

// resolveInvolvement is the involvement at coords, and the index of the
// override that decides it (-1 for the project's own).
func resolveInvolvement(def string, overrides []involvementOverride, coords map[string]string) (string, int) {
	best, axes, depth := -1, 0, -1
	for i, o := range overrides {
		if !overrideApplies(o, coords) {
			continue
		}
		n, d := len(o.Coordinates), involvementDepth(o.Involvement)
		if n > axes || (n == axes && d > depth) {
			best, axes, depth = i, n, d
		}
	}
	if best < 0 {
		return def, -1
	}
	return overrides[best].Involvement, best
}

// overrideApplies reports whether every axis the override names has its
// value in coords.
func overrideApplies(o involvementOverride, coords map[string]string) bool {
	for axis, v := range o.Coordinates {
		if coords[axis] != v {
			return false
		}
	}
	return true
}

// projectInvolvement is a project's involvement resolved for each of its
// collections.
type projectInvolvement struct {
	def       string
	overrides []involvementOverride
	// byColl is the involvement of each collection, by id.
	byColl map[string]string
	// reach counts, per override, the collections it decides.
	reach []int
	// translated lists the collections that are not source only.
	translated []string
}

// involvementFor resolves the project's involvement over its collections.
func involvementFor(proj *platstore.Project, colls []*platstore.Collection) projectInvolvement {
	pi := projectInvolvement{
		def:       involvementOf(proj),
		overrides: involvementOverridesOf(proj),
		byColl:    make(map[string]string, len(colls)),
	}
	pi.reach = make([]int, len(pi.overrides))
	for _, c := range colls {
		if c == nil || c.ID == "" {
			continue
		}
		v, i := resolveInvolvement(pi.def, pi.overrides, c.Context)
		pi.byColl[c.ID] = v
		if i >= 0 {
			pi.reach[i]++
		}
		if !c.SourceOnly() {
			pi.translated = append(pi.translated, c.ID)
		}
	}
	return pi
}

// of is the involvement of a collection; the blocks in no collection ("")
// and a collection the project does not list take the project's own.
func (pi projectInvolvement) of(collectionID string) string {
	if v, ok := pi.byColl[collectionID]; ok {
		return v
	}
	if pi.def == "" {
		return involvementContext
	}
	return pi.def
}

// every lists the collections under every, sorted, with "" (the blocks in
// no collection) when the project's own involvement is every.
func (pi projectInvolvement) every() []string {
	var out []string
	if pi.def == involvementEvery {
		out = append(out, "")
	}
	for id, v := range pi.byColl {
		if v == involvementEvery {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

// allEvery reports a project that leaves nothing to the checks: its own
// involvement and every translated collection's are every.
func (pi projectInvolvement) allEvery() bool {
	if pi.def != involvementEvery {
		return false
	}
	for _, id := range pi.translated {
		if pi.byColl[id] != involvementEvery {
			return false
		}
	}
	return true
}

// heldIn reports whether a hold reaches the collection: its involvement
// leaves part of the work to the checks.
func (pi projectInvolvement) heldIn(collectionID string) bool {
	return pi.of(collectionID) != involvementEvery
}

// split divides a deck query by involvement: rest reads the collections a
// hold reaches, open the ones under every. Either is nil when it reads
// nothing, so a project with no override under every runs one query.
func (pi projectInvolvement) split(q platstore.ReviewDeckQuery) (rest, open *platstore.ReviewDeckQuery) {
	if q.CollectionID != nil {
		if pi.heldIn(*q.CollectionID) {
			return &q, nil
		}
		return nil, &q
	}
	every := pi.every()
	switch {
	case len(every) == 0:
		return &q, nil
	case pi.allEvery():
		return nil, &q
	}
	r, o := q, q
	r.ExcludeCollections = slices.Concat(q.ExcludeCollections, every)
	o.OnlyCollections = every
	return &r, &o
}

// storeHold is the store's hold on a deck while the plan holds
// translations back: the desk group alone, except in the collections under
// every.
func (pi projectInvolvement) storeHold() *platstore.ReviewDeckHold {
	return &platstore.ReviewDeckHold{Rank: deckRank(reviewdeck.DeckDesk), Open: pi.every()}
}

// involvementScopeOf reads a project's deck scope and involvement from one
// listing of its collections.
func (s *Server) involvementScopeOf(ctx context.Context, proj *platstore.Project, stream string) (deckScope, projectInvolvement, error) {
	colls, err := s.ContentStore.ListCollections(ctx, proj.ID, stream)
	if err != nil {
		return deckScope{}, projectInvolvement{}, fmt.Errorf("list collections: %w", err)
	}
	return deckScopeFrom(colls), involvementFor(proj, colls), nil
}

// deckSplit is a deck's counts divided by involvement: rest counts the
// collections a hold reaches, open the ones under every.
type deckSplit struct {
	rest, open platstore.ReviewDeckCounts
}

// waiting is what the deck deals: with held, rest's translations outside
// the desk group move to Held, and open's all wait.
func (d deckSplit) waiting(held bool) deckWaiting {
	w := deckWaitingOf(d.rest)
	if held {
		w = w.held()
	}
	return w.plus(deckWaitingOf(d.open))
}

// countDeckSplit counts one language's deck by involvement.
func countDeckSplit(ctx context.Context, decks platstore.ReviewDeckStore, q platstore.ReviewDeckQuery, pi projectInvolvement) (deckSplit, error) {
	var out deckSplit
	rest, open := pi.split(q)
	var err error
	if rest != nil {
		if out.rest, err = decks.CountReviewDeck(ctx, *rest); err != nil {
			return out, err
		}
	}
	if open != nil {
		if out.open, err = decks.CountReviewDeck(ctx, *open); err != nil {
			return out, err
		}
	}
	return out, nil
}

// countDeckSplitByLocale counts each language's deck by involvement, one
// query per part, the parts at once. Every locale asked for is present.
func countDeckSplitByLocale(ctx context.Context, decks platstore.ReviewDeckStore, q platstore.ReviewDeckQuery, locales []string, pi projectInvolvement) (map[string]deckSplit, error) {
	rest, open := pi.split(q)
	var restN, openN map[string]platstore.ReviewDeckCounts
	g, gctx := errgroup.WithContext(ctx)
	if rest != nil {
		g.Go(func() (err error) {
			restN, err = decks.CountReviewDeckByLocale(gctx, *rest, locales)
			return err
		})
	}
	if open != nil {
		g.Go(func() (err error) {
			openN, err = decks.CountReviewDeckByLocale(gctx, *open, locales)
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	out := make(map[string]deckSplit, len(locales))
	for _, loc := range locales {
		out[loc] = deckSplit{rest: restN[loc], open: openN[loc]}
	}
	return out, nil
}

// plus adds another deck's counts.
func (w deckWaiting) plus(o deckWaiting) deckWaiting {
	w.Waiting += o.Waiting
	w.Fast += o.Fast
	w.Closer += o.Closer
	w.Desk += o.Desk
	w.Unclassified += o.Unclassified
	w.Held += o.Held
	return w
}

// interfacePages is the pages a reviewer reads: the sample in a collection
// under context (see interfaceSample), every page elsewhere, in deck order.
func interfacePages(pages []platstore.ReviewDeckPage, pi projectInvolvement) []platstore.ReviewDeckPage {
	return interfaceSampleWhere(pages, func(collectionID string) bool {
		return pi.of(collectionID) == involvementContext
	})
}

// involvementOverrideDTO is one override as the API shows it, with the
// collections it decides.
type involvementOverrideDTO struct {
	involvementOverride
	Collections int `json:"collections"`
}

// overridesDTO lists the project's overrides with their reach.
func (pi projectInvolvement) overridesDTO() []involvementOverrideDTO {
	out := make([]involvementOverrideDTO, 0, len(pi.overrides))
	for i, o := range pi.overrides {
		out = append(out, involvementOverrideDTO{involvementOverride: o, Collections: pi.reach[i]})
	}
	return out
}

// reviewInvolvementResponse is the project's involvement, its overrides and
// the axes an override may name, each with the values the project's
// collections hold.
type reviewInvolvementResponse struct {
	Involvement string                   `json:"involvement"`
	Overrides   []involvementOverrideDTO `json:"overrides"`
	Axes        map[string][]string      `json:"axes"`
}

// reviewInvolvementOverridesRequest replaces the project's overrides.
type reviewInvolvementOverridesRequest struct {
	Overrides []involvementOverride `json:"overrides"`
}

// HandleGetReviewInvolvement answers GET
// /:ws/:id/review/plan/involvement with the project's involvement, its
// overrides and the axes an override may name.
func (s *Server) HandleGetReviewInvolvement(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.ContentStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "store not configured")
	}
	ctx := c.Request().Context()
	proj, err := s.ContentStore.GetProject(ctx, projectParam(c))
	if err != nil || proj == nil {
		return apiErr(c, http.StatusNotFound, errNoProject.Error())
	}
	out, err := s.reviewInvolvement(ctx, proj)
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// reviewInvolvement reads the involvement the main stream's collections
// resolve.
func (s *Server) reviewInvolvement(ctx context.Context, proj *platstore.Project) (reviewInvolvementResponse, error) {
	colls, err := s.ContentStore.ListCollections(ctx, proj.ID, reviewInboxStream)
	if err != nil {
		return reviewInvolvementResponse{}, err
	}
	pi := involvementFor(proj, colls)
	return reviewInvolvementResponse{
		Involvement: pi.def,
		Overrides:   pi.overridesDTO(),
		Axes:        coordinateAxesOf(colls),
	}, nil
}

// coordinateAxesOf lists each axis the collections declare with the values
// they hold on it, sorted.
func coordinateAxesOf(colls []*platstore.Collection) map[string][]string {
	out := map[string][]string{}
	for _, c := range colls {
		if c == nil {
			continue
		}
		for axis, v := range c.Context {
			if v != "" && !slices.Contains(out[axis], v) {
				out[axis] = append(out[axis], v)
			}
		}
	}
	for _, vs := range out {
		slices.Sort(vs)
	}
	return out
}

// HandlePutReviewInvolvementOverrides answers PUT
// /:ws/:id/review/plan/involvement/overrides {"overrides": [{"coordinates":
// {"product": "docs"}, "involvement": "context"}]}: it replaces the
// project's overrides and answers as the GET does. Each override names at
// least one axis the project's collections declare, with a value one of them
// holds there, and no two name the same point. It needs the permission to
// edit the project's settings, and records the caller as the person who set
// the involvement.
func (s *Server) HandlePutReviewInvolvementOverrides(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageProject); err != nil {
		return err
	}
	if s.ContentStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "store not configured")
	}
	var req reviewInvolvementOverridesRequest
	if err := c.Bind(&req); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	ctx := c.Request().Context()
	proj, err := s.ContentStore.GetProject(ctx, projectParam(c))
	if err != nil || proj == nil {
		return apiErr(c, http.StatusNotFound, errNoProject.Error())
	}
	colls, err := s.ContentStore.ListCollections(ctx, proj.ID, reviewInboxStream)
	if err != nil {
		return serverErr(c, err)
	}
	list, err := validateOverrides(req.Overrides, coordinateAxesOf(colls))
	if err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	raw, err := json.Marshal(list)
	if err != nil {
		return serverErr(c, err)
	}
	userID, _ := c.Get("user_id").(string)
	if err := s.updatePlanProperties(ctx, proj.ID, map[string]string{
		propInvolvementOverrides: string(raw),
		propInvolvementSetBy:     userID,
		propInvolvementSetAt:     time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		return serverErr(c, err)
	}
	if proj, err = s.ContentStore.GetProject(ctx, proj.ID); err != nil || proj == nil {
		return apiErr(c, http.StatusNotFound, errNoProject.Error())
	}
	out, err := s.reviewInvolvement(ctx, proj)
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// validateOverrides checks overrides against the project's axes and answers
// them in a stable order: fewer axes first, then by point.
func validateOverrides(in []involvementOverride, axes map[string][]string) ([]involvementOverride, error) {
	if len(in) > involvementOverrideLimit {
		return nil, fmt.Errorf("a project holds at most %d overrides", involvementOverrideLimit)
	}
	out := make([]involvementOverride, 0, len(in))
	seen := map[string]bool{}
	for _, o := range in {
		if !validInvolvement(o.Involvement) {
			return nil, errors.New("involvement is context, interfaces or every")
		}
		if len(o.Coordinates) == 0 {
			return nil, errors.New("an override names at least one axis")
		}
		coords := make(map[string]string, len(o.Coordinates))
		for axis, v := range o.Coordinates {
			axis, v = strings.TrimSpace(axis), strings.TrimSpace(v)
			values, ok := axes[axis]
			if !ok {
				return nil, fmt.Errorf("the project declares no axis %q", axis)
			}
			if !slices.Contains(values, v) {
				return nil, fmt.Errorf("no collection sits at %s=%s", axis, v)
			}
			coords[axis] = v
		}
		key := overridePoint(coords)
		if seen[key] {
			return nil, fmt.Errorf("two overrides name %s", key)
		}
		seen[key] = true
		out = append(out, involvementOverride{Coordinates: coords, Involvement: o.Involvement})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if a, b := len(out[i].Coordinates), len(out[j].Coordinates); a != b {
			return a < b
		}
		return overridePoint(out[i].Coordinates) < overridePoint(out[j].Coordinates)
	})
	return out, nil
}

// overridePoint names a point by its axes in order: "channel=ui,product=app".
func overridePoint(coords map[string]string) string {
	keys := slices.Sorted(maps.Keys(coords))
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+coords[k])
	}
	return strings.Join(parts, ",")
}
