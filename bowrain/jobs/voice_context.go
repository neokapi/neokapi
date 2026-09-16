package jobs

import (
	"context"
	"log/slog"
	"slices"
	"strings"

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
// workspace terms store. A concept naming a target-locale term yields a rule
// requiring that rendering: Term in the source, Replacement in the target. A
// concept marked do-not-translate yields a rule requiring the source term
// verbatim, which names no replacement and answers for every target language,
// including one the store has never heard of.
//
// Workspace-scoped concepts (empty ProjectID) and concepts scoped to this
// project both apply; concepts scoped to other projects are excluded, and a
// project-scoped claim wins over a workspace-scoped one for the same term.
// Returns nil (not an empty slice) when the terms store has no terms for the
// locale pair.
//
// Rules come back ordered by term, and a keep-verbatim claim before a rendering
// for the same term, so one terms store yields one prompt and one fingerprint
// however the concepts were read.
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

	byClaim := make(map[termClaim]coreprofile.TermRule)
	projectScoped := make(map[termClaim]bool)
	for _, concept := range concepts {
		if concept.ProjectID != "" && concept.ProjectID != projectID {
			continue // another project's terminology
		}
		// The one derivation every surface shares (terms.RuleForConcept), so
		// the jobs mandate the renderings and forms the CLI gate accepts.
		rule, ok := terms.RuleForConcept(concept, sourceLocale, targetLocale)
		// A rule with neither a rendering nor a keep-verbatim claim demands
		// nothing: "say this instead" needs a this. A do-not-translate rule has
		// no replacement by construction and is exactly the case this used to
		// drop, which left the ship gate enforcing a concept that never reached
		// the drafter or recycle.
		if !ok || (rule.Replacement == "" && !rule.DoNotTranslate) {
			continue
		}
		claim := termClaim{term: rule.Term, keepVerbatim: rule.DoNotTranslate}
		scoped := concept.ProjectID == projectID && concept.ProjectID != ""
		if _, exists := byClaim[claim]; exists && (projectScoped[claim] || !scoped) {
			// Keep the existing rule unless this one is more specific: a
			// project-scoped claim replaces a workspace-scoped one; equal
			// specificity keeps the first (Concepts is ordered by ID, so the
			// pick is deterministic across runs).
			continue
		}
		byClaim[claim] = rule
		projectScoped[claim] = scoped
	}
	if len(byClaim) == 0 {
		return nil, nil
	}
	claims := make([]termClaim, 0, len(byClaim))
	for claim := range byClaim {
		claims = append(claims, claim)
	}
	slices.SortFunc(claims, func(a, b termClaim) int {
		if a.term != b.term {
			return strings.Compare(a.term, b.term)
		}
		if a.keepVerbatim == b.keepVerbatim {
			return 0
		}
		if a.keepVerbatim {
			return -1
		}
		return 1
	})
	rules := make([]coreprofile.TermRule, 0, len(byClaim))
	for _, claim := range claims {
		rules = append(rules, byClaim[claim])
	}
	return rules, nil
}

// termClaim identifies what a rule demands of one word, which is what the
// derivation deduplicates on. Two concepts can claim the same source term, one
// requiring it verbatim and one naming a rendering, and those are separate
// demands: the ship gate derives from the framework derivation, which holds a
// target to both, so collapsing them here is how the gate and the drafter came
// to disagree about a single word.
type termClaim struct {
	term         string
	keepVerbatim bool
}
