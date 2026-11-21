package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/host/reviewdeck"
)

// The review plan's snapshot: the counts of a project's plan that read its
// translations, its check tallies and its context, stored once per project
// for the main stream. The plan route and the review inbox read it and add
// what depends on the caller (the languages they follow, the context they may
// keep), so neither counts a translation per request.

// The server recomputes a snapshot in the background after changes. Each
// event about a project marks its snapshot changed; a snapshot is due once no
// change has arrived for planSnapshotConfig.quiet, or once its first
// uncounted change is planSnapshotConfig.maxWait old, so a burst of decisions
// costs one compute. Every instance polls for due snapshots and claims one at
// a time; the claim is a lease in the database, so two instances never
// compute the same snapshot at once, and a change during a compute leaves the
// snapshot due again.

// A project with no snapshot, or one taken under other inputs (another deck
// policy, other target languages, collections or involvement), is counted
// on the request and stored. A branch stream is always counted on the
// request. The settle pass and the deck holds read their own counts, never
// the snapshot, because they decide on them.

// planSnapshotGroup is the durable consumer group that hears the changes a
// snapshot counts. Stable by design: the group's cursor is keyed by it.
const planSnapshotGroup = "review-plan-snapshot"

// planSnapshotVersion names the snapshot's shape. A snapshot of another
// shape is counted again on its next read.
const planSnapshotVersion = "1"

// planSnapshotConfig times the snapshot's refresh.
type planSnapshotConfig struct {
	// quiet is how long a snapshot waits after its last change.
	quiet time.Duration
	// maxWait is the longest a change waits under continuous change.
	maxWait time.Duration
	// poll is how often an instance looks for due snapshots and writes the
	// changes it heard.
	poll time.Duration
	// lease bounds one compute: a claim not completed by then is free.
	lease time.Duration
	// retry spaces out the computes of a snapshot that keeps failing.
	retry time.Duration
	// maxAge has a read mark a snapshot changed when no change has reached
	// it for this long, which bounds what a change the events missed costs.
	maxAge time.Duration
}

var defaultPlanSnapshotConfig = planSnapshotConfig{
	quiet:   3 * time.Second,
	maxWait: 10 * time.Second,
	poll:    time.Second,
	lease:   2 * time.Minute,
	retry:   30 * time.Second,
	maxAge:  10 * time.Minute,
}

// planSnapshot is the stored counts of a project's plan.
type planSnapshot struct {
	// Fingerprint names the inputs the counts were taken under
	// (planFingerprint).
	Fingerprint string `json:"fingerprint"`
	// Exceptions is the last project-wide check, source rules only.
	Exceptions planExceptionsDTO `json:"exceptions"`
	// Concepts, Untranslated and Proposable count the workspace's concepts
	// as planContextDTO does.
	Concepts     int            `json:"concepts"`
	Untranslated map[string]int `json:"untranslated"`
	Proposable   int            `json:"proposable"`
	// ContextOpen is the context cards waiting for any reviewer.
	ContextOpen int `json:"context_open"`
	// Languages counts each target language's deck.
	Languages map[string]planSnapshotLanguage `json:"languages"`
	// asOf is when the counts were taken.
	asOf time.Time
}

// planSnapshotLanguage is one language's deck counts.
type planSnapshotLanguage struct {
	// Rest counts the collections a hold reaches, Open the ones under every.
	Rest platstore.ReviewDeckCounts `json:"rest"`
	Open platstore.ReviewDeckCounts `json:"open"`
	// Rules is the rules behind the desk group, most findings first.
	Rules             []platstore.RuleTally `json:"rules,omitempty"`
	Pages             int                   `json:"pages"`
	PagesMore         bool                  `json:"pages_more,omitempty"`
	PagesTotal        int                   `json:"pages_total"`
	PagesWithFindings int                   `json:"pages_with_findings"`
}

// split is the language's counts as the plan divides them.
func (l planSnapshotLanguage) split() deckSplit {
	return deckSplit{rest: l.Rest, open: l.Open}
}

// planFingerprint covers the deck policy, languages, collection scope and
// the involvement that determines each collection's held work and page sample.
func planFingerprint(policy string, proj *platstore.Project, scope deckScope, pi projectInvolvement) string {
	h := sha256.New()
	fmt.Fprintf(h, "%q\n%q\n%q\n%q\n", planSnapshotVersion, policy, proj.DefaultSourceLanguage, slices.Sorted(slices.Values(proj.TargetLanguages)))
	for _, ids := range [][]string{scope.Exclude, scope.High, scope.Low} {
		fmt.Fprintf(h, "%q\n", slices.Sorted(slices.Values(ids)))
	}
	fmt.Fprintf(h, "%q\n%q\n", pi.def, pi.byColl)
	return hex.EncodeToString(h.Sum(nil)[:12])
}

// computePlanSnapshot counts a project's plan over every target language.
// The reads are independent, so they run at once.
func (s *Server) computePlanSnapshot(ctx context.Context, ws *platauth.Workspace, proj *platstore.Project, stream string, scope deckScope, pi projectInvolvement, fingerprint string) (planSnapshot, error) {
	out := planSnapshot{Fingerprint: fingerprint, Untranslated: map[string]int{}, Languages: map[string]planSnapshotLanguage{}}
	locales := make([]string, 0, len(proj.TargetLanguages))
	for _, l := range proj.TargetLanguages {
		locales = append(locales, string(l))
	}
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) {
		out.Exceptions, err = s.planExceptions(gctx, proj, stream)
		return err
	})
	g.Go(func() error {
		k, err := s.planConceptCounts(gctx, ws, proj)
		out.Concepts, out.Untranslated, out.Proposable = k.Concepts, k.Untranslated, k.Proposable
		return err
	})
	g.Go(func() (err error) {
		out.ContextOpen, err = s.projectContextOpen(gctx, ws, proj)
		return err
	})
	g.Go(func() (err error) {
		out.Languages, err = s.countPlanLanguages(gctx, proj, stream, locales, scope, pi)
		return err
	})
	if err := g.Wait(); err != nil {
		return planSnapshot{}, err
	}
	out.asOf = time.Now().UTC()
	return out, nil
}

// countPlanLanguages counts, for each language, the deck's groups split by
// involvement, the rules behind its desk group and its pages. Each read is one
// query over every language; the deck's counts take a second when some
// collections are under every and others are not.
func (s *Server) countPlanLanguages(ctx context.Context, proj *platstore.Project, stream string, locales []string, scope deckScope, pi projectInvolvement) (map[string]planSnapshotLanguage, error) {
	out := map[string]planSnapshotLanguage{}
	decks, ok := platstore.ReviewDeckStores(s.ContentStore)
	if !ok || len(locales) == 0 {
		return out, nil
	}
	q := platstore.ReviewDeckQuery{ProjectID: proj.ID, Stream: stream, Policy: deckPolicyKey(s.deckPolicy())}
	scope.apply(&q)
	desk := deckRank(reviewdeck.DeckDesk)
	var (
		counts  map[string]deckSplit
		top     map[string][]platstore.RuleTally
		pagesOf map[string][]platstore.ReviewDeckPage
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) {
		counts, err = countDeckSplitByLocale(gctx, decks, q, locales, pi)
		return err
	})
	g.Go(func() (err error) {
		top, err = decks.CountReviewDeckRulesByLocale(gctx, q, locales, desk, planRuleLimit)
		return err
	})
	g.Go(func() (err error) {
		pagesOf, err = decks.ListReviewDeckPagesByLocale(gctx, q, locales, reviewPagesCountLimit+1)
		return err
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}
	for _, loc := range locales {
		n := counts[loc]
		lang := planSnapshotLanguage{Rest: n.rest, Open: n.open, Rules: top[loc]}
		pages := pagesOf[loc]
		if len(pages) > reviewPagesCountLimit {
			pages, lang.PagesMore = pages[:reviewPagesCountLimit], true
		}
		lang.PagesTotal = len(pages)
		for _, p := range pages {
			if p.Findings > 0 {
				lang.PagesWithFindings++
			}
		}
		lang.Pages = len(interfacePages(pages, pi))
		out[loc] = lang
	}
	return out, nil
}

// planBasis is what a plan is built from: the stream's collections, the deck
// scope and involvement they resolve, and the counts.
type planBasis struct {
	colls []*platstore.Collection
	scope deckScope
	pi    projectInvolvement
	snap  planSnapshot
}

// planBasisOf reads the stream's collections and the counts of its plan.
func (s *Server) planBasisOf(ctx context.Context, ws *platauth.Workspace, proj *platstore.Project, stream string) (planBasis, error) {
	colls, err := s.ContentStore.ListCollections(ctx, proj.ID, stream)
	if err != nil {
		return planBasis{}, err
	}
	b := planBasis{colls: colls, scope: deckScopeFrom(colls), pi: involvementFor(proj, colls)}
	b.snap, err = s.planSnapshotFor(ctx, ws, proj, stream, b.scope, b.pi)
	return b, err
}

// planSnapshotFor answers the counts of a project's plan: the stored
// snapshot when it was taken under the same inputs, else counted now and
// stored. Without a running refresh, and on a branch stream, it counts on
// every call.
func (s *Server) planSnapshotFor(ctx context.Context, ws *platauth.Workspace, proj *platstore.Project, stream string, scope deckScope, pi projectInvolvement) (planSnapshot, error) {
	fp := planFingerprint(deckPolicyKey(s.deckPolicy()), proj, scope, pi)
	r := s.planSnapshots
	if r == nil || stream != reviewInboxStream {
		return s.computePlanSnapshot(ctx, ws, proj, stream, scope, pi, fp)
	}
	if snap, ok := r.read(ctx, proj.ID, stream, fp); ok {
		return snap, nil
	}
	v, err, _ := r.flight.Do(proj.ID+"\x00"+stream+"\x00"+fp, func() (any, error) {
		return r.computeNow(ctx, ws, proj, stream, scope, pi, fp)
	})
	if err != nil {
		return planSnapshot{}, err
	}
	return v.(planSnapshot).clone(), nil
}

// clone copies the snapshot's maps, so callers sharing one compute never
// share a map.
func (p planSnapshot) clone() planSnapshot {
	p.Untranslated = maps.Clone(p.Untranslated)
	p.Languages = maps.Clone(p.Languages)
	return p
}

// touchPlanSnapshot records a change to a project's snapshot that no event
// carries.
func (s *Server) touchPlanSnapshot(projectID string) {
	if r := s.planSnapshots; r != nil && projectID != "" {
		r.touch([]string{projectID}, nil)
	}
}

// planSnapshotRefresher keeps one instance's part of the snapshots fresh: it
// hears changes, writes them as marks, and computes the due snapshots it
// claims.
type planSnapshotRefresher struct {
	s     *Server
	store platstore.ReviewPlanSnapshotStore
	cfg   planSnapshotConfig

	mu         sync.Mutex
	projects   map[string]struct{}
	workspaces map[string]struct{}

	flight singleflight.Group
	// computes counts the snapshots this instance counted and stored.
	computes atomic.Int64
	// onCompute, when set, runs at the start of each background compute.
	onCompute atomic.Pointer[func(platstore.ReviewPlanSnapshotClaim)]

	sub    *platev.Subscription
	cancel context.CancelFunc
	done   chan struct{}
}

// startPlanSnapshots starts the instance's snapshot refresh, when the
// content store keeps snapshots and review decks.
func (s *Server) startPlanSnapshots(cfg planSnapshotConfig) *planSnapshotRefresher {
	if s.ContentStore == nil || s.AuthStore == nil || s.planSnapshots != nil {
		return s.planSnapshots
	}
	st, ok := platstore.ReviewPlanSnapshots(s.ContentStore)
	if !ok {
		return nil
	}
	if _, ok := platstore.ReviewDeckStores(s.ContentStore); !ok {
		return nil
	}
	// The refresh outlives every request; Close stops it.
	ctx, cancel := context.WithCancel(context.Background())
	r := &planSnapshotRefresher{
		s: s, store: st, cfg: cfg,
		projects: map[string]struct{}{}, workspaces: map[string]struct{}{},
		cancel: cancel, done: make(chan struct{}),
	}
	if s.EventBus != nil {
		r.sub = s.EventBus.SubscribeGroup(planSnapshotGroup, r.hear)
	}
	s.planSnapshots = r
	go r.run(ctx)
	return r
}

// Close stops the refresh and writes the changes it heard, within ctx.
func (r *planSnapshotRefresher) Close(ctx context.Context) {
	if r == nil {
		return
	}
	if r.sub != nil && r.s.EventBus != nil {
		r.s.EventBus.Unsubscribe(r.sub)
	}
	r.cancel()
	<-r.done
	r.flush(ctx)
}

// planSnapshotIgnored lists the event families that change nothing a
// snapshot counts.
var planSnapshotIgnored = []string{
	"auth.", "authz.", "agent.", "token.", "session.", "invite.", "member.", "role.", "platform_config.",
}

// hear marks the snapshots an event may change. An event about a project
// marks the project's; one about a workspace marks every project's in it,
// since the workspace's concepts feed each plan. It always acknowledges: the
// marks are written on the next poll.
func (r *planSnapshotRefresher) hear(ev platev.Event) error {
	for _, p := range planSnapshotIgnored {
		if strings.HasPrefix(string(ev.Type), p) {
			return nil
		}
	}
	if ev.ProjectID != "" {
		r.touch([]string{ev.ProjectID}, nil)
		return nil
	}
	r.touch([]string{ev.Data["project_id"]}, []string{ev.WorkspaceID, ev.Data["workspace_slug"]})
	return nil
}

// touch queues marks for the projects and workspaces named.
func (r *planSnapshotRefresher) touch(projects, workspaces []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range projects {
		if p != "" {
			r.projects[p] = struct{}{}
		}
	}
	for _, w := range workspaces {
		if w != "" {
			r.workspaces[w] = struct{}{}
		}
	}
}

// flush writes the queued marks in one statement. Marks that fail to write
// stay queued for the next poll.
func (r *planSnapshotRefresher) flush(ctx context.Context) {
	r.mu.Lock()
	projects, workspaces := r.projects, r.workspaces
	if len(projects) == 0 && len(workspaces) == 0 {
		r.mu.Unlock()
		return
	}
	r.projects, r.workspaces = map[string]struct{}{}, map[string]struct{}{}
	r.mu.Unlock()
	err := r.store.MarkReviewPlanSnapshotsChanged(ctx, slices.Collect(maps.Keys(projects)), slices.Collect(maps.Keys(workspaces)))
	if err == nil {
		return
	}
	slog.WarnContext(ctx, "review plan snapshot: could not mark changes", "error", err)
	r.mu.Lock()
	maps.Copy(r.projects, projects)
	maps.Copy(r.workspaces, workspaces)
	r.mu.Unlock()
}

// run polls until Close: it writes the marks heard, then computes the due
// snapshots one at a time.
func (r *planSnapshotRefresher) run(ctx context.Context) {
	defer close(r.done)
	t := time.NewTicker(r.cfg.poll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		r.flush(ctx)
		for ctx.Err() == nil {
			claims, err := r.store.ClaimDueReviewPlanSnapshots(ctx, r.cfg.quiet, r.cfg.maxWait, r.cfg.lease, 1)
			if err != nil {
				if ctx.Err() == nil {
					slog.WarnContext(ctx, "review plan snapshot: could not claim due snapshots", "error", err)
				}
				break
			}
			if len(claims) == 0 {
				break
			}
			for _, c := range claims {
				r.refresh(ctx, c)
			}
		}
	}
}

// refresh computes and stores one claimed snapshot. A compute that fails
// releases the claim, and the snapshot is tried again after the retry delay.
func (r *planSnapshotRefresher) refresh(ctx context.Context, c platstore.ReviewPlanSnapshotClaim) {
	if hook := r.onCompute.Load(); hook != nil {
		(*hook)(c)
	}
	snap, err := r.computeClaimed(ctx, c)
	if err == nil {
		err = r.complete(ctx, c, snap)
	}
	if err != nil {
		slog.WarnContext(ctx, "review plan snapshot: compute failed", "project", c.ProjectID, "stream", c.Stream, "error", err)
		if rerr := r.store.ReleaseReviewPlanSnapshot(context.WithoutCancel(ctx), c, r.cfg.retry); rerr != nil {
			slog.WarnContext(ctx, "review plan snapshot: could not release a claim", "project", c.ProjectID, "error", rerr)
		}
	}
}

// computeClaimed reads the project a claim names and counts its plan.
func (r *planSnapshotRefresher) computeClaimed(ctx context.Context, c platstore.ReviewPlanSnapshotClaim) (planSnapshot, error) {
	proj, err := r.s.ContentStore.GetProject(ctx, c.ProjectID)
	if err != nil || proj == nil {
		return planSnapshot{}, errNoProject
	}
	ws, err := r.s.AuthStore.GetWorkspace(ctx, proj.WorkspaceID)
	if err != nil || ws == nil {
		return planSnapshot{}, errNoProject
	}
	colls, err := r.s.ContentStore.ListCollections(ctx, proj.ID, c.Stream)
	if err != nil {
		return planSnapshot{}, err
	}
	scope, pi := deckScopeFrom(colls), involvementFor(proj, colls)
	fp := planFingerprint(deckPolicyKey(r.s.deckPolicy()), proj, scope, pi)
	return r.s.computePlanSnapshot(ctx, ws, proj, c.Stream, scope, pi, fp)
}

// complete stores a claim's counts.
func (r *planSnapshotRefresher) complete(ctx context.Context, c platstore.ReviewPlanSnapshotClaim, snap planSnapshot) error {
	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	r.computes.Add(1)
	return r.store.CompleteReviewPlanSnapshot(context.WithoutCancel(ctx), c, data)
}

// computeNow counts a snapshot on a request and stores it when it can claim
// it. While another compute holds the claim, the counts answer the request
// and that compute stores its own.
func (r *planSnapshotRefresher) computeNow(ctx context.Context, ws *platauth.Workspace, proj *platstore.Project, stream string, scope deckScope, pi projectInvolvement, fp string) (planSnapshot, error) {
	claim, claimed, err := r.store.ClaimReviewPlanSnapshot(ctx, proj.ID, stream, ws.ID, ws.Slug, r.cfg.lease)
	if err != nil {
		slog.WarnContext(ctx, "review plan snapshot: could not claim on a read", "project", proj.ID, "error", err)
	}
	snap, err := r.s.computePlanSnapshot(ctx, ws, proj, stream, scope, pi, fp)
	if err != nil {
		if claimed {
			_ = r.store.ReleaseReviewPlanSnapshot(context.WithoutCancel(ctx), claim, 0)
		}
		return planSnapshot{}, err
	}
	if claimed {
		if err := r.complete(ctx, claim, snap); err != nil {
			slog.WarnContext(ctx, "review plan snapshot: could not store", "project", proj.ID, "error", err)
		}
	}
	return snap, nil
}

// read answers the stored snapshot when it was taken under fingerprint. A
// snapshot no change has reached for maxAge is marked changed, so a change
// the events missed is counted within a few seconds of the next read.
func (r *planSnapshotRefresher) read(ctx context.Context, projectID, stream, fingerprint string) (planSnapshot, bool) {
	row, err := r.store.GetReviewPlanSnapshot(ctx, projectID, stream)
	if err != nil {
		slog.WarnContext(ctx, "review plan snapshot: could not read", "project", projectID, "error", err)
		return planSnapshot{}, false
	}
	if row == nil || len(row.Data) == 0 {
		return planSnapshot{}, false
	}
	var snap planSnapshot
	if json.Unmarshal(row.Data, &snap) != nil || snap.Fingerprint != fingerprint {
		return planSnapshot{}, false
	}
	if snap.Untranslated == nil {
		snap.Untranslated = map[string]int{}
	}
	if snap.Languages == nil {
		snap.Languages = map[string]planSnapshotLanguage{}
	}
	snap.asOf = row.ComputedAt.UTC()
	if !row.Changed && time.Since(row.ComputedAt) > r.cfg.maxAge {
		r.touch([]string{projectID}, nil)
	}
	return snap, true
}
