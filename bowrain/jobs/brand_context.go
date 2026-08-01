package jobs

import (
	"context"
	"log/slog"

	"github.com/neokapi/neokapi/bowrain/core/brandscope"
	"github.com/neokapi/neokapi/core/model"
	brand "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
)

// This file resolves the brand context — voice profile + terminology glossary —
// for a server-side translation job, so every AI translation the worker runs
// (convergence-created jobs, manual enqueue, automation) carries the same
// standing context the CLI flow binds via ApplyProjectBindings. Resolution is
// strictly best-effort: a missing profile, an unreadable terms, or a store
// error degrades the job to a bare translation, never fails it — the context
// is advisory and the checks still report what the model got wrong.

// TermsResolver returns the workspace's server terms, so a translation job can
// build the per-locale glossary that reaches the model's prompt. It mirrors the
// server's workspaceStores.getTB: a per-workspace, PostgreSQL-backed terms
// keyed by the workspace slug. Optional on WorkerDeps — when nil the job
// translates without a glossary.
type TermsResolver interface {
	GetTB(workspaceSlug string) (terms.Terminology, error)
}

// TermsResolverFunc adapts a plain function to the TermsResolver interface.
type TermsResolverFunc func(workspaceSlug string) (terms.Terminology, error)

// GetTB implements TermsResolver.
func (f TermsResolverFunc) GetTB(workspaceSlug string) (terms.Terminology, error) {
	return f(workspaceSlug)
}

// resolveJobBrandProfile resolves the brand voice profile a translation job
// should carry, via the platform's hierarchical binding ladder (brandscope):
// an explicit collection/stream/project binding (brand_voice_profile_id in
// Properties) wins over the workspace-level default profile, and nothing bound
// at any level means no profile — the same resolution the editor and MCP
// scoring surfaces use. The job's target locale selects the profile's locale
// override, so e.g. a per-locale formality adjustment reaches the prompt.
//
// Returns nil (and logs) on any resolution failure: brand voice must never
// fail a translation job.
func resolveJobBrandProfile(ctx context.Context, deps *WorkerDeps, job *TranslationJob) *brand.VoiceProfile {
	if deps == nil || deps.BrandStore == nil {
		return nil
	}
	profile, err := brandscope.Resolve(ctx, deps.ContentStore, deps.WorkspaceDefault, deps.BrandStore, brandscope.Scope{
		WorkspaceID: job.WorkspaceID,
		ProjectID:   job.ProjectID,
		Stream:      "main",
		Locale:      model.LocaleID(job.TargetLocale),
	})
	if err != nil {
		slog.WarnContext(ctx, "brand profile resolution failed; translating without brand voice",
			"job_id", job.ID, "project_id", job.ProjectID, "error", err)
		return nil
	}
	return profile
}

// resolveJobGlossary builds the source→target glossary for a translation job
// from the workspace terms, mirroring the CLI's ResolveProjectGlossary via
// the shared GlossaryFromTerms derivation.
//
// Returns nil (and logs) when no terms resolves, it has no terms for the
// locale pair, or any read fails: terminology must never fail a translation
// job.
func resolveJobGlossary(ctx context.Context, deps *WorkerDeps, job *TranslationJob, sourceLocale, targetLocale model.LocaleID) map[string]string {
	if deps == nil || deps.TermsResolver == nil {
		return nil
	}
	if sourceLocale == "" || targetLocale == "" {
		return nil
	}
	slug := job.WorkspaceSlug
	if slug == "" {
		slug = "_anon"
	}
	tb, err := deps.TermsResolver.GetTB(slug)
	if err != nil || tb == nil {
		if err != nil {
			slog.WarnContext(ctx, "terms resolution failed; translating without glossary",
				"job_id", job.ID, "workspace", slug, "error", err)
		}
		return nil
	}
	glossary, err := GlossaryFromTerms(ctx, tb, job.ProjectID, sourceLocale, targetLocale)
	if err != nil {
		slog.WarnContext(ctx, "terms read failed; translating without glossary",
			"job_id", job.ID, "workspace", slug, "error", err)
		return nil
	}
	return glossary
}

// GlossaryFromTerms derives the source→target glossary a translation
// prompt carries from a workspace terms: for each concept, the
// source-locale term paired with the preferred (or first approved)
// target-locale term becomes a glossary mandate. Workspace-scoped concepts
// (empty ProjectID) and concepts scoped to this project both apply; concepts
// scoped to other projects are excluded, and a project-scoped rendering wins
// over a workspace-scoped one for the same source term. Returns nil (not an
// empty map) when the terms store has no terms for the locale pair.
//
// It is the single derivation shared by every server-side translation
// surface — the worker's jobs (resolveJobGlossary) and the synchronous editor
// translate in bowrain/server — so both mandate identical renderings.
func GlossaryFromTerms(ctx context.Context, tb terms.Terminology, projectID string, sourceLocale, targetLocale model.LocaleID) (map[string]string, error) {
	if tb == nil || sourceLocale == "" || targetLocale == "" {
		return nil, nil
	}
	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return nil, err
	}

	glossary := make(map[string]string)
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
		if _, exists := glossary[src.Text]; exists && (projectScoped[src.Text] || !scoped) {
			// Keep the existing entry unless this one is more specific: a
			// project-scoped rendering replaces a workspace-scoped one; equal
			// specificity keeps the first (Concepts is ordered by ID, so the
			// pick is deterministic across runs).
			continue
		}
		glossary[src.Text] = tgt.Text
		projectScoped[src.Text] = scoped
	}
	if len(glossary) == 0 {
		return nil, nil
	}
	return glossary, nil
}
