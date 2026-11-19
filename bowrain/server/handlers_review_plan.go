package server

import (
	"context"
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
	// propFirstPassBy and propFirstPassAt record the person who settled the
	// project's first pass on the checks alone, and when. Later passes wait
	// for the key language.
	propFirstPassBy = "review_first_pass_by"
	propFirstPassAt = "review_first_pass_at"
	// propTermScanJob is the last context scan started for the project from
	// the review plan.
	propTermScanJob = "review_term_scan_job"
)

// reasonReviewPolicy is the reason a settle pass records on each decision
// in the ledger.
const reasonReviewPolicy = "review policy"

// interfaceSampleEvery is the share of each collection's pages a reviewer
// reads under context: one page in this many, plus every page with a
// failing finding.
const interfaceSampleEvery = 10

// How a settle pass may run, as the plan reports it.
const (
	// settleFirstPass settles on the checks alone: the key language is still
	// open and the project has had no first pass.
	settleFirstPass = "first_pass"
	// settlePolicy settles under the full policy: the key language is decided.
	settlePolicy = "policy"
	// settleHeld waits for the key language: the first pass is taken.
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
	planKeyLanguage = "key_language"
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
	// every.
	Involvement string `json:"involvement"`
	// Next is the first level with something waiting for the caller, or ""
	// when nothing does.
	Next        string             `json:"next"`
	Coordinates planCoordinatesDTO `json:"coordinates"`
	KeyLanguage planKeyLanguageDTO `json:"key_language"`
	Exceptions  planExceptionsDTO  `json:"exceptions"`
	Languages   []planLanguageDTO  `json:"languages"`
	Policy      planPolicyDTO      `json:"policy"`
}

// planPolicyDTO is what the involvement leaves to the checks, and whether a
// settle pass may run now.
type planPolicyDTO struct {
	// Settle is first_pass, policy, held or none.
	Settle string `json:"settle"`
	// KeyLanguageOpen counts the project's open key-language decisions:
	// context cards waiting for anyone, and target terms still missing.
	KeyLanguageOpen int    `json:"key_language_open"`
	FirstPassBy     string `json:"first_pass_by,omitempty"`
	FirstPassAt     string `json:"first_pass_at,omitempty"`
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
	// Scan is the last project scan for candidate terms started from the
	// plan, if any.
	Scan *planScanDTO `json:"scan,omitempty"`
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
	// under context, every page with a translation waiting otherwise.
	Pages     int  `json:"pages"`
	PagesMore bool `json:"pages_more,omitempty"`
	// PagesTotal is every page with a translation waiting, and
	// PagesWithFindings the ones holding a failing finding.
	PagesTotal        int `json:"pages_total"`
	PagesWithFindings int `json:"pages_with_findings"`
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
	if out.Languages, err = s.planLanguages(ctx, proj, stream, reviewLocalesOf(proj, perms, langs), out.Involvement, out.Exceptions.ByRule.Targets); err != nil {
		return serverErr(c, err)
	}
	out.KeyLanguage.Scan = s.planScan(ctx, proj, wsID)
	if out.Policy, err = s.planPolicy(ctx, ws, proj, out.Involvement, out.KeyLanguage.Untranslated); err != nil {
		return serverErr(c, err)
	}
	out.Next = nextPlanLevel(out)
	return c.JSON(http.StatusOK, out)
}

// planScan reads the state of the last project scan the plan started.
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

// planPolicy answers whether a settle pass may run. The key language is
// counted for the whole project, whoever asks: a pass settles everyone's
// content, so it waits on every open decision, not only the caller's.
func (s *Server) planPolicy(ctx context.Context, ws *platauth.Workspace, proj *platstore.Project, involvement string, untranslated map[string]int) (planPolicyDTO, error) {
	out := planPolicyDTO{
		FirstPassBy: proj.Properties[propFirstPassBy],
		FirstPassAt: proj.Properties[propFirstPassAt],
	}
	open, err := s.projectContextOpen(ctx, ws, proj)
	if err != nil {
		return out, err
	}
	for _, n := range untranslated {
		open += n
	}
	out.KeyLanguageOpen = open
	switch {
	case involvement == involvementEvery:
		out.Settle = settleNone
	case open == 0:
		out.Settle = settlePolicy
	case out.FirstPassBy == "":
		out.Settle = settleFirstPass
	default:
		out.Settle = settleHeld
	}
	return out, nil
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
		if l.Pages > 0 {
			return planInterfaces
		}
	}
	if p.Policy.Settle == settleFirstPass || p.Policy.Settle == settlePolicy {
		for _, l := range p.Languages {
			if l.Settleable > 0 {
				return planPolicy
			}
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

// planLanguages counts, for each language, the deck's groups and pages, and
// files the rules behind the deck's desk group in rules by language.
func (s *Server) planLanguages(ctx context.Context, proj *platstore.Project, stream string, locales []string, involvement string, rules map[string][]planRuleDTO) ([]planLanguageDTO, error) {
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
		if lang.Failing > 0 {
			top, err := decks.CountReviewDeckRules(ctx, q, deckRank(reviewdeck.DeckDesk), planRuleLimit)
			if err != nil {
				return nil, err
			}
			if len(top) > 0 {
				rules[loc] = planRulesOf(top)
			}
		}
		if involvement != involvementEvery {
			lang.Settleable = lang.Fast
		}
		pages, err := decks.ListReviewDeckPages(ctx, q, reviewPagesCountLimit+1)
		if err != nil {
			return nil, err
		}
		if len(pages) > reviewPagesCountLimit {
			pages, lang.PagesMore = pages[:reviewPagesCountLimit], true
		}
		lang.PagesTotal = len(pages)
		for _, p := range pages {
			if p.Findings > 0 {
				lang.PagesWithFindings++
			}
		}
		lang.Pages = lang.PagesTotal
		if involvement == involvementContext {
			lang.Pages = len(interfaceSample(pages))
		}
		out = append(out, lang)
	}
	return out, nil
}

// interfaceSample is the pages a reviewer reads under context: every page
// with a failing finding, and the first of every interfaceSampleEvery pages of
// each collection, in deck order. Each collection with a page waiting gives at
// least one.
func interfaceSample(pages []platstore.ReviewDeckPage) []platstore.ReviewDeckPage {
	seen := map[string]int{}
	var out []platstore.ReviewDeckPage
	for _, p := range pages {
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
// plan's last step. It establishes every waiting translation that clears
// every bar, as approve-passing does, and records each decision in the ledger
// as the caller's with the reason "review policy".
//
// A project settles on the checks alone once, for its first pass; after
// that a pass waits until the key language is decided (409). Under the
// involvement every nothing is left to the checks (409).
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
	key, err := s.planKeyLanguage(ctx, ws, proj, "", 0, nil)
	if err != nil {
		return serverErr(c, err)
	}
	policy, err := s.planPolicy(ctx, ws, proj, involvementOf(proj), key.Untranslated)
	if err != nil {
		return serverErr(c, err)
	}
	switch policy.Settle {
	case settleNone:
		return apiErr(c, http.StatusConflict, "under the involvement every, a person establishes every translation")
	case settleHeld:
		return apiErr(c, http.StatusConflict, "the first pass is taken; decide the key language before the next one")
	}
	resp, err := s.approvePassing(c, proj, stream, req.Locales, []string{reasonReviewPolicy})
	if err != nil {
		return serverErr(c, err)
	}
	out := reviewPlanSettleResponse{ApprovePassingResponse: resp}
	if policy.Settle == settleFirstPass {
		userID, _ := c.Get("user_id").(string)
		if err := s.updatePlanProperties(ctx, proj.ID, map[string]string{
			propFirstPassBy: userID,
			propFirstPassAt: time.Now().UTC().Format(time.RFC3339),
		}); err != nil {
			return serverErr(c, err)
		}
		out.FirstPass = true
	}
	return c.JSON(http.StatusOK, out)
}

// reviewPlanScanResponse names the scan the plan started.
type reviewPlanScanResponse struct {
	Scan planScanDTO `json:"scan"`
}

// HandleStartReviewPlanScan answers POST /:ws/:id/review/plan/scan: it
// starts a context scan of the project, which files the project's candidate
// terms in the context deck, and records it so the plan shows its state. It
// needs the permission to manage voice profiles, as any context scan does.
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
	job, status, err := s.enqueueContextScan(ctx, wsID, c.Param("ws"), jobs.ContextScanRequest{Project: proj.ID})
	if err != nil {
		return apiErr(c, status, err.Error())
	}
	if err := s.updatePlanProperties(ctx, proj.ID, map[string]string{propTermScanJob: job.ID}); err != nil {
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
	proj, err := s.ContentStore.GetProject(ctx, projectID)
	if err != nil || proj == nil {
		return errNoProject
	}
	if proj.Properties == nil {
		proj.Properties = map[string]string{}
	}
	maps.Copy(proj.Properties, props)
	return s.ContentStore.UpdateProject(ctx, proj)
}
