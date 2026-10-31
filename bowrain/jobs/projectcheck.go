package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/terms"
)

// ProjectCheckItemName is the sentinel ItemName that marks a job row as a
// project-wide check: the checks at the source locale run over every item
// of a stream, and each item's counts are written as its tally
// (store.ItemCheckTally). Like the model sweep, the job rides the
// translation job table and queue.
const ProjectCheckItemName = "__project_check__"

// NewProjectCheckJob builds the durable job that checks one stream of a
// project, requested by createdBy.
func NewProjectCheckJob(workspaceID, workspaceSlug, projectID, stream, createdBy string) *TranslationJob {
	if stream == "" {
		stream = "main"
	}
	return &TranslationJob{
		ID:            id.New(),
		WorkspaceID:   workspaceID,
		WorkspaceSlug: workspaceSlug,
		ProjectID:     projectID,
		ItemName:      ProjectCheckItemName,
		Stream:        stream,
		CreatedBy:     createdBy,
		Status:        StatusQueued,
	}
}

// IsProjectCheck reports whether the job is a project-wide check.
func (j *TranslationJob) IsProjectCheck() bool { return j.ItemName == ProjectCheckItemName }

// ItemFinding is one finding of the checks at the source locale on a block,
// with the family `kapi check` names it by and the fix it carries.
type ItemFinding struct {
	Finding check.Finding
	// Family is "terms" or "voice" for a source-side finding, and the
	// check's own name for a standard one.
	Family string
	// Fix is the change operation that applies the finding's replacement to
	// the block, nil when it has none.
	Fix *change.Op
}

// ItemCheck names one item to check: the project, the stream, the item and
// the path its content is known by, which a fix is addressed to.
type ItemCheck struct {
	ProjectID   string
	Stream      string
	WorkspaceID string
	Item        string
	Doc         string
}

// CheckItemSource runs the checks at the source locale over one item's
// translatable blocks, in the order the item holds them, and hands each block
// and its findings to visit. One item's blocks are the unit of work: the read
// names the item, and nothing reads the rest of the project. The checks are
// resolved once, at the item's point, and returned.
func CheckItemSource(ctx context.Context, b PointCheckBinding, it ItemCheck, visit func(*venue.StoredBlock, []ItemFinding) error) (PointChecks, error) {
	proj, err := b.Store.GetProject(ctx, it.ProjectID)
	if err != nil {
		return PointChecks{}, fmt.Errorf("check project: %w", err)
	}
	if proj == nil {
		return PointChecks{}, errors.New("check project is unavailable")
	}
	source := proj.DefaultSourceLanguage
	checks, err := ResolvePointChecks(ctx, b, it.ProjectID, it.Stream, it.Item, it.WorkspaceID, source)
	if err != nil {
		return checks, err
	}
	doc := it.Doc
	if doc == "" {
		doc = it.Item
	}
	if it.Item == "" {
		return checks, errors.New("check an item: no item named")
	}
	translatable := true
	blocks, err := b.Store.GetBlocks(ctx, store.BlockQuery{
		ProjectID: it.ProjectID, Stream: it.Stream, ItemName: it.Item, Translatable: &translatable, Order: store.BlockOrderDocument,
	})
	if err != nil {
		return checks, fmt.Errorf("read the blocks of %s: %w", it.Item, err)
	}
	for _, sb := range blocks {
		if sb == nil || sb.Block == nil {
			continue
		}
		found, err := blockSourceFindings(ctx, sb.Block, checks, doc, source)
		if err != nil {
			return checks, err
		}
		if err := visit(sb, found); err != nil {
			return checks, err
		}
	}
	return checks, nil
}

// blockSourceFindings runs the checks on one block, the standard set first
// and then the voice, each finding with its family and fix.
func blockSourceFindings(ctx context.Context, b *model.Block, checks PointChecks, doc string, source model.LocaleID) ([]ItemFinding, error) {
	standard, err := StandardFindings(ctx, b, checks)
	if err != nil {
		return nil, err
	}
	voice, err := VoiceFindings(ctx, b, checks)
	if err != nil {
		return nil, err
	}
	out := make([]ItemFinding, 0, len(standard)+len(voice))
	for _, f := range standard {
		family := f.Check
		if family == "" {
			family = f.Category
		}
		out = append(out, ItemFinding{Finding: f, Family: family})
	}
	rev := check.SourceRevision(b, source)
	for _, f := range voice {
		out = append(out, ItemFinding{
			Finding: f,
			Family:  VoiceFamily(f),
			Fix:     check.Fix(f, b.SourceRuns(), change.Ref{Doc: doc, Block: change.BlockKey(b)}, rev),
		})
	}
	return out, nil
}

// ProjectCheck is one project-wide check of a stream.
type ProjectCheck struct {
	Binding     PointCheckBinding
	Tallies     store.ItemCheckTallyStore
	ProjectID   string
	Stream      string
	WorkspaceID string
	// Progress is told how many items are checked of how many, after each
	// item. Nil reports nothing.
	Progress func(ctx context.Context, done, total int) error
	// Now stamps the tallies. Nil is time.Now.
	Now func() time.Time
}

// ProjectCheckResult is what a project-wide check counted.
type ProjectCheckResult struct {
	Items     int
	Findings  int
	Fixable   int
	CheckedAt time.Time
}

// Run checks every item of the stream at the source locale, item by item,
// and records each item's tally. A block's findings are counted and let go,
// so the check holds one item's blocks at a time.
func (pc ProjectCheck) Run(ctx context.Context) (ProjectCheckResult, error) {
	var res ProjectCheckResult
	if pc.Binding.Store == nil || pc.Tallies == nil {
		return res, errors.New("project check: no content store or no tally store")
	}
	now := pc.Now
	if now == nil {
		now = time.Now
	}
	stream := pc.Stream
	if stream == "" {
		stream = "main"
	}
	items, err := pc.Binding.Store.ListItems(ctx, pc.ProjectID, stream)
	if err != nil {
		return res, fmt.Errorf("project check: list items: %w", err)
	}
	res.CheckedAt = now().UTC()
	for i, it := range items {
		if it == nil {
			continue
		}
		doc := it.Properties[store.ItemPropSourcePath]
		if doc == "" {
			doc = it.Name
		}
		tally := store.ItemCheckTally{ProjectID: pc.ProjectID, Stream: stream, ItemID: it.ID, CheckedAt: res.CheckedAt}
		_, err := CheckItemSource(ctx, pc.Binding, ItemCheck{
			ProjectID: pc.ProjectID, Stream: stream, WorkspaceID: pc.WorkspaceID, Item: it.Name, Doc: doc,
		}, func(_ *venue.StoredBlock, found []ItemFinding) error {
			tally.Findings += len(found)
			for _, f := range found {
				if f.Fix != nil {
					tally.Fixable++
				}
			}
			return nil
		})
		if err != nil {
			return res, fmt.Errorf("project check: %s: %w", it.Name, err)
		}
		if err := pc.Tallies.PutItemCheckTally(ctx, tally); err != nil {
			return res, err
		}
		res.Items++
		res.Findings += tally.Findings
		res.Fixable += tally.Fixable
		if pc.Progress != nil {
			if err := pc.Progress(ctx, i+1, len(items)); err != nil {
				return res, err
			}
		}
	}
	return res, nil
}

// workerCheckBinding is where a worker reads the checks at a point from.
func workerCheckBinding(deps *WorkerDeps, job *TranslationJob) PointCheckBinding {
	b := PointCheckBinding{Store: deps.ContentStore, Voice: deps.VoiceStore, WorkspaceDefault: deps.WorkspaceDefault}
	if deps.TermsResolver != nil {
		slug := job.WorkspaceSlug
		if slug == "" {
			slug = "_anon"
		}
		b.Terms = func(context.Context) (terms.Terminology, error) { return deps.TermsResolver.GetTB(slug) }
	}
	return b
}

// processProjectCheckJob runs a project-wide check. Each item's progress is
// the job's (done and total count items), and each item's tally is written
// as it is counted, so a check that fails part way leaves the items it
// counted with their new tallies. A failure is retried through the shared
// attempt budget: the check writes nothing but tallies, which a second run
// replaces.
func processProjectCheckJob(ctx context.Context, deps *WorkerDeps, job *TranslationJob, epoch int64) error {
	stopHeartbeat := startLeaseHeartbeat(ctx, deps.JobStore, job.ID, epoch)
	defer stopHeartbeat()

	tallies, ok := store.ItemCheckTallies(deps.ContentStore)
	if !ok || job.ProjectID == "" {
		err := fmt.Errorf("project check: job %s has no project or the worker no tally store", job.ID)
		_, _ = deps.JobStore.FailJob(ctx, job.ID, epoch, err.Error())
		return err
	}
	emitLog(deps, job.StepID, "info", "Checking the project's items at the source language",
		map[string]string{"project": job.ProjectID, "stream": job.Stream})
	res, err := ProjectCheck{
		Binding:     workerCheckBinding(deps, job),
		Tallies:     tallies,
		ProjectID:   job.ProjectID,
		Stream:      job.Stream,
		WorkspaceID: job.WorkspaceID,
		Progress: func(ctx context.Context, done, total int) error {
			return deps.JobStore.UpdateJobProgress(ctx, job.ID, epoch, done, total)
		},
	}.Run(ctx)
	if err != nil {
		if errors.Is(err, errLeaseLost) {
			slog.InfoContext(ctx, "project check: lease lost mid-run; abandoning to fresh owner", "job_id", job.ID)
			return nil
		}
		retry, rerr := deps.JobStore.RetryOrFail(ctx, job.ID, epoch, deps.maxJobAttempts(), err.Error())
		if rerr != nil {
			slog.WarnContext(ctx, "project check: retry bookkeeping failed", "job_id", job.ID, "error", rerr)
		}
		if retry {
			return &transientError{err: err}
		}
		return err
	}
	emitLog(deps, job.StepID, "info", fmt.Sprintf("Checked %d items: %d findings, %d with a fix", res.Items, res.Findings, res.Fixable),
		map[string]string{"project": job.ProjectID, "stream": job.Stream})
	if _, err := deps.JobStore.CompleteJob(ctx, job.ID, epoch); err != nil {
		return fmt.Errorf("set completed: %w", err)
	}
	return nil
}
