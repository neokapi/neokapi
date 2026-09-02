package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/core/venue"
)

// Measured steerability (model recommendation sweeps): the platform
// periodically measures how much each candidate model actually obeys THIS
// project's standing context — voice profile, term rules, and
// do-not-translate terms — and recommends the model with the highest measured
// context lift. No customer authoring, no customer spend: the fixture set is
// derived from the project's own content, both arms run on the PLATFORM
// provider, and credits are never deducted (see processModelSweepJob).

// ModelSweepItemName is the sentinel ItemName that marks a job row as a model
// recommendation sweep rather than a translation. Like "__forge_ingest__",
// sweep jobs ride the existing translation job table + queue so they inherit
// the claim/lease/retry/sweeper machinery without a second job system.
const ModelSweepItemName = "__model_sweep__"

// NewModelSweepJob builds the durable job for one (project, locale) model
// recommendation sweep. Field reuse on TranslationJob (mirroring the sync-push
// and forge-ingest precedents):
//
//   - ItemName     — the ModelSweepItemName sentinel (routes the worker)
//   - TargetLocale — the target locale this sweep measures
//   - ProjectID    — the project whose brand context is measured
//   - WorkspaceID  — the durable workspace id (scopes brand/terms reads)
//   - WorkspaceSlug— the workspace slug (terms resolution + ai_usage)
//
// The candidate model list and the brand context are re-read at execution
// time, so a config or profile change between enqueue and execution is
// honored rather than replayed stale.
func NewModelSweepJob(workspaceID, workspaceSlug, projectID, locale string) *TranslationJob {
	return &TranslationJob{
		ID:            id.New(),
		WorkspaceID:   workspaceID,
		WorkspaceSlug: workspaceSlug,
		ProjectID:     projectID,
		ItemName:      ModelSweepItemName,
		TargetLocale:  locale,
		Status:        StatusQueued,
	}
}

// IsModelSweep reports whether the job is a model recommendation sweep.
func (j *TranslationJob) IsModelSweep() bool { return j.ItemName == ModelSweepItemName }

// ModelSweepSettings is the platform-config surface a sweep consults: the
// master gate and the candidate model list (the ctrl-onboarded enabled set).
// *platformconfig.Service satisfies it directly.
type ModelSweepSettings interface {
	// ModelSweepsEnabled is the instance-wide gate (default OFF). Both sweep
	// execution and recommendation consumption respect it.
	ModelSweepsEnabled() bool
	// AIEnabledModels is the admin-onboarded candidate model list a sweep
	// measures.
	AIEnabledModels() []string
	// IsModelEnabled guards consumption: a recommendation is only applied while
	// its model is still in the enabled set.
	IsModelEnabled(id string) bool
}

// Fixture derivation --------------------------------------------------------

// SweepFixture is one trap segment in a sweep's fixture set: a source segment
// from the project's own content that genuinely exercises the brand context.
type SweepFixture struct {
	BlockID    string
	SourceText string
	// Trap classifies why the segment was selected: "terms" (contains a
	// terms source term), "voice" (tempts the profile's forbidden/competitor
	// vocabulary or a mechanical style rule), or "dnt" (carries a
	// do-not-translate term).
	Trap string
}

// Fixture-set sizing. The sweep's cost bound is ~maxSweepFixtures segments × 2
// arms × candidate models per (project, locale) — bounded by construction. The
// per-trap quotas keep the set balanced; spare capacity is refilled in trap
// order so a project rich in one trap kind still fills the budget.
const (
	maxSweepFixtures      = 24
	sweepTermsQuota       = 10
	sweepVoiceQuota       = 10
	sweepDNTQuota         = 4
	sweepMaxSourceLen     = 500 // skip very long segments; traps are short by nature
	sweepFixtureDigestVer = "v2"
)

// SweepContext is the standing context one sweep measures against: the same
// voice profile, term rules, and DNT terms the worker binds into every
// translation job (#1334), captured once so both arms and the digest see one
// consistent snapshot.
type SweepContext struct {
	Profile   *coreprofile.VoiceProfile
	TermRules []coreprofile.TermRule // term → mandated target rendering
	DNT       []string               // do-not-translate terms
}

// Empty reports whether there is any context to measure. A project with no
// profile, no term rules, and no DNT terms has nothing steerability can lift.
func (c *SweepContext) Empty() bool {
	return (c.Profile == nil || c.Profile.Name == "" && c.Profile.ID == "") &&
		len(c.TermRules) == 0 && len(c.DNT) == 0
}

// DeriveSweepFixtures selects up to maxSweepFixtures genuine trap segments
// from the project's stored source blocks, deterministically (blocks are
// visited in ID order; the same content + context always yields the same set):
//
//   - terms traps: the source contains a terms store source term, so the
//     mandated rendering is checkable in the target (term-check);
//   - voice traps: the source tempts the profile's forbidden/competitor
//     vocabulary (coreprofile.MatchVocabulary against the source — a model that
//     carries the term over is non-compliant) or matches a mechanical prohibited
//     style pattern;
//   - dnt traps: the source carries a do-not-translate term whose verbatim
//     survival dnt-check verifies.
//
// A block matching several traps is counted once under the first matching
// category. Non-translatable and oversized blocks are skipped.
func DeriveSweepFixtures(blocks []*venue.StoredBlock, sc *SweepContext) []SweepFixture {
	if sc == nil {
		return nil
	}
	// Deterministic visit order.
	sorted := make([]*venue.StoredBlock, 0, len(blocks))
	for _, sb := range blocks {
		if sb == nil || sb.Block == nil || !sb.Block.Translatable {
			continue
		}
		sorted = append(sorted, sb)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Block.ID < sorted[j].Block.ID })

	ruledTerms := make([]string, 0, len(sc.TermRules))
	for _, r := range sc.TermRules {
		ruledTerms = append(ruledTerms, r.Term)
	}
	sort.Strings(ruledTerms)

	classify := func(text string) string {
		switch {
		case containsAnyTerm(text, ruledTerms):
			return "terms"
		case isVoiceTrap(text, sc.Profile):
			return "voice"
		case containsAnyTerm(text, sc.DNT):
			return "dnt"
		}
		return ""
	}

	// Pass 1 — balanced quotas per trap kind.
	quota := map[string]int{"terms": sweepTermsQuota, "voice": sweepVoiceQuota, "dnt": sweepDNTQuota}
	counts := map[string]int{}
	selected := map[string]bool{}
	var fixtures []SweepFixture
	for _, sb := range sorted {
		text := sb.Block.SourceText()
		if strings.TrimSpace(text) == "" || len(text) > sweepMaxSourceLen {
			continue
		}
		trap := classify(text)
		if trap == "" || counts[trap] >= quota[trap] {
			continue
		}
		counts[trap]++
		selected[sb.Block.ID] = true
		fixtures = append(fixtures, SweepFixture{BlockID: sb.Block.ID, SourceText: text, Trap: trap})
		if len(fixtures) >= maxSweepFixtures {
			return fixtures
		}
	}

	// Pass 2 — refill spare capacity from the leftover traps (still in block-ID
	// order), so a project rich in one trap kind fills the whole budget.
	for _, sb := range sorted {
		if len(fixtures) >= maxSweepFixtures {
			break
		}
		if selected[sb.Block.ID] {
			continue
		}
		text := sb.Block.SourceText()
		if strings.TrimSpace(text) == "" || len(text) > sweepMaxSourceLen {
			continue
		}
		trap := classify(text)
		if trap == "" {
			continue
		}
		selected[sb.Block.ID] = true
		fixtures = append(fixtures, SweepFixture{BlockID: sb.Block.ID, SourceText: text, Trap: trap})
	}
	return fixtures
}

// containsAnyTerm reports whether text contains any of the terms as a whole
// word (check.FindTerm — the same Unicode-aware matcher every checker uses).
func containsAnyTerm(text string, terms []string) bool {
	for _, term := range terms {
		if term == "" {
			continue
		}
		if len(check.FindTerm(text, term)) > 0 {
			return true
		}
	}
	return false
}

// isVoiceTrap reports whether the source text tempts the profile's vocabulary
// rules (a forbidden/competitor term present in the source is likely to be
// carried into a target unless steered) or matches a mechanical prohibited
// style pattern.
func isVoiceTrap(text string, profile *coreprofile.VoiceProfile) bool {
	if profile == nil {
		return false
	}
	return len(coreprofile.MatchVocabulary(profile, text)) > 0 || len(coreprofile.MatchPatterns(profile, text)) > 0
}

// SweepFixtureDigest keys sweep results to exactly what was measured: the
// fixture content, the profile identity+version, and the term rules/DNT terms
// derived from the terms store and project settings. Any change to content or
// context yields a new digest, so stale rows are never mistaken for a
// measurement of the current state.
func SweepFixtureDigest(projectID, locale string, fixtures []SweepFixture, sc *SweepContext) string {
	h := sha256.New()
	write := func(parts ...string) {
		for _, p := range parts {
			h.Write([]byte(p))
			h.Write([]byte{0})
		}
	}
	write(sweepFixtureDigestVer, projectID, locale)
	if sc != nil && sc.Profile != nil {
		write("profile", sc.Profile.ID, strconv.Itoa(sc.Profile.Version))
	}
	if sc != nil {
		rules := append([]coreprofile.TermRule(nil), sc.TermRules...)
		sort.Slice(rules, func(i, j int) bool { return rules[i].Term < rules[j].Term })
		for _, r := range rules {
			write("term", r.Term, r.Replacement)
		}
		dnt := append([]string(nil), sc.DNT...)
		sort.Strings(dnt)
		for _, t := range dnt {
			write("dnt", t)
		}
	}
	for _, f := range fixtures {
		write("fixture", f.BlockID, f.SourceText, f.Trap)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Deterministic scoring -----------------------------------------------------

// sweepArmScore is the deterministic score of one arm (one model, with or
// without context) over the fixture set: how many fixtures came back adherent.
type sweepArmScore struct {
	Adherent int
	Total    int
}

// Rate returns the arm's adherence as a fraction of the fixture set (0..1).
func (s sweepArmScore) Rate() float64 {
	if s.Total == 0 {
		return 0
	}
	return float64(s.Adherent) / float64(s.Total)
}

// scoreSweepBlocks scores translated fixture blocks deterministically with the
// REAL core check tools — the same ones the convergence loop's checks pass
// runs (no LLM judge, zero extra AI spend):
//
//   - term-check (core/tools): every ruled source term present in the
//     source must have its mandated rendering in the target — read back from
//     the tool's term-check-passed property;
//   - dnt-check (core/tools): every DNT term present in the source must
//     survive verbatim into the target — read back from the tool's
//     do-not-translate findings on the unified quality.findings annotation;
//   - brand vocabulary: coreprofile.MatchVocabulary over the target plus the
//     profile's compliance bar (coreprofile.CalculateScore ≥ ComplianceBar) — the same
//     zero-AI matcher behind persistDraftVoiceScores and every HTTP scoring
//     surface.
//
// A fixture whose block came back without a target counts as non-adherent: a
// model that fails to produce is the worst possible adherence.
func scoreSweepBlocks(ctx context.Context, blocks []*model.Block, locale model.LocaleID, sc *SweepContext) (sweepArmScore, error) {
	score := sweepArmScore{Total: len(blocks)}
	if len(blocks) == 0 {
		return score, nil
	}

	// Run the real checkers over the translated blocks. Both are read-only
	// Annotate tools; they stamp properties/findings the loop below reads.
	parts := make([]*model.Part, 0, len(blocks))
	for _, b := range blocks {
		parts = append(parts, &model.Part{Type: model.PartBlock, Resource: b})
	}
	if len(sc.TermRules) > 0 {
		termTool := coretools.NewTermCheckTool(&coretools.TermCheckConfig{
			TermRules:    sc.TermRules,
			TargetLocale: locale,
		})
		var err error
		if parts, err = runToolOnParts(ctx, termTool, parts); err != nil {
			return score, fmt.Errorf("term-check: %w", err)
		}
	}
	if len(sc.DNT) > 0 {
		dntTool := coretools.NewDNTCheckTool(&coretools.DNTCheckConfig{
			TargetLocale: locale,
			Terms:        sc.DNT,
		})
		var err error
		if parts, err = runToolOnParts(ctx, dntTool, parts); err != nil {
			return score, fmt.Errorf("dnt-check: %w", err)
		}
	}

	for _, b := range partsToBlocks(parts) {
		if b == nil || !b.HasTarget(locale) {
			continue
		}
		if b.Properties[coretools.PropTermCheckPassed] == "false" {
			continue
		}
		if countDNTFindings(b) > 0 {
			continue
		}
		if !sweepVoiceAdherent(b.TargetText(locale), sc.Profile) {
			continue
		}
		score.Adherent++
	}
	return score, nil
}

// countDNTFindings counts the dnt-check findings the checker recorded on the
// block's unified quality.findings annotation.
func countDNTFindings(b *model.Block) int {
	a, ok := b.Annotations[check.AnnotationKey].(*check.FindingsAnnotation)
	if !ok {
		return 0
	}
	n := 0
	for _, f := range a.Findings {
		if f.Category == "do-not-translate" {
			n++
		}
	}
	return n
}

// sweepVoiceAdherent applies the voice bar: the target's deterministic score —
// vocabulary and prohibited style patterns — must clear the profile's compliant
// bar. A nil profile always passes (there is no voice to violate).
func sweepVoiceAdherent(target string, profile *coreprofile.VoiceProfile) bool {
	if profile == nil {
		return true
	}
	findings := coreprofile.Findings(profile, target, []model.Run{{Text: &model.TextRun{Text: target}}})
	return coreprofile.CalculateScore(findings).Overall >= profile.ComplianceBar()
}

// resolveSweepContext captures the project's standing context for a sweep, via
// the very same assembly the translation worker binds into every AI job
// (#1334): the voicescope profile ladder, the workspace term rules, and the
// project's DNT terms. A sweep measures models under the context the jobs it
// informs will actually run with.
func resolveSweepContext(ctx context.Context, deps *WorkerDeps, job *TranslationJob, proj *store.Project) *SweepContext {
	cfg := jobTranslateConfig(ctx, deps, job, proj)
	return &SweepContext{
		Profile:   cfg.Profile,
		TermRules: cfg.TermRules,
		DNT:       cfg.DNT,
	}
}
