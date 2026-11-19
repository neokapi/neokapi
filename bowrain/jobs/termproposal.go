package jobs

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/termcandidate"
	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/terms"
)

// Bounds on one term-proposal run.
const (
	// termProposalMaxRequests caps the concept and language pairs one run
	// asks about, widest reach first, so a large terms store cannot fan out
	// into unbounded provider spend. A later run picks up the rest.
	termProposalMaxRequests = 300
	// termProposalBatch is how many concepts one prompt carries.
	termProposalBatch = 25
	// termProposalMaxBlocks caps the blocks the example walk reads.
	termProposalMaxBlocks = 20000
)

// TermProposalRequest asks a context-scan job to propose target-language
// terms for the concepts of ContextScanRequest.Project instead of scanning:
// for each concept with a source term and no usable term in one of the
// project's target languages, the term the AI proposes is filed on the
// concept for review (termcandidate.SetTranslation).
type TermProposalRequest struct {
	// Concepts narrows the run to these concept ids. Empty is every concept
	// of the project.
	Concepts []string `json:"concepts,omitempty"`
}

// TermProposalSummary is what a term-proposal run did.
type TermProposalSummary struct {
	// Asked counts the concept and language pairs sent to the AI.
	Asked int `json:"asked"`
	// Proposed counts the proposals filed.
	Proposed int `json:"proposed"`
	// Remaining counts the pairs left for a later run by the cap.
	Remaining int `json:"remaining"`
}

// termProposalTarget is one concept waiting for a term in one language.
type termProposalTarget struct {
	concept terms.Concept
	locale  model.LocaleID
	reach   int
}

// termProposalTargets lists the concept and language pairs of the project
// that wait for a proposal (termcandidate.NeedsTranslation), widest reach
// first. only, when non-empty, narrows it to those concept ids.
func termProposalTargets(concepts []terms.Concept, project *platstore.Project, only []string) []termProposalTarget {
	source := sourceLocale(project)
	var out []termProposalTarget
	for _, cp := range concepts {
		if p := termcandidate.ProjectOf(cp); p != "" && p != project.ID {
			continue
		}
		if len(only) > 0 && !slices.Contains(only, cp.ID) {
			continue
		}
		reach, _ := termcandidate.ReachOf(cp)
		for _, loc := range project.TargetLanguages {
			if termcandidate.NeedsTranslation(cp, source, loc) {
				out = append(out, termProposalTarget{concept: cp, locale: loc, reach: reach.Blocks})
			}
		}
	}
	slices.SortStableFunc(out, func(a, b termProposalTarget) int {
		return cmp.Or(cmp.Compare(b.reach, a.reach), strings.Compare(a.concept.ID, b.concept.ID), strings.Compare(string(a.locale), string(b.locale)))
	})
	return out
}

// errExamplesFull ends the example walk once every term has its examples.
var errExamplesFull = errors.New("examples gathered")

// termExamples walks the project's source blocks that hold one of the
// terms, in batches, and gathers a few that use each.
func termExamples(ctx context.Context, cs platstore.ContentStore, project *platstore.Project, sourceTerms []string) (*termcandidate.Examples, error) {
	ex := termcandidate.NewExamples(sourceLocale(project), sourceTerms, tools.MaxTermTranslateExamples)
	if cs == nil || len(ex.Terms()) == 0 {
		return ex, nil
	}
	translatable := true
	read := 0
	err := platstore.EachBlockBatch(ctx, cs, platstore.BlockQuery{
		ProjectID: project.ID, Stream: project.DefaultStream, Translatable: &translatable, TextAny: ex.Terms(),
	}, 0, func(batch []*venue.StoredBlock) error {
		for _, b := range batch {
			if b == nil || b.Block == nil {
				continue
			}
			ex.Add(b.SourceText())
		}
		read += len(batch)
		if ex.Full() || read >= termProposalMaxBlocks {
			return errExamplesFull
		}
		return nil
	})
	if errors.Is(err, errExamplesFull) {
		err = nil
	}
	return ex, err
}

// executeTermProposals runs a term-proposal job: it reads the project's
// concepts, gathers examples, asks the platform AI for each target term in
// batches per language, and files every answer on its concept, re-reading
// the concept first so a decision made while the job ran is not
// overwritten. Each batch is billed and recorded like a scan phase, gated on
// a fresh lease check.
func executeTermProposals(ctx context.Context, deps *ContextScanWorkerDeps, job *ContextScanJob, epoch int64, req *ContextScanRequest) error {
	project, err := deps.scanProjectRow(ctx, job.WorkspaceID, req.Project)
	if err != nil {
		return err
	}
	if deps.TermsResolver == nil {
		return errors.New("the worker has no terms store")
	}
	tb, err := deps.TermsResolver.GetTB(job.WorkspaceSlug)
	if err != nil || tb == nil {
		return fmt.Errorf("open the terms store: %w", err)
	}
	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return fmt.Errorf("read the terms store: %w", err)
	}
	setContextScanProgress(ctx, deps, job.ID, epoch, contextScanProgressReading, "reading-terms")
	targets := termProposalTargets(concepts, project, req.TermProposals.Concepts)
	summary := TermProposalSummary{}
	if len(targets) > termProposalMaxRequests {
		summary.Remaining = len(targets) - termProposalMaxRequests
		targets = targets[:termProposalMaxRequests]
	}

	totalTokens := 0
	if len(targets) > 0 {
		source := sourceLocale(project)
		var sourceTerms []string
		for _, t := range targets {
			sourceTerms = append(sourceTerms, termcandidate.SourceText(t.concept, source))
		}
		ex, err := termExamples(ctx, deps.ContentStore, project, sourceTerms)
		if err != nil {
			// Examples sharpen the answer; a proposal without them is
			// still one a person reviews.
			slog.WarnContext(ctx, "term-proposal example walk failed", "job_id", job.ID, "error", err)
		}

		platform := activePlatform(ctx, "", deps.Platform, deps.PlatformResolver)
		if platform == nil {
			return errors.New("platform provider not configured")
		}
		prov, provKind, err := platform.Build("")
		if err != nil {
			return fmt.Errorf("build platform provider: %w", err)
		}
		usageModel := cmp.Or(platform.Model, provKind)

		setContextScanProgress(ctx, deps, job.ID, epoch, contextScanProgressTerms, "proposing-terms")
		byLocale := map[model.LocaleID][]termProposalTarget{}
		var locales []model.LocaleID
		for _, t := range targets {
			if _, ok := byLocale[t.locale]; !ok {
				locales = append(locales, t.locale)
			}
			byLocale[t.locale] = append(byLocale[t.locale], t)
		}
		for _, loc := range locales {
			for batch := range slices.Chunk(byLocale[loc], termProposalBatch) {
				reqs := make([]tools.TermTranslateRequest, len(batch))
				for i, t := range batch {
					term := termcandidate.SourceText(t.concept, source)
					reqs[i] = tools.TermTranslateRequest{
						ID: t.concept.ID, Term: term, Definition: t.concept.Definition,
						TurnedDown: termcandidate.RejectedTranslations(t.concept, loc),
					}
					if ex != nil {
						reqs[i].Examples = ex.For(term)
					}
				}
				answers, usage, perr := tools.ProposeTermTranslations(ctx, prov, source, loc, reqs)
				summary.Asked += len(reqs)
				if tokens := usage.TotalTokens(); tokens > 0 {
					owner, lerr := deps.Store.RenewLease(ctx, job.ID, epoch)
					if lerr != nil {
						return fmt.Errorf("renew lease: %w", lerr)
					}
					if !owner {
						return errLeaseLost
					}
					recordContextScanUsage(ctx, deps, job, usageModel, usage, tokens)
					if deps.BillingHooks != nil && job.WorkspaceID != "" {
						deps.BillingHooks.DeductTokens(ctx, job.WorkspaceID, tokens, "context_scan", fmt.Sprintf("%s:terms:%s:%d", job.ID, loc, summary.Asked))
					}
					if terr := deps.Store.AddContextScanTokens(ctx, job.ID, epoch, tokens); terr != nil {
						slog.WarnContext(ctx, "term-proposal token bookkeeping failed", "job_id", job.ID, "error", terr)
					}
					totalTokens += tokens
				}
				if perr != nil {
					return perr
				}
				n, ferr := fileTermProposals(ctx, tb, source, loc, answers)
				summary.Proposed += n
				if ferr != nil {
					return ferr
				}
			}
		}
	}

	resultJSON, err := json.Marshal(ContextScanResult{Artefacts: []ArtefactProposal{}, Sources: []ContextScanSource{}, TermProposals: &summary})
	if err != nil {
		return fmt.Errorf("encode term-proposal result: %w", err)
	}
	owner, err := deps.Store.CompleteContextScanJob(ctx, job.ID, epoch, resultJSON, totalTokens)
	if err != nil {
		return fmt.Errorf("complete term-proposal job: %w", err)
	}
	if !owner {
		return errLeaseLost
	}
	emitContextScanLog(deps, job.ID, "info",
		fmt.Sprintf("Proposed %d target terms for %d concept and language pairs, %d tokens", summary.Proposed, summary.Asked, totalTokens), nil)
	return nil
}

// fileTermProposals files each answer on its concept as read now, and
// returns how many it filed. The concept's UpdatedAt is left as it was, so
// the write does not move the revision the context deck attests.
func fileTermProposals(ctx context.Context, tb terms.Terminology, source, target model.LocaleID, answers []tools.TermTranslation) (int, error) {
	filed := 0
	for _, a := range answers {
		cp, ok, err := tb.GetConcept(ctx, a.ID)
		if err != nil {
			return filed, fmt.Errorf("read concept %s: %w", a.ID, err)
		}
		if !ok || !termcandidate.SetTranslation(&cp, source, target, a.Term) {
			continue
		}
		if err := tb.AddConcept(ctx, cp); err != nil {
			return filed, fmt.Errorf("file the proposal for %s: %w", a.ID, err)
		}
		filed++
	}
	return filed, nil
}

// scanProjectRow reads the project a job names, refusing one outside the
// job's workspace.
func (d *ContextScanWorkerDeps) scanProjectRow(ctx context.Context, workspaceID, projectID string) (*platstore.Project, error) {
	if d.ContentStore == nil {
		return nil, errors.New("the worker has no content store")
	}
	project, err := d.ContentStore.GetProject(ctx, strings.TrimSpace(projectID))
	if err != nil || project == nil || workspaceID == "" || project.WorkspaceID != workspaceID {
		return nil, errors.New("the project could not be read")
	}
	return project, nil
}
