package jobs

import (
	"context"
	"log/slog"
	"maps"
	"slices"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
)

// This file resolves the governing context — voice profile + term rules —
// for a server-side translation job, so every AI translation the worker runs
// (convergence-created jobs, manual enqueue, automation) carries the same
// standing context the CLI flow binds via ApplyProjectBindings. Resolution is
// strictly best-effort: a missing profile, an unreadable terms store, or a store
// error degrades the job to a bare translation, never fails it — the context
// is advisory and the checks still report what the model got wrong.

// TermsResolver returns the workspace's server terms, so a translation job can
// build the per-locale term rules that reach the model's prompt. It mirrors the
// server's workspaceStores.getTerms: a per-workspace, PostgreSQL-backed terms
// keyed by the workspace slug. Optional on WorkerDeps — when nil the job
// translates without terminology.
type TermsResolver interface {
	GetTB(workspaceSlug string) (terms.Terminology, error)
}

// TermsResolverFunc adapts a plain function to the TermsResolver interface.
type TermsResolverFunc func(workspaceSlug string) (terms.Terminology, error)

// GetTB implements TermsResolver.
func (f TermsResolverFunc) GetTB(workspaceSlug string) (terms.Terminology, error) {
	return f(workspaceSlug)
}

// resolveJobVoiceProfile resolves the voice profile a translation job
// should carry, via the platform's hierarchical binding ladder (voicescope):
// an explicit collection/stream/project binding (voice_profile_id in
// Properties) wins over the workspace-level default profile, and nothing bound
// at any level means no profile — the same resolution the editor and MCP
// scoring surfaces use. The job's target locale selects the profile's locale
// override, so e.g. a per-locale formality adjustment reaches the prompt.
//
// It resolves through the shared binding, so a job scores its drafts against
// the profile its translation was produced under.
//
// Returns nil (and logs) on any resolution failure: voice must never
// fail a translation job.
func resolveJobVoiceProfile(ctx context.Context, deps *WorkerDeps, job *TranslationJob) *coreprofile.VoiceProfile {
	b := jobTranslateBinding(deps, job, nil)
	return b.VoiceProfile(ctx, b.Collection(ctx))
}

// resolveJobTerms returns the workspace terms a job's terminology derives from,
// or nil when the worker has no resolver, the workspace has no store, or the
// read fails: terminology must never fail a translation job.
func resolveJobTerms(deps *WorkerDeps, job *TranslationJob) terms.Terminology {
	if deps == nil || deps.TermsResolver == nil {
		return nil
	}
	slug := job.WorkspaceSlug
	if slug == "" {
		slug = "_anon"
	}
	tb, err := deps.TermsResolver.GetTB(slug)
	if err != nil {
		slog.Warn("terms resolution failed; translating without terminology",
			"job_id", job.ID, "workspace", slug, "error", err)
		return nil
	}
	if tb == nil {
		return nil
	}
	return tb
}

// TermRulesFromConcepts derives the term rules a translation carries from a
// workspace terms store: for each concept, the source-locale term paired with
// the preferred (or first approved) target-locale term becomes one rule —
// Term in the source, Replacement required in the target. Workspace-scoped
// concepts (empty ProjectID) and concepts scoped to this project both apply;
// concepts scoped to other projects are excluded, and a project-scoped
// rendering wins over a workspace-scoped one for the same source term. Returns
// nil (not an empty slice) when the terms store has no terms for the locale
// pair.
//
// Rules come back ordered by term, so one terms store yields one prompt and one
// fingerprint however the concepts were read.
//
// It is the single derivation shared by every server-side translation surface —
// the worker's jobs (resolveJobTermRules) and the synchronous editor translate
// in bowrain/server — so both mandate identical renderings, and it is the
// server's mirror of the CLI's host.ResolveTermRules.
func TermRulesFromConcepts(ctx context.Context, tb terms.Terminology, projectID string, sourceLocale, targetLocale model.LocaleID) ([]coreprofile.TermRule, error) {
	if tb == nil || sourceLocale == "" || targetLocale == "" {
		return nil, nil
	}
	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return nil, err
	}

	replacement := make(map[string]string)
	projectScoped := make(map[string]bool)
	for i := range concepts {
		concept := &concepts[i]
		if concept.ProjectID != "" && concept.ProjectID != projectID {
			continue // another project's terminology
		}
		src := concept.SourceTerm(sourceLocale)
		if src == nil || src.Text == "" {
			continue
		}
		tgt := concept.PreferredTerm(targetLocale)
		if tgt == nil || tgt.Text == "" {
			continue
		}
		scoped := concept.ProjectID == projectID && concept.ProjectID != ""
		if _, exists := replacement[src.Text]; exists && (projectScoped[src.Text] || !scoped) {
			// Keep the existing rule unless this one is more specific: a
			// project-scoped rendering replaces a workspace-scoped one; equal
			// specificity keeps the first (Concepts is ordered by ID, so the
			// pick is deterministic across runs).
			continue
		}
		replacement[src.Text] = tgt.Text
		projectScoped[src.Text] = scoped
	}
	if len(replacement) == 0 {
		return nil, nil
	}
	rules := make([]coreprofile.TermRule, 0, len(replacement))
	for _, term := range slices.Sorted(maps.Keys(replacement)) {
		rules = append(rules, coreprofile.TermRule{Term: term, Replacement: replacement[term]})
	}
	return rules, nil
}
