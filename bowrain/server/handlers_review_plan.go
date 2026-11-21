package server

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/host/reviewdeck"
	"github.com/neokapi/neokapi/terms"
)

// The review plan: what a project asks of its reviewers, in the order that
// gives each decision the most reach (docs/internals/review-by-leverage.md).
//
//  1. Coordinates: the products, channels and languages, and the voice each
//     point binds. Each decision governs everything under it.
//  2. Context: the terms and voice rules, source first, then each
//     term's translation in every target language.
//  3. Exceptions: what fails a check against the context.
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
	// propInvolvementSetBy and propInvolvementSetAt record the person who
	// last set the involvement, and when. The settle pass the server runs
	// after a run is credited to that person.
	propInvolvementSetBy = "review_involvement_set_by"
	propInvolvementSetAt = "review_involvement_set_at"
	// propCoordinatesConfirmedBy and propCoordinatesConfirmedAt record the
	// person who confirmed the coordinates, and when.
	propCoordinatesConfirmedBy = "review_coordinates_confirmed_by"
	propCoordinatesConfirmedAt = "review_coordinates_confirmed_at"
	// propCoordinatesSnapshot holds the map the last confirmation recorded:
	// the points, their voices and the target languages, as JSON. The plan
	// lists what changed since, for the whole team.
	propCoordinatesSnapshot = "review_coordinates_snapshot"
	// propFirstPassBy and propFirstPassAt record the person who settled the
	// project's first pass on the checks alone, and when. Later passes wait
	// for the context.
	propFirstPassBy = "review_first_pass_by"
	propFirstPassAt = "review_first_pass_at"
	// propTermScanJob is the last term scan started for the project, by a
	// push or from the review plan.
	propTermScanJob = "review_term_scan_job"
)

var errReviewPlanProperty = errors.New("the review plan manages this project property")

func isReviewPlanProperty(key string) bool {
	switch key {
	case propReviewInvolvement, propInvolvementOverrides, propInvolvementSetBy, propInvolvementSetAt,
		propCoordinatesConfirmedBy, propCoordinatesConfirmedAt, propCoordinatesSnapshot,
		propFirstPassBy, propFirstPassAt, propSettleLastAt, propSettleLastApproved, propSettleLastBy, propSettleLastAutomatic,
		propTermScanJob, propTermScanBlocks, propTermScanCollections, propTermProposalJob:
		return true
	}
	return false
}

// reasonReviewPolicy is the reason a settle pass records on each decision
// in the ledger.
const reasonReviewPolicy = "review policy"

// interfaceSampleEvery is the share of each collection's pages a reviewer
// reads under context: one page in this many, plus every page with a
// failing finding.
const interfaceSampleEvery = 10

// How a settle pass may run, as the plan reports it.
const (
	// settleFirstPass settles on the checks alone: the context is still
	// open and the project has had no first pass.
	settleFirstPass = "first_pass"
	// settlePolicy settles under the full policy: the context is decided.
	settlePolicy = "policy"
	// settleHeld waits for the context: the first pass is taken.
	settleHeld = "held"
	// settleNone leaves nothing to the checks: involvement is every.
	settleNone = "none"
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
	planContext     = "context"
	planExceptions  = "exceptions"
	planInterfaces  = "interfaces"
	planPolicy      = "policy"
)

// reviewPagesCountLimit bounds the pages the plan counts per language.
const reviewPagesCountLimit = 5000

// planRuleLimit bounds the rules the plan names for the source and for each
// language: the ones with the most findings.
const planRuleLimit = 5

type reviewPlanResponse struct {
	Stream string `json:"stream"`
	// Involvement is the project's: context (the default), interfaces or
	// every. Overrides set another at a point of the project's context
	// space, and each point's Involvement is the one that applies there.
	Involvement string                   `json:"involvement"`
	Overrides   []involvementOverrideDTO `json:"overrides"`
	// Next is the first level the caller can act on, or "" when none can be.
	// A level that only waits is skipped and listed in Waiting.
	Next string `json:"next"`
	// Waiting lists the levels skipped for Next: work lies ahead there and
	// nothing waits for a decision yet. Each carries the reason.
	Waiting     []planWaitDTO      `json:"waiting"`
	Coordinates planCoordinatesDTO `json:"coordinates"`
	Context     planContextDTO     `json:"context"`
	Exceptions  planExceptionsDTO  `json:"exceptions"`
	Languages   []planLanguageDTO  `json:"languages"`
	Policy      planPolicyDTO      `json:"policy"`
	// Steps is each level in brief with its state, in order.
	Steps []planStepDTO `json:"steps"`
	// Held is set while the language decks hold translations back until the
	// context is decided; each language's Held counts them.
	Held string `json:"held,omitempty"`
	// AsOf is when the counts were taken. The server recomputes them a few
	// seconds after a change, so they may trail the latest decisions.
	AsOf string `json:"as_of,omitempty"`
}

// planPolicyDTO is what the involvement leaves to the checks, and whether a
// settle pass may run now.
type planPolicyDTO struct {
	// Settle is first_pass, policy, held or none.
	Settle string `json:"settle"`
	// ContextOpen counts the project's open context decisions:
	// context cards waiting for anyone, and target terms still missing.
	ContextOpen int    `json:"context_open"`
	FirstPassBy string `json:"first_pass_by,omitempty"`
	FirstPassAt string `json:"first_pass_at,omitempty"`
	// Auto is who the settle pass the server runs after each run is
	// credited to, or why it does not run. Only the plan route sets it.
	Auto *planAutoSettleDTO `json:"auto,omitempty"`
	// Last is the project's last settle pass, by hand or after a run.
	Last *planSettleResultDTO `json:"last,omitempty"`
}

// planPoint is one place in the project's context space: a collection's
// coordinates and the voice bound there.
type planPoint struct {
	Collection  string            `json:"collection"`
	Coordinates map[string]string `json:"coordinates"`
	Voice       string            `json:"voice,omitempty"`
	// VoiceID is the voice profile's id, which a change of voice is
	// compared by.
	VoiceID    string `json:"voice_id,omitempty"`
	SourceOnly bool   `json:"source_only,omitempty"`
	Priority   string `json:"priority,omitempty"`
	// Involvement is the involvement at the point: the project's, or the
	// override that applies there.
	Involvement string `json:"involvement,omitempty"`
	// id is the collection's id, which the coverage counts are keyed by.
	id string
}

type planCoordinatesDTO struct {
	SourceLanguage  string      `json:"source_language"`
	TargetLanguages []string    `json:"target_languages"`
	Points          []planPoint `json:"points"`
	// Confirmed is set once a person has confirmed the coordinates.
	Confirmed   bool   `json:"confirmed"`
	ConfirmedBy string `json:"confirmed_by,omitempty"`
	// ConfirmedByName is the display name of the person who confirmed.
	ConfirmedByName string `json:"confirmed_by_name,omitempty"`
	ConfirmedAt     string `json:"confirmed_at,omitempty"`
	// Changes lists what differs from the map the last confirmation
	// recorded, whoever confirmed it, and Changed counts them. While Changed
	// is above zero the step waits for the team again; one person confirming
	// settles it for everyone.
	Changes []planCoordinateChange `json:"changes"`
	Changed int                    `json:"changed"`
}

type planContextDTO struct {
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
	// Scan is the project's last scan for candidate terms, started by a push
	// or from the plan, if any.
	Scan *planScanDTO `json:"scan,omitempty"`
	// ScanWaiting is why no scan runs while the context has not
	// started: waiting_content, waiting_push, waiting_quota, waiting_credits
	// or no_ai.
	ScanWaiting string `json:"scan_waiting,omitempty"`
	// QuotaResetsAt is when the workspace's monthly AI limit resets, set
	// with waiting_quota.
	QuotaResetsAt string `json:"quota_resets_at,omitempty"`
	// Proposable counts the concept and language pairs waiting for a
	// proposed term: a concept of the project with a source term, and no
	// proposal or usable term in a target language.
	Proposable int `json:"proposable"`
	// Proposals is the last job the server started to propose the project's
	// target terms, if any.
	Proposals *planScanDTO `json:"proposals,omitempty"`
	// ProposalsWaiting is why no proposal job runs while pairs wait:
	// waiting_quota, waiting_credits or no_ai.
	ProposalsWaiting string `json:"proposals_waiting,omitempty"`
}

// planScanDTO is a project scan's state.
type planScanDTO struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Progress  int    `json:"progress"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type planExceptionsDTO struct {
	// SourceFindings is the findings the last project-wide check recorded in
	// the source; nil before any check.
	SourceFindings *int   `json:"source_findings,omitempty"`
	CheckedAt      string `json:"checked_at,omitempty"`
	// ByRule names the rules behind the most findings: in the source, as the
	// last project-wide check counted them, and in each language the caller
	// may review, over the translations the deck holds back.
	ByRule planRulesDTO `json:"by_rule"`
}

// planRulesDTO is the exceptions grouped by rule.
type planRulesDTO struct {
	Source  []planRuleDTO            `json:"source"`
	Targets map[string][]planRuleDTO `json:"targets"`
}

// planRuleDTO is one rule and the findings it raised. Key is what the review
// deck's rule filter takes (?rule=).
type planRuleDTO struct {
	Key     string `json:"key"`
	Rule    string `json:"rule"`
	Subject string `json:"subject,omitempty"`
	Message string `json:"message,omitempty"`
	// ConceptID is the terms-store concept a word rule comes from.
	ConceptID string `json:"concept_id,omitempty"`
	Fails     bool   `json:"fails,omitempty"`
	Count     int    `json:"count"`
}

func planRulesOf(tallies []platstore.RuleTally) []planRuleDTO {
	out := make([]planRuleDTO, 0, len(tallies))
	for _, t := range tallies {
		out = append(out, planRuleDTO{
			Key: t.Key, Rule: t.Rule, Subject: t.Subject, Message: t.Message,
			ConceptID: t.ConceptID, Fails: t.Fails, Count: t.Count,
		})
	}
	return out
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
	// Pages is the pages the involvement asks a person to read: the sample
	// in a collection under context, every page with a translation waiting
	// elsewhere.
	Pages     int  `json:"pages"`
	PagesMore bool `json:"pages_more,omitempty"`
	// PagesTotal is every page with a translation waiting, and
	// PagesWithFindings the ones holding a failing finding.
	PagesTotal        int `json:"pages_total"`
	PagesWithFindings int `json:"pages_with_findings"`
	// Settleable is the translations the involvement in force leaves to the
	// checks: the ones that clear every bar (the deck's fast group),
	// outside the collections under every.
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
	locales, _, err := s.followedLocales(ctx, userID, proj.ID, reviewLocalesOf(proj, perms, langs))
	if err != nil {
		return serverErr(c, err)
	}
	out, err := s.computeReviewPlan(ctx, ws, proj, stream, planCaller{
		userID: userID, perms: perms, reach: reach, locales: locales,
	})
	if err != nil {
		return serverErr(c, err)
	}
	auto := s.autoSettleActorOf(ctx, ws, proj)
	out.Policy.Auto = &auto.planAutoSettleDTO
	if last := out.Policy.Last; last != nil && last.By != "" {
		if u, err := s.AuthStore.GetUser(ctx, last.By); err == nil && u != nil {
			last.ByName = u.Name
		}
	}
	return c.JSON(http.StatusOK, out)
}

// planCaller is who a plan is computed for: what they may do, the
// coordinates their grants reach, and the languages the plan counts (the
// ones they may review and follow).
type planCaller struct {
	userID  string
	perms   platauth.Permission
	reach   platauth.CoordinateReach
	locales []string
	// context is the caller's context cards when already counted, as the
	// review inbox counts them before it builds a plan; nil counts them.
	context *callerContext
}

// callerContext is the context cards waiting for one caller, as
// contextInbox counts them.
type callerContext struct {
	entry contextInboxEntry
	ok    bool
}

// computeReviewPlan builds a project's plan for one caller. The review plan
// route and the review inbox both answer with it.
func (s *Server) computeReviewPlan(ctx context.Context, ws *platauth.Workspace, proj *platstore.Project, stream string, caller planCaller) (reviewPlanResponse, error) {
	b, err := s.planBasisOf(ctx, ws, proj, stream)
	if err != nil {
		return reviewPlanResponse{}, err
	}
	return s.reviewPlanFrom(ctx, ws, proj, stream, caller, b)
}

// reviewPlanFrom builds a plan from its counts and what depends on the
// caller: the context they may keep and the languages they follow.
func (s *Server) reviewPlanFrom(ctx context.Context, ws *platauth.Workspace, proj *platstore.Project, stream string, caller planCaller, b planBasis) (reviewPlanResponse, error) {
	pi := b.pi
	out := reviewPlanResponse{
		Stream:      stream,
		Involvement: pi.def,
		Overrides:   pi.overridesDTO(),
		Languages:   []planLanguageDTO{},
		AsOf:        b.snap.asOf.UTC().Format(time.RFC3339),
	}
	var err error
	if out.Coordinates, err = s.planCoordinatesOf(ctx, proj, stream, b.colls); err != nil {
		return out, err
	}
	for i := range out.Coordinates.Points {
		out.Coordinates.Points[i].Involvement = pi.of(out.Coordinates.Points[i].id)
	}
	out.Context = planContextDTO{
		ByKind:       map[string]int{},
		Concepts:     b.snap.Concepts,
		Untranslated: maps.Clone(b.snap.Untranslated),
		Proposable:   b.snap.Proposable,
	}
	known := caller.context
	if known == nil {
		entry, ok, err := s.contextInbox(ctx, ws, proj, caller.userID, caller.perms, caller.reach)
		if err != nil {
			return out, err
		}
		known = &callerContext{entry: entry, ok: ok}
	}
	if entry, ok := known.entry, known.ok; ok {
		out.Context.Waiting, out.Context.ByKind = entry.Waiting, entry.ByKind
	}
	out.Exceptions = planExceptionsDTO{
		SourceFindings: b.snap.Exceptions.SourceFindings,
		CheckedAt:      b.snap.Exceptions.CheckedAt,
		ByRule:         planRulesDTO{Source: b.snap.Exceptions.ByRule.Source, Targets: map[string][]planRuleDTO{}},
	}
	if out.Exceptions.ByRule.Source == nil {
		out.Exceptions.ByRule.Source = []planRuleDTO{}
	}
	out.Context.Scan = s.planScan(ctx, proj, ws.ID)
	s.planScanWaiting(ctx, ws.ID, ws.Slug, proj, &out.Context)
	s.planProposals(ctx, ws.ID, ws.Slug, proj, &out.Context)
	out.Policy = planPolicyFrom(proj, pi, b.snap.ContextOpen, out.Context.Untranslated)
	held := translationsHeldBy(out.Policy)
	if held {
		out.Held = heldContext
	}
	out.Languages = planLanguagesFrom(b.snap, caller.locales, held, out.Exceptions.ByRule.Targets)
	out.Next, out.Waiting = nextPlanLevel(out)
	out.Steps = planSteps(out)
	return out, nil
}

// planScan reads the state of the project's last term scan.
func (s *Server) planScan(ctx context.Context, proj *platstore.Project, wsID string) *planScanDTO {
	id := proj.Properties[propTermScanJob]
	if id == "" || s.ContextScanStore == nil {
		return nil
	}
	job, err := s.ContextScanStore.GetContextScanJob(ctx, id)
	if err != nil || job == nil || job.WorkspaceID != wsID {
		return nil
	}
	return &planScanDTO{
		ID: job.ID, Status: string(job.Status), Progress: job.Progress,
		CreatedAt: job.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: job.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// planPolicy answers whether a settle pass may run. The context is
// counted for the whole project, whoever asks: a pass settles everyone's
// content, so it waits on every open decision, not only the caller's. A pass
// has nothing to settle when every point of the project is under every.
func (s *Server) planPolicy(ctx context.Context, ws *platauth.Workspace, proj *platstore.Project, pi projectInvolvement) (planPolicyDTO, error) {
	counts, err := s.planConceptCounts(ctx, ws, proj)
	if err != nil {
		return planPolicyDTO{}, err
	}
	cards, err := s.projectContextOpen(ctx, ws, proj)
	if err != nil {
		return planPolicyDTO{}, err
	}
	return planPolicyFrom(proj, pi, cards, counts.Untranslated), nil
}

// planPolicyFrom is planPolicy over counts already taken: the context cards
// waiting for any reviewer, and the concepts each language lacks.
func planPolicyFrom(proj *platstore.Project, pi projectInvolvement, cards int, untranslated map[string]int) planPolicyDTO {
	out := planPolicyDTO{
		FirstPassBy: proj.Properties[propFirstPassBy],
		FirstPassAt: proj.Properties[propFirstPassAt],
	}
	open := cards
	for _, n := range untranslated {
		open += n
	}
	out.ContextOpen = open
	out.Last = lastSettleOf(proj)
	switch {
	case pi.allEvery():
		out.Settle = settleNone
	case open == 0:
		out.Settle = settlePolicy
	case out.FirstPassBy == "":
		out.Settle = settleFirstPass
	default:
		out.Settle = settleHeld
	}
	return out
}

// projectContextOpen counts the project's context cards waiting for review,
// for any reviewer.
func (s *Server) projectContextOpen(ctx context.Context, ws *platauth.Workspace, proj *platstore.Project) (int, error) {
	src, err := s.contextDeckSource(ctx, ws.ID, ws.Slug, proj, "")
	if err != nil || src.key == "" {
		return 0, err
	}
	cards, err := s.pendingContext(ctx, src, deckReviewer{perms: platauth.PermAll, solo: true})
	if err != nil {
		return 0, err
	}
	return len(cards), nil
}

// involvementOf is the project's review involvement, context when unset.
func involvementOf(p *platstore.Project) string {
	switch v := p.Properties[propReviewInvolvement]; v {
	case involvementInterfaces, involvementEvery:
		return v
	}
	return involvementContext
}

// nextPlanLevel is the first level the caller can act on, and the levels
// before it that only wait. A context that has not started is the next
// step once the coordinates are confirmed, unless its term scan runs or
// cannot start: then nothing waits for a decision there, and the level is
// listed with the reason instead. A context whose only open decisions are
// target terms in languages the caller does not follow is listed with
// other_languages and those languages: other reviewers decide them.
func nextPlanLevel(p reviewPlanResponse) (string, []planWaitDTO) {
	waiting := []planWaitDTO{}
	if !p.Coordinates.Confirmed || p.Coordinates.Changed > 0 {
		return planCoordinates, waiting
	}
	locales := make([]string, 0, len(p.Languages))
	for _, l := range p.Languages {
		locales = append(locales, l.Locale)
	}
	untranslated := false
	var others []string
	for loc, n := range p.Context.Untranslated {
		switch {
		case n <= 0:
		case slices.Contains(locales, loc):
			untranslated = true
		default:
			others = append(others, loc)
		}
	}
	slices.Sort(others)
	if reason, resets := contextWaitReason(p.Context); reason != "" {
		waiting = append(waiting, planWaitDTO{Level: planContext, Reason: reason, QuotaResetsAt: resets})
	} else if p.Context.Waiting > 0 || untranslated || contextNotStarted(p.Context) {
		return planContext, waiting
	} else if len(others) > 0 {
		waiting = append(waiting, planWaitDTO{Level: planContext, Reason: planWaitOtherLanguages, Locales: others})
	}
	if p.Exceptions.SourceFindings != nil && *p.Exceptions.SourceFindings > 0 {
		return planExceptions, waiting
	}
	for _, l := range p.Languages {
		if l.Failing > 0 {
			return planExceptions, waiting
		}
	}
	for _, l := range p.Languages {
		if l.Pages > 0 {
			return planInterfaces, waiting
		}
	}
	if p.Policy.Settle == settleFirstPass || p.Policy.Settle == settlePolicy {
		for _, l := range p.Languages {
			if l.Settleable > 0 {
				return planPolicy, waiting
			}
		}
	}
	return "", waiting
}

// contextWaitReason names why a context level that has not started has
// nothing to decide yet: the term scan runs (queued or processing), or it
// cannot start (waiting_content, waiting_quota, waiting_credits or no_ai),
// with when the AI limit resets for waiting_quota. It is "" for a level a
// person can act on, including one whose scan starts with the next push or
// failed, which a person may start again.
func contextWaitReason(k planContextDTO) (reason, quotaResetsAt string) {
	if !contextNotStarted(k) {
		return "", ""
	}
	if k.Scan != nil && scanRunning(k.Scan.Status) {
		return k.Scan.Status, ""
	}
	switch k.ScanWaiting {
	case scanWaitingQuota:
		return k.ScanWaiting, k.QuotaResetsAt
	case scanWaitingContent, scanWaitingCredits, scanNoAI:
		return k.ScanWaiting, ""
	}
	return "", ""
}

// planCoordinates reads the project's points from its collections.
func (s *Server) planCoordinates(ctx context.Context, proj *platstore.Project, stream string) (planCoordinatesDTO, error) {
	colls, err := s.ContentStore.ListCollections(ctx, proj.ID, stream)
	if err != nil {
		return planCoordinatesDTO{}, err
	}
	return s.planCoordinatesOf(ctx, proj, stream, colls)
}

// planCoordinatesOf reads the project's points from the stream's
// collections.
func (s *Server) planCoordinatesOf(ctx context.Context, proj *platstore.Project, stream string, colls []*platstore.Collection) (planCoordinatesDTO, error) {
	out := planCoordinatesDTO{
		SourceLanguage:  string(proj.DefaultSourceLanguage),
		TargetLanguages: []string{},
		Points:          []planPoint{},
		ConfirmedBy:     proj.Properties[propCoordinatesConfirmedBy],
		ConfirmedAt:     proj.Properties[propCoordinatesConfirmedAt],
		Changes:         []planCoordinateChange{},
	}
	out.Confirmed = out.ConfirmedBy != ""
	if out.Confirmed && s.AuthStore != nil {
		if u, err := s.AuthStore.GetUser(ctx, out.ConfirmedBy); err == nil && u != nil {
			out.ConfirmedByName = u.Name
		}
	}
	for _, l := range proj.TargetLanguages {
		out.TargetLanguages = append(out.TargetLanguages, string(l))
	}
	voiceNames := map[string]string{}
	for _, c := range colls {
		pt := planPoint{
			id:          c.ID,
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
			pt.Voice, pt.VoiceID = name, id
		}
		out.Points = append(out.Points, pt)
	}
	slices.SortFunc(out.Points, func(a, b planPoint) int { return strings.Compare(a.Collection, b.Collection) })
	// The confirmed map is the main stream's; a branch is not compared
	// with it.
	if out.Confirmed && stream == reviewInboxStream {
		if snap, ok := coordinatesSnapshotOf(proj); ok {
			out.Changes = coordinateChanges(snap, out)
		}
	}
	out.Changed = len(out.Changes)
	return out, nil
}

// planConceptCounts counts, of the concepts that apply in the project, the
// ones with a source term, the ones each target language lacks, and the
// pairs waiting for a proposal.
func (s *Server) planConceptCounts(ctx context.Context, ws *platauth.Workspace, proj *platstore.Project) (planContextDTO, error) {
	out := planContextDTO{ByKind: map[string]int{}, Untranslated: map[string]int{}}
	if s.wsStores == nil {
		return out, nil
	}
	tb, err := s.wsStores.getProjectTerms(ctx, cmp.Or(ws.Slug, ws.ID), proj.ID)
	if errors.Is(err, errNoPgDB) {
		return out, nil
	}
	if err != nil || tb == nil {
		return out, err
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
	out.Proposable = termProposalsWaiting(concepts, proj)
	return out, nil
}

// planExceptions sums the source findings the last project-wide check
// recorded, and their rules across items.
func (s *Server) planExceptions(ctx context.Context, proj *platstore.Project, stream string) (planExceptionsDTO, error) {
	out := planExceptionsDTO{ByRule: planRulesDTO{Source: []planRuleDTO{}, Targets: map[string][]planRuleDTO{}}}
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
	var rules platstore.RuleTallies
	for _, t := range tallies {
		findings += t.Findings
		if t.CheckedAt.After(at) {
			at = t.CheckedAt
		}
		for _, r := range t.Rules {
			rules.Add(r)
		}
	}
	out.SourceFindings = &findings
	out.ByRule.Source = planRulesOf(rules.Top(planRuleLimit))
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

// planLanguagesFrom builds each language the caller follows from the
// counts, and files the rules behind each deck's desk group in rules by
// language.
//
// While held, the translations the deck holds back move out of Waiting into
// Held; Settleable still counts the fast group, which a first pass settles.
// Both leave out the collections under every, which hold nothing back and
// leave nothing to the checks.
func planLanguagesFrom(snap planSnapshot, locales []string, held bool, rules map[string][]planRuleDTO) []planLanguageDTO {
	out := []planLanguageDTO{}
	desk := deckRank(reviewdeck.DeckDesk)
	for _, loc := range locales {
		l := snap.Languages[loc]
		n := l.split()
		if len(l.Rules) > 0 {
			rules[loc] = planRulesOf(l.Rules)
		}
		out = append(out, planLanguageDTO{
			Locale:            loc,
			deckWaiting:       n.waiting(held),
			Failing:           n.rest.ByRank[desk] + n.open.ByRank[desk],
			Settleable:        deckWaitingOf(n.rest).Fast,
			Pages:             l.Pages,
			PagesMore:         l.PagesMore,
			PagesTotal:        l.PagesTotal,
			PagesWithFindings: l.PagesWithFindings,
		})
	}
	return out
}

// interfaceSample is the pages a reviewer reads under context: every page
// with a failing finding, and the first of every interfaceSampleEvery pages of
// each collection, in deck order. Each collection with a page waiting gives at
// least one.
func interfaceSample(pages []platstore.ReviewDeckPage) []platstore.ReviewDeckPage {
	return interfaceSampleWhere(pages, func(string) bool { return true })
}

// interfaceSampleWhere samples the pages of the collections sampled names as
// interfaceSample does, and keeps every page of the others, in deck order.
func interfaceSampleWhere(pages []platstore.ReviewDeckPage, sampled func(collectionID string) bool) []platstore.ReviewDeckPage {
	seen := map[string]int{}
	var out []platstore.ReviewDeckPage
	for _, p := range pages {
		if !sampled(p.CollectionID) {
			out = append(out, p)
			continue
		}
		n := seen[p.CollectionID]
		seen[p.CollectionID] = n + 1
		if p.Findings > 0 || n%interfaceSampleEvery == 0 {
			out = append(out, p)
		}
	}
	return out
}

// reviewPlanSettleRequest scopes a settle pass.
type reviewPlanSettleRequest struct {
	Stream  string   `json:"stream,omitempty"`
	Locales []string `json:"locales,omitempty"`
}

// reviewPlanSettleResponse reports a settle pass.
type reviewPlanSettleResponse struct {
	ApprovePassingResponse
	// FirstPass is set when this pass was the project's first, on the checks
	// alone.
	FirstPass bool `json:"first_pass"`
}

// HandleSettleReviewPlan answers POST /:ws/:id/review/plan/settle: the
// plan's last step, run by hand. The pass is settleReviewPlan's, credited to
// the caller. The server runs the same pass after each run
// (review_plan_autosettle.go).
//
// A project settles on the checks alone once, for its first pass; after
// that a pass waits until the context is decided (409). Under the
// involvement every nothing is left to the checks (409). A pass already
// running for the project answers 409 too.
func (s *Server) HandleSettleReviewPlan(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermReview); err != nil {
		return err
	}
	if s.ContentStore == nil || s.AuthStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "store not configured")
	}
	var req reviewPlanSettleRequest
	if err := c.Bind(&req); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	ctx := c.Request().Context()
	proj, err := s.ContentStore.GetProject(ctx, projectParam(c))
	if err != nil || proj == nil {
		return apiErr(c, http.StatusNotFound, errNoProject.Error())
	}
	stream := strings.TrimSpace(req.Stream)
	if stream == "" {
		stream = reviewInboxStream
	}
	wsID, _ := c.Get("workspace_id").(string)
	ws, err := s.AuthStore.GetWorkspace(ctx, wsID)
	if err != nil || ws == nil {
		return apiErr(c, http.StatusNotFound, "workspace not found")
	}
	out, err := s.settleReviewPlan(c, ws, proj.ID, stream, req.Locales, false)
	switch {
	case errors.Is(err, errSettleNone), errors.Is(err, errSettleHeld), errors.Is(err, errSettleBusy):
		return apiErr(c, http.StatusConflict, err.Error())
	case err != nil:
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// reviewPlanScanResponse names the scan the plan started.
type reviewPlanScanResponse struct {
	Scan planScanDTO `json:"scan"`
}

// HandleStartReviewPlanScan answers POST /:ws/:id/review/plan/scan: it
// scans the project again, which files the project's candidate terms in the
// context deck, and records it so the plan shows its state. A push starts
// the scan on its own (review_plan_autoscan.go); this route is for a person
// who wants a fresh one. It needs the permission to manage voice profiles,
// as any context scan does.
func (s *Server) HandleStartReviewPlanScan(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageVoice); err != nil {
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
	wsID, _ := c.Get("workspace_id").(string)
	if prev := s.planScan(ctx, proj, wsID); prev != nil && scanRunning(prev.Status) {
		return c.JSON(http.StatusOK, reviewPlanScanResponse{Scan: *prev})
	}
	if proj.WorkspaceID != wsID {
		return apiErr(c, http.StatusNotFound, errNoProject.Error())
	}
	job, status, err := s.startProjectTermScan(ctx, proj, c.Param("ws"), nil)
	if err != nil && job == nil {
		return apiErr(c, status, err.Error())
	}
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusAccepted, reviewPlanScanResponse{Scan: planScanDTO{
		ID: job.ID, Status: string(job.Status), Progress: job.Progress,
		CreatedAt: job.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: job.UpdatedAt.UTC().Format(time.RFC3339),
	}})
}

// scanRunning reports a scan that has not finished.
func scanRunning(status string) bool {
	return status == string(jobs.ContextScanStatusQueued) || status == string(jobs.ContextScanStatusProcessing)
}

// reviewPlanInvolvementRequest sets the project's involvement.
type reviewPlanInvolvementRequest struct {
	Involvement string `json:"involvement"`
}

// HandlePutReviewInvolvement answers PUT /:ws/:id/review/plan/involvement
// {"involvement": "context"|"interfaces"|"every"}. It needs the permission to
// edit the project's settings, and records the caller as the person the
// settle pass after each run is credited to.
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
	userID, _ := c.Get("user_id").(string)
	return s.setPlanProperties(c, map[string]string{
		propReviewInvolvement: req.Involvement,
		propInvolvementSetBy:  userID,
		propInvolvementSetAt:  time.Now().UTC().Format(time.RFC3339),
	})
}

// HandleConfirmReviewCoordinates answers POST
// /:ws/:id/review/plan/coordinates: the caller confirms the project's
// coordinates as the plan lists them, and the main stream's map is recorded
// as the baseline later changes are listed against. It needs the permission
// to edit the project's settings.
func (s *Server) HandleConfirmReviewCoordinates(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageProject); err != nil {
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
	coords, err := s.planCoordinates(ctx, proj, reviewInboxStream)
	if err != nil {
		return serverErr(c, err)
	}
	snap, err := encodeCoordinatesSnapshot(coords)
	if err != nil {
		return serverErr(c, err)
	}
	userID, _ := c.Get("user_id").(string)
	return s.setPlanProperties(c, map[string]string{
		propCoordinatesConfirmedBy: userID,
		propCoordinatesConfirmedAt: time.Now().UTC().Format(time.RFC3339),
		propCoordinatesSnapshot:    snap,
	})
}

// setPlanProperties writes plan properties on the project and answers 204.
func (s *Server) setPlanProperties(c echo.Context, props map[string]string) error {
	if s.ContentStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "store not configured")
	}
	if err := s.updatePlanProperties(c.Request().Context(), projectParam(c), props); err != nil {
		if errors.Is(err, errNoProject) {
			return apiErr(c, http.StatusNotFound, errNoProject.Error())
		}
		return serverErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// updatePlanProperties writes plan properties on the project.
func (s *Server) updatePlanProperties(ctx context.Context, projectID string, props map[string]string) error {
	_, _, err := s.ContentStore.EditProject(ctx, projectID, func(proj *platstore.Project) (bool, error) {
		maps.Copy(proj.Properties, props)
		return len(props) > 0, nil
	})
	if errors.Is(err, sql.ErrNoRows) {
		return errNoProject
	}
	if err != nil {
		return err
	}
	s.touchPlanSnapshot(projectID)
	return nil
}
