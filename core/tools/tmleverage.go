package tools

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// TM leverage property keys stored on Block.Properties.
const (
	PropTMMatchScore = "tm-match-score"
	PropTMMatchType  = "tm-match-type"
	// PropTMSegmentMatches records segment-level leverage as "matched/total"
	// (e.g. "3/5") whenever the block carried a multi-segment source
	// segmentation overlay, even when the block target was not filled.
	PropTMSegmentMatches = "tm-segment-matches"
	// PropTMSegmentAlts is the annotation key under which the per-segment TM
	// matches are stored as one AltTranslations collection (each carrying its
	// SegmentIndex).
	PropTMSegmentAlts = "tm-segment-alts"
	// PropTMAltKey is the annotation key for a whole-block TM match's
	// AltTranslation (matches the convention used by the sievepen TM tool).
	PropTMAltKey = "alt-translation"
)

// TMProvider is the interface for translation memory lookup.
type TMProvider interface {
	// LookupExact looks up an exact match for the source text.
	// Returns the translation and true if found.
	LookupExact(source string, sourceLocale, targetLocale model.LocaleID) (string, bool)

	// LookupFuzzy looks up a fuzzy match for the source text.
	// Returns the translation, match score (0-100), and true if found above threshold.
	LookupFuzzy(source string, sourceLocale, targetLocale model.LocaleID, threshold int) (string, int, bool)
}

// TMBlockMatch is the result of a structure-aware (block-level) TM lookup.
// Unlike the text-based TMProvider results, the translation is carried as
// the matched entry's target Run sequence, so inline codes (icons, paired
// markup) survive the fill instead of being flattened into — and leaking
// as — literal token text.
type TMBlockMatch struct {
	// TargetRuns is the matched entry's target-variant runs.
	TargetRuns []model.Run
	// Score is the match score (0-100). 100 means a structurally exact
	// match (same inline-code structure); a plain-text exact whose code
	// structure differs from the block's is capped below 100 by the TM.
	Score int
	// Exact reports whether the match came from an exact tier (any of
	// generalized / structural / plain), as opposed to fuzzy.
	Exact bool
	// Ambiguous marks an exact match that the TM could not disambiguate:
	// several entries matched at full score with differing targets. An
	// ambiguous match must never be filled unattended — it is recorded as
	// an alt-translation candidate only.
	Ambiguous bool
}

// BlockTMProvider is an optional TMProvider capability for structure-aware
// lookup. When the configured Provider implements it, the tm-leverage tool
// queries the TM with the block's full source Run sequence (inline codes
// included) instead of its flattened text, and fills the target with the
// matched entry's runs. Providers backed by sievepen implement this via
// TranslationMemory.Lookup; the plain-text TMProvider methods remain the
// fallback path.
type BlockTMProvider interface {
	// LookupBlock looks up the best match for the block's source content.
	// threshold is the minimum acceptable score (0-100) for fuzzy matches.
	// Returns false when no match at or above threshold exists.
	LookupBlock(block *model.Block, sourceLocale, targetLocale model.LocaleID, threshold int) (TMBlockMatch, bool)
}

// NullTMProvider is a TMProvider that returns no matches.
// Useful for testing and as a default when no TM is available.
type NullTMProvider struct{}

// LookupExact always returns no match.
func (NullTMProvider) LookupExact(string, model.LocaleID, model.LocaleID) (string, bool) {
	return "", false
}

// LookupFuzzy always returns no match.
func (NullTMProvider) LookupFuzzy(string, model.LocaleID, model.LocaleID, int) (string, int, bool) {
	return "", 0, false
}

// TMLeverageConfig holds configuration for the TM leverage tool.
type TMLeverageConfig struct {
	TargetLocale model.LocaleID `json:"targetLocale,omitempty"   schema:"-"`
	SourceLocale model.LocaleID `json:"sourceLocale,omitempty"   schema:"-"`
	Provider     TMProvider     `json:"-"                        schema:"-"`

	// Schema-visible properties matching the bridge schema.
	FuzzyThreshold                int    `json:"fuzzyThreshold,omitempty"   schema:"title=Fuzzy Match Threshold,description=Minimum score for fuzzy matches (0-100),default=70,min=0,max=100"`
	FillTarget                    bool   `json:"fillTarget,omitempty"       schema:"title=Fill Target with Translation,description=Copy the best translation candidate into the target content,default=true"`
	FillTargetThreshold           int    `json:"fillTargetThreshold,omitempty" schema:"title=Fill Target Threshold,description=Minimum match score required to fill the target,default=95,min=0,max=100"`
	FillIfTargetIsEmpty           bool   `json:"fillIfTargetIsEmpty,omitempty" schema:"title=Only If Target Is Empty,description=Fill the target only when it has no existing content"`
	NoQueryThreshold              int    `json:"noQueryThreshold,omitempty" schema:"title=No-Query Threshold,description=Skip TM query if existing candidate scores at or above this value (101 = always query),default=101,min=0,max=101"`
	MakeTMX                       bool   `json:"makeTmx,omitempty"          schema:"title=Generate TMX Document,description=Create a TMX file with all leveraged matches"`
	TMXPath                       string `json:"tmxPath,omitempty"         schema:"title=TMX Output Path,description=File path for the generated TMX document"`
	DowngradeIdenticalBestMatches bool   `json:"downgradeIdenticalBestMatches,omitempty" schema:"title=Downgrade Identical Exact Matches,description=Reduce score by 1%% when multiple identical exact matches are returned"`
}

// ToolName returns the tool name this config applies to.
func (c *TMLeverageConfig) ToolName() string { return "tm-leverage" }

// Reset restores default values.
func (c *TMLeverageConfig) Reset() {
	c.TargetLocale = ""
	c.SourceLocale = ""
	c.Provider = nil
	c.FuzzyThreshold = 70
	c.FillTarget = true
	c.FillTargetThreshold = 95
	c.FillIfTargetIsEmpty = false
	c.NoQueryThreshold = 101
	c.MakeTMX = false
	c.TMXPath = ""
	c.DowngradeIdenticalBestMatches = false
}

// Validate checks configuration validity.
func (c *TMLeverageConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return errors.New("tm-leverage: TargetLocale is required")
	}
	if c.FuzzyThreshold < 0 || c.FuzzyThreshold > 100 {
		return errors.New("tm-leverage: FuzzyThreshold must be between 0 and 100")
	}
	if c.Provider == nil {
		return errors.New("tm-leverage: Provider is required")
	}
	return nil
}

// TMLeverageSchema returns the auto-generated schema for the TM leverage tool.
func TMLeverageSchema() *schema.ComponentSchema {
	cfg := &TMLeverageConfig{}
	cfg.Reset()
	return schema.FromStruct(cfg, schema.ToolMeta{
		ID:          "tm-leverage",
		Category:    schema.CategoryTranslation,
		DisplayName: "TM Leverage",
		Description: "Pre-fill translations from translation memory",
		Requires:    []string{schema.RequiresTargetLanguage, schema.RequiresSourceLanguage, schema.RequiresTM},
	})
}

// NewTMLeverageFromConfig creates a TM leverage tool from a config map.
func NewTMLeverageFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	cfg := &TMLeverageConfig{}
	cfg.Reset()
	if err := schema.ApplyConfig(config, cfg); err != nil {
		return nil, fmt.Errorf("tm-leverage config: %w", err)
	}
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	if cfg.FuzzyThreshold == 0 {
		cfg.FuzzyThreshold = 70
	}
	cfg.Provider = NullTMProvider{}
	return NewTMLeverageTool(cfg), nil
}

// NewTMLeverageTool creates a TM leveraging tool that pre-fills translations
// from a translation memory. It first attempts exact matches, then falls back
// to fuzzy matching if a threshold is configured.
func NewTMLeverageTool(cfg *TMLeverageConfig) *tool.BaseTool {
	if cfg.FuzzyThreshold == 0 {
		cfg.FuzzyThreshold = 70
	}
	// Default FillTarget to true if not explicitly configured (backward compat).
	if !cfg.FillTarget && cfg.FillTargetThreshold == 0 {
		cfg.FillTarget = true
		cfg.FillTargetThreshold = 0 // 0 means accept any score
	}

	t := &tool.BaseTool{
		ToolName:        "tm-leverage",
		ToolDescription: "Pre-fills translations from translation memory using exact and fuzzy matching",
		Cfg:             cfg,
	}
	// Translate: tm-leverage writes a target translation from TM; source is read-only.
	t.Translate = func(v tool.TargetView) error {
		if !v.Translatable() {
			return nil
		}

		conf := t.Cfg.(*TMLeverageConfig)
		if conf.Provider == nil {
			return nil
		}

		sourceText := v.SourceText()
		if sourceText == "" {
			return nil
		}

		// Check no-query threshold: skip TM query if an existing match scores at/above.
		if conf.NoQueryThreshold <= 101 {
			if existing, ok := v.Annotations()[string(model.AnnoTMMatch)].(*TMMatchAnnotation); ok && existing.Score >= conf.NoQueryThreshold {
				return nil
			}
		}

		// Segment-aware path: when the block carries a multi-segment source
		// segmentation overlay, leverage the TM sentence by sentence (TM stores
		// segment pairs) and assemble the block target from the per-segment
		// translations. This recovers leverage for multi-sentence (prose) blocks
		// that would never match the sentence-keyed TM as one unit. Handled and
		// filled here only when every segment matches; otherwise it records
		// partial leverage and falls through so a later stage translates the
		// whole block. Single-segment blocks (most software-localization
		// strings) skip this and use the whole-block path below unchanged.
		if leverageSegments(conf, v) {
			return nil
		}

		// Structure-aware path: when the provider can match on the block's
		// full Run sequence (inline codes included), prefer it — a block
		// whose source carries icon/markup runs only scores 100 against an
		// entry with the same code structure, and the fill preserves the
		// entry's runs (tokens stay model objects, never literal text).
		if bp, ok := conf.Provider.(BlockTMProvider); ok {
			if leverageBlockRuns(conf, v, bp) {
				return nil
			}
		}

		// Try exact match first.
		if translation, found := conf.Provider.LookupExact(sourceText, conf.SourceLocale, conf.TargetLocale); found {
			score := 100
			if conf.DowngradeIdenticalBestMatches {
				score = 99
			}
			recordWholeBlockMatch(v, conf, translation, score, model.MatchExact, "exact")
			return nil
		}

		// Try fuzzy match.
		if translation, score, found := conf.Provider.LookupFuzzy(sourceText, conf.SourceLocale, conf.TargetLocale, conf.FuzzyThreshold); found {
			recordWholeBlockMatch(v, conf, translation, score, model.MatchFuzzy, "fuzzy")
			return nil
		}

		return nil
	}
	return t
}

// recordWholeBlockMatch records a whole-block TM hit the same auditable way as
// the segment path: the match is attached as an AltTranslation annotation
// (source/target runs, score, match type, provenance) regardless of fill, and
// when the block target is filled it is committed as a real Target carrying
// `tm` provenance, the score, and draft status — not an opaque string. The
// summary properties remain for quick gating and backward compatibility.
func recordWholeBlockMatch(v tool.TargetView, conf *TMLeverageConfig, translation string, score int, mt model.MatchType, propType string) {
	targetRuns := []model.Run{{Text: &model.TextRun{Text: translation}}}
	v.AddAltTranslation(&model.AltTranslation{
		Source:    v.SourceRuns(),
		Target:    targetRuns,
		Locale:    conf.TargetLocale,
		Origin:    "tm",
		Score:     float64(score) / 100,
		MatchType: mt,
		ToolID:    "tm-leverage",
	})
	if shouldFillTarget(conf, v, score) {
		v.SetTarget(conf.TargetLocale, &model.Target{
			Runs:   targetRuns,
			Status: model.TargetStatusDraft,
			Origin: model.Origin{Kind: "tm", Tool: "tm-leverage"},
			Score:  float64(score) / 100,
		})
	}
	v.Annotate(string(model.AnnoTMMatch), &TMMatchAnnotation{Score: score, Type: propType})
}

// leverageBlockRuns performs the structure-aware whole-block leverage. It
// returns true when it has handled the block — the match was filled, or
// it was Ambiguous (recorded but deliberately not filled) — so the caller
// must not run the text-based path on top of it. It returns false when
// the provider has no block-level match, when the matched target's inline
// codes cannot be mapped onto the block's source codes, or when the match
// scored below the fill policy: the flattened text path keys differently
// (inline codes and placeholders drop out of its query), so it can still
// recover legacy entries whose source text was authored without them. A
// sub-threshold block match is recorded as an alt-translation candidate
// before falling through.
//
// Fill semantics differ from the text path in two ways:
//   - the target is written as the matched entry's Run sequence, so paired
//     codes and placeholders survive as model objects;
//   - an Ambiguous match (several full-score exacts with differing
//     targets) is recorded as an alt-translation candidate but never
//     filled, regardless of fillTargetThreshold — unattended leverage must
//     not turn an arbitrary pick into published content. The text path is
//     skipped too: it would resolve the same tie by arbitrary pick.
func leverageBlockRuns(conf *TMLeverageConfig, v tool.TargetView, bp BlockTMProvider) bool {
	block := &model.Block{
		ID:           v.ID(),
		Translatable: true,
		SourceLocale: v.SourceLocale(),
		Source:       v.SourceRuns(),
	}
	for key, payload := range v.Annotations() {
		block.SetAnno(key, payload)
	}

	m, found := bp.LookupBlock(block, conf.SourceLocale, conf.TargetLocale, conf.FuzzyThreshold)
	if !found || len(m.TargetRuns) == 0 {
		return false
	}
	if !targetCodesCompatible(v.SourceRuns(), m.TargetRuns) {
		// The entry's target carries inline codes the block's source does
		// not have — filling its runs would inject foreign markup. Leave
		// the block to the text path (which flattens codes away).
		return false
	}

	matchType, propType := model.MatchFuzzy, "fuzzy"
	if m.Exact {
		matchType, propType = model.MatchExact, "exact"
	}
	targetRuns := cloneRuns(m.TargetRuns)
	v.AddAltTranslation(&model.AltTranslation{
		Source:    v.SourceRuns(),
		Target:    targetRuns,
		Locale:    conf.TargetLocale,
		Origin:    "tm",
		Score:     float64(m.Score) / 100,
		MatchType: matchType,
		ToolID:    "tm-leverage",
	})
	if m.Ambiguous {
		// Recorded as a candidate only. Handled: the text path would
		// resolve the same tie by arbitrary pick.
		v.Annotate(string(model.AnnoTMMatch), &TMMatchAnnotation{Score: m.Score, Type: propType})
		return true
	}
	if !shouldFillTarget(conf, v, m.Score) {
		// Below the fill policy: keep the candidate recorded, but let the
		// text path try its differently-keyed lookup (it may overwrite the
		// tm-match annotation with a better match).
		v.Annotate(string(model.AnnoTMMatch), &TMMatchAnnotation{Score: m.Score, Type: propType})
		return false
	}
	v.SetTarget(conf.TargetLocale, &model.Target{
		Runs:   targetRuns,
		Status: model.TargetStatusDraft,
		Origin: model.Origin{Kind: "tm", Tool: "tm-leverage"},
		Score:  float64(m.Score) / 100,
	})
	v.Annotate(string(model.AnnoTMMatch), &TMMatchAnnotation{Score: m.Score, Type: propType})
	return true
}

// targetCodesCompatible reports whether every inline code in the candidate
// target runs has a counterpart in the source runs, so splicing the target
// into the block cannot introduce markup the source does not carry. Paired
// codes match by ID, placeholders by Equiv (falling back to ID), subblock
// references by ID. Text / plural / select runs are not constrained.
func targetCodesCompatible(source, target []model.Run) bool {
	avail := map[string]int{}
	for _, r := range source {
		if sig, ok := runCodeSignature(r); ok {
			avail[sig]++
		}
	}
	for _, r := range target {
		sig, ok := runCodeSignature(r)
		if !ok {
			continue
		}
		if avail[sig] == 0 {
			return false
		}
		avail[sig]--
	}
	return true
}

// runCodeSignature returns a comparable identity for an inline-code run,
// or ok=false for non-code runs (text, plural, select).
func runCodeSignature(r model.Run) (string, bool) {
	switch {
	case r.PcOpen != nil:
		return "pc-open:" + r.PcOpen.ID, true
	case r.PcClose != nil:
		return "pc-close:" + r.PcClose.ID, true
	case r.Ph != nil:
		if r.Ph.Equiv != "" {
			return "ph:" + r.Ph.Equiv, true
		}
		return "ph:" + r.Ph.ID, true
	case r.Sub != nil:
		return "sub:" + r.Sub.ID, true
	}
	return "", false
}

// cloneRuns deep-copies a Run sequence so a TM entry's stored runs are
// never aliased into block targets (an in-memory TM hands out its own
// slices; a later edit to one filled block must not rewrite the entry or
// another block).
func cloneRuns(runs []model.Run) []model.Run {
	if runs == nil {
		return nil
	}
	out := make([]model.Run, len(runs))
	for i, r := range runs {
		out[i] = cloneRun(r)
	}
	return out
}

func cloneRun(r model.Run) model.Run {
	var c model.Run
	switch {
	case r.Text != nil:
		t := *r.Text
		c.Text = &t
	case r.Ph != nil:
		p := *r.Ph
		if p.Constraints != nil {
			cc := *p.Constraints
			p.Constraints = &cc
		}
		c.Ph = &p
	case r.PcOpen != nil:
		p := *r.PcOpen
		if p.Constraints != nil {
			cc := *p.Constraints
			p.Constraints = &cc
		}
		c.PcOpen = &p
	case r.PcClose != nil:
		p := *r.PcClose
		c.PcClose = &p
	case r.Sub != nil:
		s := *r.Sub
		c.Sub = &s
	case r.Plural != nil:
		p := model.PluralRun{Pivot: r.Plural.Pivot}
		if r.Plural.Forms != nil {
			p.Forms = make(map[model.PluralForm][]model.Run, len(r.Plural.Forms))
			for k, form := range r.Plural.Forms {
				p.Forms[k] = cloneRuns(form)
			}
		}
		c.Plural = &p
	case r.Select != nil:
		s := model.SelectRun{Pivot: r.Select.Pivot}
		if r.Select.Cases != nil {
			s.Cases = make(map[string][]model.Run, len(r.Select.Cases))
			for k, cs := range r.Select.Cases {
				s.Cases[k] = cloneRuns(cs)
			}
		}
		c.Select = &s
	}
	return c
}

// leverageSegments attempts sentence-level TM leverage over a multi-segment
// block. It returns true when it has fully handled the block (every segment
// matched at/above the fuzzy threshold and, if permitted, the assembled target
// was written), so the caller should stop. It returns false — leaving the block
// for whole-block leverage and any later translation stage — when the block has
// no usable multi-segment overlay or when at least one segment did not match.
//
// Assembly preserves inter-segment text by requiring the segment spans to be
// contiguous (their concatenation reproduces the source); when they are not
// (gaps the overlay does not cover), it records partial leverage but does not
// fill, so nothing is silently dropped.
func leverageSegments(conf *TMLeverageConfig, v tool.TargetView) bool {
	if v.SourceSegmentation() == nil {
		return false
	}
	n := v.SourceSegmentCount()
	if n < 2 {
		return false
	}

	segTexts := make([]string, n)
	translations := make([]string, n)
	minScore := 101
	matched := 0
	allExact := true
	for seg := range v.SourceUnits(model.LayerPrimary) {
		i := seg.Index()
		segRuns := seg.SourceRuns()
		segTexts[i] = model.RunsText(segRuns)
		if segTexts[i] == "" {
			// An empty segment (e.g. a span of only ignorable runs) is treated
			// as a trivial match so it never blocks assembly; nothing to record.
			translations[i] = segTexts[i]
			matched++
			continue
		}
		if tr, found := conf.Provider.LookupExact(segTexts[i], conf.SourceLocale, conf.TargetLocale); found {
			translations[i] = tr
			matched++
			score := 100
			if conf.DowngradeIdenticalBestMatches {
				score = 99
				allExact = false
			}
			if score < minScore {
				minScore = score
			}
			annotateSegmentMatch(v, conf, i, segRuns, tr, score, model.MatchExact)
			continue
		}
		if tr, score, found := conf.Provider.LookupFuzzy(segTexts[i], conf.SourceLocale, conf.TargetLocale, conf.FuzzyThreshold); found {
			translations[i] = tr
			matched++
			allExact = false
			if score < minScore {
				minScore = score
			}
			annotateSegmentMatch(v, conf, i, segRuns, tr, score, model.MatchFuzzy)
			continue
		}
		allExact = false
	}

	// Always record segment-level leverage for visibility (editor, AI, reports),
	// even when the block target is not filled. The per-segment matches above are
	// recorded as alt-translation annotations regardless of fill, so partial
	// leverage is never lost.
	segMatches := strconv.Itoa(matched) + "/" + strconv.Itoa(n)
	v.Annotate(string(model.AnnoTMMatch), &TMMatchAnnotation{SegmentMatches: segMatches})

	if matched < n {
		// Partial leverage: leave the block for a later whole-block translation
		// stage. We deliberately do not write a half-translated target here;
		// per-segment fill (mixing TM and MT within one block) needs
		// segment-aligned target storage and is a separate change. The matched
		// segments survive as alt-translation annotations for that stage / the
		// editor to consume.
		return false
	}

	// Every segment matched. Assemble only when the spans are contiguous, so the
	// concatenated translations reproduce a faithful whole-block target.
	if strings.Join(segTexts, "") != v.SourceText() {
		return false
	}

	if minScore == 101 {
		minScore = 100 // all segments were empty/trivial
	}
	matchType := "segmented-fuzzy"
	if allExact {
		matchType = "segmented-exact"
	}
	if shouldFillTarget(conf, v, minScore) {
		// Commit a real Target carrying provenance and score, not an opaque
		// string: a TM pre-fill is a reviewable draft assembled from segment
		// matches, so a reviewer/tool can see it came from TM and at what score.
		v.SetTarget(conf.TargetLocale, &model.Target{
			Runs:   []model.Run{{Text: &model.TextRun{Text: strings.Join(translations, "")}}},
			Status: model.TargetStatusDraft,
			Origin: model.Origin{Kind: "tm", Tool: "tm-leverage"},
			Score:  float64(minScore) / 100,
		})
	}
	v.Annotate(string(model.AnnoTMMatch), &TMMatchAnnotation{Score: minScore, Type: matchType, SegmentMatches: segMatches})
	return true
}

// annotateSegmentMatch records a per-segment TM hit as an AltTranslation
// annotation anchored by its source runs and segment index, so the leverage is
// auditable and partial matches survive even when the block target is not
// filled. The annotation key carries the segment index; the annotation itself
// carries the matched source/target, score (0-1), match type, and provenance.
func annotateSegmentMatch(v tool.TargetView, conf *TMLeverageConfig, idx int, srcRuns []model.Run, translation string, score int, mt model.MatchType) {
	v.AppendAltUnder(PropTMSegmentAlts, &model.AltTranslation{
		Source:       srcRuns,
		Target:       []model.Run{{Text: &model.TextRun{Text: translation}}},
		Locale:       conf.TargetLocale,
		Origin:       "tm",
		Score:        float64(score) / 100,
		MatchType:    mt,
		ToolID:       "tm-leverage",
		SegmentIndex: idx,
	})
}

// shouldFillTarget decides whether to copy the translation into the target based on config.
func shouldFillTarget(conf *TMLeverageConfig, v tool.TargetView, score int) bool {
	if !conf.FillTarget {
		return false
	}
	if score < conf.FillTargetThreshold {
		return false
	}
	if conf.FillIfTargetIsEmpty {
		// Only fill if target is empty.
		if v.HasTarget(conf.TargetLocale) && v.TargetText(conf.TargetLocale) != "" {
			return false
		}
	}
	return true
}
