package server

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/labstack/echo/v4"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/termcandidate"
	"github.com/neokapi/neokapi/bowrain/termscope"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/expand"
)

// The review plan's context step proposes target terms on its own. When a
// term scan of a project completes, a target language is added to it, or a
// push lands on its default stream, the server waits for the triggers to go
// quiet and then starts the whole-project proposal job
// (enqueueTermProposals) when a concept of the project with a source term has
// no proposal and no usable term in a target language. A job never starts
// while another started this way is queued or processing, nor while the
// workspace cannot pay for one; the plan's context step reports the reason
// instead, and the next trigger decides again.
//
// The job is the platform's, on behalf of the project: it records no person,
// and the proposals it files carry termcandidate.ProposerIdentity.
//
// Keeping a proposed term starts a second job that writes the term's forms
// (the plural, the definite form, a case ending) the way `kapi terms expand`
// does, under the same guard.

// termProposalsGroup is the durable consumer group that proposes a project's
// target terms. Stable by design (see convergeOnPushGroup): the group's cursor
// is keyed by this name.
const termProposalsGroup = "term-proposals-auto"

// propTermProposalJob is the last whole-project proposal job the server
// started for the project.
const propTermProposalJob = "review_term_proposal_job"

// termProposalQuiet is how long the triggers for one project must stay quiet
// before its proposal job starts, so a scan that completes right after a push
// starts one job, not two. A variable so tests can shorten it.
var termProposalQuiet = 30 * time.Second

// projectDebouncer runs a function once per key after the calls for that key
// stop for a quiet period. The zero value is ready to use.
type projectDebouncer struct {
	mu     sync.Mutex
	timers map[string]*time.Timer
}

// after runs fn once quiet has passed with no other call for key. A call
// while one waits replaces it and starts the wait again.
func (d *projectDebouncer) after(key string, quiet time.Duration, fn func()) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.timers == nil {
		d.timers = map[string]*time.Timer{}
	}
	if t, ok := d.timers[key]; ok {
		t.Stop()
	}
	var t *time.Timer
	t = time.AfterFunc(quiet, func() {
		d.mu.Lock()
		if d.timers[key] == t {
			delete(d.timers, key)
		}
		d.mu.Unlock()
		fn()
	})
	d.timers[key] = t
}

// termProposalTrigger reports whether an event can leave a project's
// concepts waiting for a target term: a completed term scan, a target
// language added (by a member, not proposed for the recipe), or a push.
func termProposalTrigger(ev platev.Event) bool {
	switch ev.Type {
	case platev.EventContextScanCompleted, platev.EventPushCompleted:
		return true
	case platev.EventRecipeChanged:
		rc := platev.ParseRecipeChange(ev.Data)
		return rc.Source != recipeChangeSourceProposal && rc.Targets != nil && len(rc.Targets.Added()) > 0
	case platev.EventProjectUpdated:
		return ev.Data["new_locales"] != ""
	}
	return false
}

// subscribeTermProposals proposes a project's target terms after the events
// termProposalTrigger names. The handler always acknowledges: the job starts
// after a quiet period, and a job that could not start is logged; the next
// trigger decides again.
func (s *Server) subscribeTermProposals() {
	if s.EventBus == nil || s.ContentStore == nil {
		return
	}
	s.EventBus.SubscribeGroup(termProposalsGroup, func(ev platev.Event) error {
		if ev.ProjectID == "" || !termProposalTrigger(ev) {
			return nil
		}
		if ev.Type == platev.EventPushCompleted && ev.Data["stream"] != "" {
			// A push to another stream changes no concept the job reads.
			if proj, err := s.ContentStore.GetProject(context.Background(), ev.ProjectID); err != nil || proj == nil ||
				pushedStream(ev.Data["stream"], proj) != pushedStream("", proj) {
				return nil
			}
		}
		projectID, wsSlug := ev.ProjectID, ev.Data["workspace_slug"]
		s.proposeLater.after(projectID, termProposalQuiet, func() {
			// Timer callback: the job outlives the event that asked for it,
			// so no request context applies.
			s.maybeProposeTerms(context.Background(), projectID, wsSlug)
		})
		return nil
	})
}

// termProposalsWaiting counts the concept and language pairs of the project
// waiting for a proposed term (termcandidate.NeedsTranslation), over the
// concepts that apply in the project (termscope.AppliesIn).
func termProposalsWaiting(concepts []terms.Concept, proj *platstore.Project) int {
	n := 0
	for i := range concepts {
		if !termscope.AppliesIn(concepts[i], proj.ID) {
			continue
		}
		for _, loc := range proj.TargetLanguages {
			if termcandidate.NeedsTranslation(concepts[i], proj.DefaultSourceLanguage, loc) {
				n++
			}
		}
	}
	return n
}

// lastTermProposals reads the project's last proposal job the server started.
func (s *Server) lastTermProposals(ctx context.Context, proj *platstore.Project, wsID string) *planScanDTO {
	id := proj.Properties[propTermProposalJob]
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

// maybeProposeTerms starts the project's proposal job when a pair waits and
// nothing holds the job back, records it on the project, and answers the job
// id, or "".
func (s *Server) maybeProposeTerms(ctx context.Context, projectID, wsSlug string) string {
	if s.ContextScanStore == nil || s.ContextScanQueue == nil {
		return ""
	}
	proj, err := s.ContentStore.GetProject(ctx, projectID)
	if err != nil || proj == nil {
		slog.WarnContext(ctx, "term proposals: could not load the project", "project", projectID, "error", err)
		return ""
	}
	if len(proj.TargetLanguages) == 0 {
		return ""
	}
	if last := s.lastTermProposals(ctx, proj, proj.WorkspaceID); last != nil && scanRunning(last.Status) {
		return ""
	}
	tb, err := s.projectTermsByID(ctx, proj.WorkspaceID, proj.ID)
	if err != nil || tb == nil {
		return ""
	}
	concepts, err := tb.Concepts(ctx)
	if err != nil {
		slog.WarnContext(ctx, "term proposals: could not read the concepts", "project", proj.ID, "error", err)
		return ""
	}
	waiting := termProposalsWaiting(concepts, proj)
	if waiting == 0 {
		return ""
	}
	wsSlug = s.workspaceSlug(ctx, wsSlug, proj.WorkspaceID)
	if reason, _ := s.termScanBlocked(ctx, proj.WorkspaceID, wsSlug); reason != "" {
		slog.InfoContext(ctx, "term proposals: due but waiting", "project", proj.ID, "pairs", waiting, "reason", reason)
		return ""
	}
	jobID, started, err := s.enqueueTermProposals(ctx, proj.WorkspaceID, wsSlug, proj, nil)
	if err != nil || !started {
		slog.WarnContext(ctx, "term proposals: could not start", "project", proj.ID, "error", err)
		return ""
	}
	if err := s.updatePlanProperties(ctx, proj.ID, map[string]string{propTermProposalJob: jobID}); err != nil {
		slog.WarnContext(ctx, "term proposals: could not record the job", "project", proj.ID, "job", jobID, "error", err)
	}
	slog.InfoContext(ctx, "term proposals: started", "project", proj.ID, "pairs", waiting, "job", jobID)
	return jobID
}

// planProposals sets the context's proposal job and, while pairs wait for a
// proposed term and no job runs, why none starts: waiting_quota (with
// quota_resets_at), waiting_credits or no_ai.
func (s *Server) planProposals(ctx context.Context, wsID, wsSlug string, proj *platstore.Project, k *planContextDTO) {
	k.Proposals = s.lastTermProposals(ctx, proj, wsID)
	if k.Proposable == 0 || (k.Proposals != nil && scanRunning(k.Proposals.Status)) {
		return
	}
	reason, resets := s.termScanBlocked(ctx, wsID, wsSlug)
	if reason == "" {
		return
	}
	k.ProposalsWaiting = reason
	if reason == scanWaitingQuota {
		k.QuotaResetsAt = resets.UTC().Format(time.RFC3339)
	}
}

// expandFormsAfterKeep starts a job that writes the forms of the term a
// reviewer just kept for a concept in locale, the way `kapi terms expand`
// does. It starts none when the term declares forms already, or when the
// workspace cannot pay for the call. A failure is logged and costs only the
// forms: the keep stands.
func (s *Server) expandFormsAfterKeep(c echo.Context, src deckSource, concept terms.Concept, locale model.LocaleID) string {
	ctx := c.Request().Context()
	if s.ContextScanStore == nil || s.ContextScanQueue == nil {
		return ""
	}
	opts := expand.Options{
		SourceLocale: src.project.DefaultSourceLanguage,
		Locales:      []model.LocaleID{locale},
		Concepts:     []string{concept.ID},
	}
	if expand.Count([]terms.Concept{concept}, opts) == 0 {
		return ""
	}
	if reason, _ := s.termScanBlocked(ctx, src.wsID, src.wsSlug); reason != "" {
		slog.InfoContext(ctx, "term forms after keep: waiting", "project", src.project.ID, "concept", concept.ID, "reason", reason)
		return ""
	}
	job, _, err := s.enqueueContextScan(ctx, src.wsID, src.wsSlug, jobs.ContextScanRequest{
		Project:   src.project.ID,
		TermForms: &jobs.TermFormsRequest{Concepts: []string{concept.ID}, Locales: []model.LocaleID{locale}},
	})
	if err != nil {
		slog.WarnContext(ctx, "term forms after keep were not started", "project", src.project.ID, "concept", concept.ID, "error", err)
		return ""
	}
	return job.ID
}
