package server

import (
	"context"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/core/model"
)

// The review plan's term scan starts on its own. When a push lands, the
// server compares what the project holds with what its last term scan read
// and starts a scan when one is due (termScanDue). A scan never starts while
// another is queued or processing, nor while the workspace cannot pay for one;
// the plan reports that reason instead, and the next push decides again.
// POST /review/plan/scan scans again by hand.

// termScanOnPushGroup is the durable consumer group that starts a project's
// term scan after a push. Stable by design (see convergeOnPushGroup): the
// group's cursor is keyed by this name.
const termScanOnPushGroup = "term-scan-on-push"

// Project properties recording what the last term scan read: the source
// blocks of the default stream and the names of its collections, both taken
// when the scan was started.
const (
	propTermScanBlocks      = "review_term_scan_blocks"
	propTermScanCollections = "review_term_scan_collections"
)

// The growth in source blocks that makes a scan due again: 500 blocks, or
// one in five of the blocks the last scan read (20%), whichever comes first.
const (
	termScanGrowthBlocks = 500
	termScanGrowthShare  = 5
)

// Why the plan's term scan waits instead of running. The plan carries the
// reason in place of a scan status.
const (
	// scanWaitingContent: the project holds no source text yet.
	scanWaitingContent = "waiting_content"
	// scanWaitingPush: the project holds source text and has had no scan;
	// the next push starts one.
	scanWaitingPush = "waiting_push"
	// scanWaitingQuota: the workspace used its monthly AI limit. The scan
	// starts after the limit resets (quota_resets_at).
	scanWaitingQuota = "waiting_quota"
	// scanWaitingCredits: the workspace has no AI credits left.
	scanWaitingCredits = "waiting_credits"
	// scanNoAI: the platform has no AI provider configured.
	scanNoAI = "no_ai"
)

// termScanCoverage is what a term scan reads: the translatable blocks of the
// project's default stream and its collections, by name and sorted.
type termScanCoverage struct {
	Blocks      int
	Collections []string
}

// props is the coverage as the project properties that record it.
func (c termScanCoverage) props() map[string]string {
	return map[string]string{
		propTermScanBlocks:      strconv.Itoa(c.Blocks),
		propTermScanCollections: strings.Join(c.Collections, "\n"),
	}
}

// recordedTermScanCoverage reads the coverage the last scan recorded, and
// false when no scan recorded one.
func recordedTermScanCoverage(p *platstore.Project) (termScanCoverage, bool) {
	raw, ok := p.Properties[propTermScanBlocks]
	if !ok {
		return termScanCoverage{}, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return termScanCoverage{}, false
	}
	c := termScanCoverage{Blocks: n}
	if names := p.Properties[propTermScanCollections]; names != "" {
		c.Collections = strings.Split(names, "\n")
	}
	return c, true
}

// termScanCoverageOf counts what a scan of the project would read now.
func (s *Server) termScanCoverageOf(ctx context.Context, proj *platstore.Project) (termScanCoverage, error) {
	stream := pushedStream("", proj)
	translatable := true
	counts, err := s.ContentStore.CountBlocks(ctx, platstore.BlockQuery{
		ProjectID: proj.ID, Stream: stream, Translatable: &translatable,
	})
	if err != nil {
		return termScanCoverage{}, err
	}
	colls, err := s.ContentStore.ListCollections(ctx, proj.ID, stream)
	if err != nil {
		return termScanCoverage{}, err
	}
	out := termScanCoverage{Blocks: counts.Total, Collections: make([]string, 0, len(colls))}
	for _, c := range colls {
		out.Collections = append(out.Collections, c.Name)
	}
	slices.Sort(out.Collections)
	return out, nil
}

// termScanDue reports whether a term scan should start, and why. last is the
// project's last scan (nil when it never had one), recorded the coverage that
// scan recorded (hasRecorded false when none), now what the project holds,
// and concepts the workspace's concepts with a source term.
//
// A scan is due when the project holds source text and one of these holds:
//
//   - the project has never had a term scan, and the workspace has no concept
//     with a term in the project's source language;
//   - the last scan failed;
//   - a collection exists that the last scan did not read;
//   - the source blocks grew since the last scan by termScanGrowthBlocks, or
//     by one block in termScanGrowthShare of those it read.
func termScanDue(last *planScanDTO, recorded termScanCoverage, hasRecorded bool, now termScanCoverage, concepts int) (bool, string) {
	if now.Blocks == 0 {
		return false, ""
	}
	if last == nil {
		if concepts == 0 {
			return true, "first content"
		}
		return false, ""
	}
	if scanRunning(last.Status) {
		return false, ""
	}
	if last.Status == string(jobs.ContextScanStatusFailed) {
		return true, "the last scan failed"
	}
	if !hasRecorded {
		return false, ""
	}
	for _, c := range now.Collections {
		if !slices.Contains(recorded.Collections, c) {
			return true, "new collection"
		}
	}
	growth := now.Blocks - recorded.Blocks
	if growth > 0 && (growth >= termScanGrowthBlocks || growth*termScanGrowthShare >= recorded.Blocks) {
		return true, "new source text"
	}
	return false, ""
}

// termScanBlocked names what keeps a term scan from running now, or "" when
// nothing does. For the AI limit it also answers when the limit resets.
func (s *Server) termScanBlocked(ctx context.Context, wsID, wsSlug string) (string, time.Time) {
	if s.PlatformConfig == nil || s.PlatformConfig.AIProvider() == "" {
		return scanNoAI, time.Time{}
	}
	if s.QuotaStore != nil && wsSlug != "" {
		remaining, err := s.QuotaStore.CheckQuota(ctx, wsSlug)
		if err != nil {
			// The worker checks the limit again before it spends anything.
			slog.WarnContext(ctx, "term scan: AI limit check failed", "workspace", wsSlug, "error", err)
		} else if remaining <= 0 {
			return scanWaitingQuota, jobs.QuotaResetsAt(time.Now())
		}
	}
	if s.insufficientPlatformCredits(ctx, wsID, "platform") {
		return scanWaitingCredits, time.Time{}
	}
	return "", time.Time{}
}

// sourceConcepts counts the workspace's concepts with a term in the
// language, zero when the workspace has no terms store.
func (s *Server) sourceConcepts(ctx context.Context, wsID string, src model.LocaleID) (int, error) {
	tb, err := s.workspaceTermsByID(ctx, wsID)
	if err != nil || tb == nil {
		return 0, nil //nolint:nilerr // a workspace with no terms store has no concepts to count
	}
	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range concepts {
		if concepts[i].HeadTerm(src) != nil {
			n++
		}
	}
	return n, nil
}

// startProjectTermScan enqueues a term scan of the project and records it on
// the project, with the coverage it reads. A nil coverage is counted here;
// when it cannot be, the job is still recorded.
func (s *Server) startProjectTermScan(ctx context.Context, proj *platstore.Project, wsSlug string, cov *termScanCoverage) (*jobs.ContextScanJob, int, error) {
	job, status, err := s.enqueueContextScan(ctx, proj.WorkspaceID, wsSlug, jobs.ContextScanRequest{Project: proj.ID})
	if err != nil {
		return nil, status, err
	}
	props := map[string]string{propTermScanJob: job.ID}
	if cov == nil {
		if c, cerr := s.termScanCoverageOf(ctx, proj); cerr == nil {
			cov = &c
		} else {
			slog.WarnContext(ctx, "term scan: could not count what the scan reads", "project", proj.ID, "error", cerr)
		}
	}
	if cov != nil {
		maps.Copy(props, cov.props())
	}
	if err := s.updatePlanProperties(ctx, proj.ID, props); err != nil {
		return job, status, err
	}
	return job, status, nil
}

// subscribeTermScanOnPush starts a project's term scan after a push when one
// is due. The handler always acknowledges: a scan that could not start is
// logged, and the next push decides again.
func (s *Server) subscribeTermScanOnPush() {
	if s.EventBus == nil || s.ContentStore == nil {
		return
	}
	s.EventBus.SubscribeGroup(termScanOnPushGroup, func(ev platev.Event) error {
		if ev.Type != platev.EventPushCompleted || ev.ProjectID == "" {
			return nil
		}
		// Event-bus callback: the scan outlives the push that published the
		// event, so no request context applies.
		s.maybeStartTermScan(context.Background(), ev.ProjectID, ev.Data["stream"], ev.Data["workspace_slug"])
		return nil
	})
}

// maybeStartTermScan starts the project's term scan when one is due after a
// push to stream, and answers the job it started, or nil.
func (s *Server) maybeStartTermScan(ctx context.Context, projectID, stream, wsSlug string) *jobs.ContextScanJob {
	if s.ContextScanStore == nil || s.ContextScanQueue == nil {
		return nil
	}
	proj, err := s.ContentStore.GetProject(ctx, projectID)
	if err != nil || proj == nil {
		slog.WarnContext(ctx, "term scan: could not load the pushed project", "project", projectID, "error", err)
		return nil
	}
	// A scan reads the default stream, so a push to another stream changes
	// nothing it reads.
	if pushedStream(stream, proj) != pushedStream("", proj) {
		return nil
	}
	last := s.planScan(ctx, proj, proj.WorkspaceID)
	if last != nil && scanRunning(last.Status) {
		return nil
	}
	now, err := s.termScanCoverageOf(ctx, proj)
	if err != nil {
		slog.WarnContext(ctx, "term scan: could not count the project's content", "project", proj.ID, "error", err)
		return nil
	}
	recorded, hasRecorded := recordedTermScanCoverage(proj)
	if last != nil && !hasRecorded && last.Status != string(jobs.ContextScanStatusFailed) {
		// A scan with no recorded coverage is measured from here on.
		if err := s.updatePlanProperties(ctx, proj.ID, now.props()); err != nil {
			slog.WarnContext(ctx, "term scan: could not record the coverage", "project", proj.ID, "error", err)
		}
		return nil
	}
	concepts, err := s.sourceConcepts(ctx, proj.WorkspaceID, proj.DefaultSourceLanguage)
	if err != nil {
		slog.WarnContext(ctx, "term scan: could not count the concepts", "project", proj.ID, "error", err)
		return nil
	}
	due, why := termScanDue(last, recorded, hasRecorded, now, concepts)
	if !due {
		return nil
	}
	wsSlug = s.workspaceSlug(ctx, wsSlug, proj.WorkspaceID)
	if reason, _ := s.termScanBlocked(ctx, proj.WorkspaceID, wsSlug); reason != "" {
		slog.InfoContext(ctx, "term scan: due but waiting", "project", proj.ID, "trigger", why, "reason", reason)
		return nil
	}
	job, _, err := s.startProjectTermScan(ctx, proj, wsSlug, &now)
	if err != nil {
		slog.WarnContext(ctx, "term scan: could not start", "project", proj.ID, "trigger", why, "error", err)
		return nil
	}
	slog.InfoContext(ctx, "term scan: started after a push", "project", proj.ID, "trigger", why, "job", job.ID)
	return job
}

// planScanWaiting sets why the context's term scan waits, while the
// language has not started and no scan is running.
func (s *Server) planScanWaiting(ctx context.Context, wsID, wsSlug string, proj *platstore.Project, k *planContextDTO) {
	if !contextNotStarted(*k) || (k.Scan != nil && scanRunning(k.Scan.Status)) {
		return
	}
	cov, err := s.termScanCoverageOf(ctx, proj)
	if err != nil {
		slog.WarnContext(ctx, "review plan: could not count the project's content", "project", proj.ID, "error", err)
		return
	}
	if cov.Blocks == 0 {
		k.ScanWaiting = scanWaitingContent
		return
	}
	reason, resets := s.termScanBlocked(ctx, wsID, wsSlug)
	switch {
	case reason == scanWaitingQuota:
		k.ScanWaiting, k.QuotaResetsAt = reason, resets.UTC().Format(time.RFC3339)
	case reason != "":
		k.ScanWaiting = reason
	case k.Scan == nil:
		k.ScanWaiting = scanWaitingPush
	}
}
