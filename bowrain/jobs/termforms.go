package jobs

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/expand"
)

// termFormsMaxConcepts caps the concepts one term-forms job asks about, so a
// request cannot fan out into unbounded provider spend.
const termFormsMaxConcepts = 50

// TermFormsRequest asks a context-scan job to propose the surface forms (the
// plural, the definite form, a case ending) of the target terms of the named
// concepts, as `kapi terms expand` does, and to write them onto the terms. A
// term that declares forms already keeps them.
type TermFormsRequest struct {
	// Concepts are the concept ids to expand. A job naming none fails.
	Concepts []string `json:"concepts"`
	// Locales narrows the terms to these languages. Empty is every target
	// term: each language except the project's source language.
	Locales []model.LocaleID `json:"locales,omitempty"`
}

// TermFormsSummary is what a term-forms run did.
type TermFormsSummary struct {
	// Asked counts the terms sent to the AI.
	Asked int `json:"asked"`
	// Written counts the terms that gained forms.
	Written int `json:"written"`
}

// executeTermForms runs a term-forms job: it reads the workspace's concepts,
// asks the platform AI for the forms of each selected term through the same
// code as `kapi terms expand` (expand.Propose), and writes the forms
// onto each concept as read now, so a decision made while the job ran is not
// overwritten. The usage is billed and recorded like a scan phase, after a
// fresh lease check.
func executeTermForms(ctx context.Context, deps *ContextScanWorkerDeps, job *ContextScanJob, epoch int64, req *ContextScanRequest) error {
	if len(req.TermForms.Concepts) == 0 {
		return errors.New("a term-forms job names no concept")
	}
	if len(req.TermForms.Concepts) > termFormsMaxConcepts {
		return fmt.Errorf("a term-forms job names %d concepts; the limit is %d", len(req.TermForms.Concepts), termFormsMaxConcepts)
	}
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
	opts := expand.Options{
		SourceLocale: sourceLocale(project),
		Locales:      req.TermForms.Locales,
		Concepts:     req.TermForms.Concepts,
	}
	summary := TermFormsSummary{Asked: expand.Count(concepts, opts)}

	totalTokens := 0
	if summary.Asked > 0 {
		platform := activePlatform(ctx, "", deps.Platform, deps.PlatformResolver)
		if platform == nil {
			return errors.New("platform provider not configured")
		}
		prov, provKind, err := platform.Build("")
		if err != nil {
			return fmt.Errorf("build platform provider: %w", err)
		}
		usageModel := cmp.Or(platform.Model, provKind)

		setContextScanProgress(ctx, deps, job.ID, epoch, contextScanProgressTerms, "expanding-terms")
		meter := &usageMeter{LLMProvider: prov}
		proposals, perr := expand.Propose(ctx, meter, concepts, opts)
		usage := meter.total()
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
				deps.BillingHooks.DeductTokens(ctx, job.WorkspaceID, tokens, "context_scan", job.ID+":forms")
			}
			totalTokens = tokens
		}
		if perr != nil {
			return perr
		}
		n, ferr := fileTermForms(ctx, tb, proposals)
		summary.Written = n
		if ferr != nil {
			return ferr
		}
	}

	resultJSON, err := json.Marshal(ContextScanResult{Artefacts: []ArtefactProposal{}, Sources: []ContextScanSource{}, TermForms: &summary})
	if err != nil {
		return fmt.Errorf("encode term-forms result: %w", err)
	}
	owner, err := deps.Store.CompleteContextScanJob(ctx, job.ID, epoch, resultJSON, totalTokens)
	if err != nil {
		return fmt.Errorf("complete term-forms job: %w", err)
	}
	if !owner {
		return errLeaseLost
	}
	emitContextScanLog(deps, job.ID, "info",
		fmt.Sprintf("Wrote forms onto %d of %d target terms, %d tokens", summary.Written, summary.Asked, totalTokens), nil)
	return nil
}

// fileTermForms writes each proposal's forms onto its concept as read now,
// and returns how many terms gained forms. The concept's UpdatedAt is left
// as it was: the forms change how a check finds the term, not the decision
// the context deck attested.
func fileTermForms(ctx context.Context, tb terms.Terminology, proposals []expand.Proposal) (int, error) {
	byConcept := map[string][]expand.Proposal{}
	var order []string
	for _, p := range proposals {
		if len(p.Forms) == 0 {
			continue
		}
		if _, ok := byConcept[p.ConceptID]; !ok {
			order = append(order, p.ConceptID)
		}
		byConcept[p.ConceptID] = append(byConcept[p.ConceptID], p)
	}
	written := 0
	for _, cid := range order {
		cp, ok, err := tb.GetConcept(ctx, cid)
		if err != nil {
			return written, fmt.Errorf("read concept %s: %w", cid, err)
		}
		if !ok {
			continue
		}
		one := []terms.Concept{cp}
		changed, n := expand.Apply(one, byConcept[cid], false)
		if !changed[cid] {
			continue
		}
		if err := tb.AddConcept(ctx, one[0]); err != nil {
			return written, fmt.Errorf("write the forms of %s: %w", cid, err)
		}
		written += n
	}
	return written, nil
}

// usageMeter sums the token usage of every call made through it, for a tool
// that reports none of its own.
type usageMeter struct {
	aiprovider.LLMProvider
	mu    sync.Mutex
	usage aiprovider.TokenUsage
}

func (m *usageMeter) add(resp *aiprovider.ChatResponse) {
	if resp == nil {
		return
	}
	m.mu.Lock()
	m.usage = m.usage.Add(resp.Usage)
	m.mu.Unlock()
}

func (m *usageMeter) total() aiprovider.TokenUsage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.usage
}

func (m *usageMeter) Chat(ctx context.Context, messages []aiprovider.Message) (*aiprovider.ChatResponse, error) {
	resp, err := m.LLMProvider.Chat(ctx, messages)
	m.add(resp)
	return resp, err
}

func (m *usageMeter) ChatStructured(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
	resp, err := m.LLMProvider.ChatStructured(ctx, messages, schema)
	m.add(resp)
	return resp, err
}
