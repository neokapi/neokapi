// Package leverage bridges a content memory to the framework recycle tool.
//
// The recycle tool (core/tools) owns the fill policy, provenance stamping and
// registry integration; a content memory (memory) owns tiered matching and
// entity adaptation. This package is the one adapter between them, so the single
// registered `recycle` tool backs both the kapi CLI and the bowrain platform —
// there is no second recycle implementation to stamp targets differently.
//
// It lives beside memory rather than inside it because the recycle tool's
// provider interfaces are in core/tools, and core/tools already depends on
// memory (through terms); importing it back into memory would be a cycle.
package leverage

import (
	"context"
	"math"
	"strings"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
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
	cfg.Provider = NewProvider(tm)
	if fuzzyThreshold > 0 {
		cfg.FuzzyThreshold = fuzzyThreshold
	}
	return tools.NewMemoryLeverageTool(cfg)
}

// Provider adapts a content memory to the recycle tool's provider interfaces
// (tools.MemoryProvider and tools.BlockMemoryProvider). Structure-aware block
// lookup with entity adaptation is the preferred path; the plain-text
// exact/fuzzy methods are the fallback the tool uses for legacy entries whose
// source was stored without inline codes.
type Provider struct {
	tm memory.ContentMemory
}

// NewProvider wraps a content memory as a recycle-tool provider.
func NewProvider(tm memory.ContentMemory) *Provider {
	return &Provider{tm: tm}
}

var (
	_ tools.MemoryProvider         = (*Provider)(nil)
	_ tools.BlockMemoryProvider    = (*Provider)(nil)
	_ tools.ExactMemoryProvider    = (*Provider)(nil)
	_ aitools.PriorVersionProvider = (*Provider)(nil)
)

// PriorVersion answers what a block said before, for the translate tool.
//
// It is the same adapter role the rest of this type plays for recycle: the
// framework declares the interface, this package supplies the content memory
// behind it. The gate lives inside PriorVersionFor, so a caller cannot obtain
// an ungoverned answer by forgetting to check one.
//
// A store that cannot answer version queries returns nothing rather than
// erroring: an in-memory corpus seeded for one run has no chain, which is a
// true statement about the answer and not a failure.
func (p *Provider) PriorVersion(ctx context.Context, unit, point string, source, target model.LocaleID, fingerprint string) (priorSource, priorTarget string, ok bool) {
	vr, isVersioned := p.tm.(memory.VersionReader)
	if !isVersioned {
		return "", "", false
	}
	return PriorVersionFor(ctx, vr, unit, point, source, target, fingerprint)
}

// LookupExactAt returns a plain-text exact match approved nearest at.
//
// It is LookupExact with the point applied — the same MinScore of 1.0, so the
// answer is the same content or nothing, but resolved against where the fill is
// happening rather than against whichever exact the corpus returned first. The
// block path has always been given the point; this is the plain-text path
// catching up, and it matters for the same reason: an exact answer approved
// somewhere else is still an answer from somewhere else.
func (p *Provider) LookupExactAt(ctx context.Context, source string, sourceLocale, targetLocale model.LocaleID, at string) (string, bool) {
	matches, err := p.tm.LookupText(ctx, source, sourceLocale, targetLocale, memory.LookupOptions{
		MinScore:   1.0,
		MaxResults: 1,
		MatchModes: []memory.MatchMode{memory.MatchModePlain},
		Point:      at,
	})
	if err != nil || len(matches) == 0 {
		return "", false
	}
	return matches[0].Entry.VariantText(targetLocale), true
}

// LookupExact returns a plain-text exact match's target text.
func (p *Provider) LookupExact(ctx context.Context, source string, sourceLocale, targetLocale model.LocaleID) (string, bool) {
	matches, err := p.tm.LookupText(ctx, source, sourceLocale, targetLocale, memory.LookupOptions{
		MinScore:   1.0,
		MaxResults: 1,
		MatchModes: []memory.MatchMode{memory.MatchModePlain},
	})
	if err != nil || len(matches) == 0 {
		return "", false
	}
	return matches[0].Entry.VariantText(targetLocale), true
}

// LookupFuzzy returns the best plain-text match at or above threshold (0-100)
// and its score.
func (p *Provider) LookupFuzzy(ctx context.Context, source string, sourceLocale, targetLocale model.LocaleID, threshold int) (string, int, bool) {
	matches, err := p.tm.LookupText(ctx, source, sourceLocale, targetLocale, memory.LookupOptions{
		MinScore:   float64(threshold) / 100.0,
		MaxResults: 1,
	})
	if err != nil || len(matches) == 0 {
		return "", 0, false
	}
	return matches[0].Entry.VariantText(targetLocale), int(matches[0].Score * 100), true
}

// LookupBlock performs a structure-aware lookup over the block's full source Run
// sequence (inline codes included) via the content memory's tiered matching
// (generalized → structural → plain → fuzzy). A structurally identical entry
// scores 100; a plain-text exact with differing inline codes is capped below
// 100 by the content memory; ambiguous exacts carry the Ambiguous flag so the
// tool records them without filling. Entity adaptations computed during the
// match are applied to the target runs so a stored translation is retargeted to
// the current source's entity values before it is offered as a fill.
//
// at is the context point the fill is happening at. A source the record answers
// more than one way — one wording approved in this collection, another approved
// elsewhere — resolves to the approval nearest here, which is what keeps one
// surface's reviewed wording out of another's.
func (p *Provider) LookupBlock(ctx context.Context, block *model.Block, sourceLocale, targetLocale model.LocaleID, threshold int, at string) (tools.MemoryBlockMatch, bool) {
	matches, err := p.tm.Lookup(ctx, block, sourceLocale, targetLocale, memory.LookupOptions{
		MinScore:   float64(threshold) / 100.0,
		MaxResults: 1,
		Point:      at,
	})
	if err != nil || len(matches) == 0 {
		return tools.MemoryBlockMatch{}, false
	}
	m := matches[0]
	runs := ApplyEntityAdaptations(m.Entry.Variant(targetLocale), m.EntityAdaptations)
	if len(runs) == 0 {
		return tools.MemoryBlockMatch{}, false
	}
	return tools.MemoryBlockMatch{
		TargetRuns: runs,
		Score:      int(math.Round(m.Score * 100)),
		Exact:      m.MatchType.IsExact(),
		Ambiguous:  m.Ambiguous,
		// The classification belongs here because this is the only place both
		// sources are in hand: the block being translated, and the source the
		// matched answer was approved for. The tool sees a score and a target
		// and could never work it out.
		Edit: tools.ClassifyEdit(
			m.Entry.VariantText(sourceLocale),
			model.FlattenRuns(block.Source),
		),
	}, true
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
