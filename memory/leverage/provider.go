// Package leverage bridges a content memory to the framework's producers.
//
// The framework declares what a producer may ask (core/memory) and owns what it
// does with the answer — the fill policy, provenance stamping, prompt
// assembly. A content memory (memory) owns tiered matching, entity adaptation
// and version chains. This package is the one adapter between them, so the
// single registered `recycle` tool and the single `translate` tool back both
// the kapi CLI and the bowrain platform: there is no second implementation to
// answer differently.
//
// It lives beside memory rather than inside it because the interface is in
// core/, and core/tools already depends on memory (through terms); importing it
// back into memory would be a cycle. That constraint is why the interface is
// declared with its callers — which is also the right shape independently.
package leverage

import (
	"context"
	"math"
	"strings"

	"github.com/neokapi/neokapi/core/edit"
	corememory "github.com/neokapi/neokapi/core/memory"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/memory"
)

// NewTool builds the framework recycle tool backed by a content memory, with
// the canonical fill defaults (fill near-exact matches at or above the fill
// threshold as reviewable drafts; record the rest as alt-translation
// candidates). fuzzyThreshold is the 0-100 lookup floor; a non-positive value
// keeps the tool's default. This is the one constructor both the kapi CLI and
// the bowrain platform build recycle from, so a recycled target is stamped the
// same way everywhere.
func NewTool(tm memory.ContentMemory, source, target model.LocaleID, fuzzyThreshold int) tool.Tool {
	cfg := &tools.MemoryLeverageConfig{}
	cfg.Reset()
	cfg.SourceLocale = source
	cfg.TargetLocale = target
	cfg.Memory = NewProvider(tm)
	if fuzzyThreshold > 0 {
		cfg.FuzzyThreshold = fuzzyThreshold
	}
	return tools.NewMemoryLeverageTool(cfg)
}

// Provider answers a content memory's questions for the framework.
type Provider struct {
	tm memory.ContentMemory
}

// NewProvider wraps a content memory as a producer-facing provider.
func NewProvider(tm memory.ContentMemory) *Provider {
	return &Provider{tm: tm}
}

var _ corememory.Provider = (*Provider)(nil)

// Lookup answers what the corpus has approved for the request's content.
//
// The two request forms reach the corpus differently, and deliberately. A block
// is matched over its full Run sequence, so inline codes participate and the
// answer's runs come back intact: the tiered matcher scores a structurally
// identical entry at 100 and caps a plain-text exact whose codes differ. Text
// is the flattened form, which is how an entry stored without codes can still
// be found.
func (p *Provider) Lookup(ctx context.Context, req corememory.Request) (corememory.Match, bool) {
	opts := memory.LookupOptions{
		MinScore:   float64(req.MinScore) / 100.0,
		MaxResults: 1,
		Point:      req.Point,
	}
	if req.Verbatim {
		// Plain tier only: no entity adaptation, no structural generalization.
		// A verbatim request wants the guarantee that one string does not
		// render two ways, which a best-effort tier cannot give.
		opts.MatchModes = []memory.MatchMode{memory.MatchModePlain}
	}

	var (
		matches []memory.Match
		err     error
	)
	switch {
	case req.Block != nil:
		matches, err = p.tm.Lookup(ctx, req.Block, req.Source, req.Target, opts)
	case req.Text != "":
		matches, err = p.tm.LookupText(ctx, req.Text, req.Source, req.Target, opts)
	default:
		return corememory.Match{}, false
	}
	if err != nil || len(matches) == 0 {
		return corememory.Match{}, false
	}

	m := matches[0]
	out := corememory.Match{
		Score:     int(math.Round(m.Score * 100)),
		Exact:     m.MatchType.IsExact(),
		Ambiguous: m.Ambiguous,
	}

	if req.Block != nil {
		out.TargetRuns = ApplyEntityAdaptations(m.Entry.Variant(req.Target), m.EntityAdaptations)
		// Classified here because this is the only place both sources are in
		// hand: the content being translated, and the content the matched
		// answer was approved for. A caller sees a score and a target, and
		// could never work it out.
		out.Edit = edit.Classify(m.Entry.VariantText(req.Source), model.FlattenRuns(req.Block.Source))
	} else {
		// The flattened path has no source to compare against — the corpus was
		// asked by text and answers with a target — so it cannot classify, and
		// the caller falls back to the score. That is the honest reason this
		// path is the weaker one, and why the block form is preferred.
		target := m.Entry.VariantText(req.Target)
		if target == "" {
			return corememory.Match{}, false
		}
		out.TargetRuns = []model.Run{{Text: &model.TextRun{Text: target}}}
	}
	if len(out.TargetRuns) == 0 {
		return corememory.Match{}, false
	}
	return out, true
}

// PriorVersion answers what a block said before, when the rules it was approved
// under still hold.
//
// A store that cannot answer version queries returns nothing rather than
// erroring: an in-memory corpus seeded for one run has no history worth the
// name, and that is a true statement about the answer rather than a failure.
// Expressed as a return value rather than a missing method, so a caller cannot
// distinguish "this store keeps no chains" from "this block has no history" —
// because it should not behave differently on them.
func (p *Provider) PriorVersion(ctx context.Context, req corememory.VersionRequest) (corememory.Version, bool) {
	vr, versioned := p.tm.(memory.VersionReader)
	if !versioned {
		return corememory.Version{}, false
	}
	src, tgt, ok := PriorVersionFor(ctx, vr, req.Unit, req.Point, req.Source, req.Target, req.GovernedBy)
	if !ok {
		return corememory.Version{}, false
	}
	return corememory.Version{Source: src, Target: tgt}, true
}

// ApplyEntityAdaptations substitutes entity values in a target Run sequence
// based on the adaptations computed during matching, returning a new Run
// sequence; the input runs are not mutated. Text is substituted inside TextRun
// only — Ph/PcOpen/PcClose payloads are passed through unchanged, so inline
// codes and placeholders survive the retargeting. An empty adaptation set (the
// common case: no entity annotations, or a plain match) returns the runs
// unchanged.
func ApplyEntityAdaptations(target []model.Run, adaptations []memory.EntityAdaptation) []model.Run {
	if len(target) == 0 || len(adaptations) == 0 {
		return target
	}
	out := make([]model.Run, len(target))
	copy(out, target)
	for _, adapt := range adaptations {
		for i := range out {
			if out[i].Text == nil {
				continue
			}
			if strings.Contains(out[i].Text.Text, adapt.StoredValue) {
				newRun := *out[i].Text
				newRun.Text = strings.Replace(newRun.Text, adapt.StoredValue, adapt.CurrentValue, 1)
				out[i] = model.Run{Text: &newRun}
				break
			}
		}
	}
	return out
}
