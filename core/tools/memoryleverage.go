package tools

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/edit"
	corememory "github.com/neokapi/neokapi/core/memory"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// content-memory leverage property keys stored on Block.Properties.
const (
	PropMemoryMatchScore = "tm-match-score"
	PropMemoryMatchType  = "tm-match-type"
	// PropMemorySegmentMatches records segment-level leverage as "matched/total"
	// (e.g. "3/5") whenever the block carried a multi-segment source
	// segmentation overlay, even when the block target was not filled.
	PropMemorySegmentMatches = "tm-segment-matches"
	// PropMemorySegmentAlts is the annotation key under which the per-segment content memory
	// matches are stored as one AltTranslations collection (each carrying its
	// SegmentIndex).
	PropMemorySegmentAlts = "tm-segment-alts"
	// PropMemoryAltKey is the annotation key for a whole-block content-memory match's
	// AltTranslation (matches the convention used by the memory content memory tool).
	PropMemoryAltKey = "alt-translation"
)

// MemoryLeverageConfig holds configuration for the content-memory leverage tool.
type MemoryLeverageConfig struct {
	TargetLocale model.LocaleID      `json:"targetLocale,omitempty"   schema:"-"`
	SourceLocale model.LocaleID      `json:"sourceLocale,omitempty"   schema:"-"`
	Memory       corememory.Provider `json:"-"                   schema:"-"`

	// Schema-visible properties matching the bridge schema.
	FuzzyThreshold                int    `json:"fuzzyThreshold,omitempty"   schema:"title=Fuzzy Match Threshold,description=Minimum score for a match; the fuzzy path below 100 is retiring,default=70,min=0,max=100"`
	FillTarget                    bool   `json:"fillTarget,omitempty"       schema:"title=Fill Target with Translation,description=Copy the best translation candidate into the target content,default=true"`
	FillTargetThreshold           int    `json:"fillTargetThreshold,omitempty" schema:"title=Fill Target Threshold,description=Minimum match score required to fill the target (100 = exact only),default=95,min=0,max=100"`
	FillIfTargetIsEmpty           bool   `json:"fillIfTargetIsEmpty,omitempty" schema:"title=Only If Target Is Empty,description=Fill the target only when it has no existing content"`
	NoQueryThreshold              int    `json:"noQueryThreshold,omitempty" schema:"title=No-Query Threshold,description=Skip content-memory query if existing candidate scores at or above this value (101 = always query),default=101,min=0,max=101"`
	MakeTMX                       bool   `json:"makeTmx,omitempty"          schema:"title=Generate TMX Document,description=Create a TMX file with all leveraged matches"`
	TMXPath                       string `json:"tmxPath,omitempty"         schema:"title=TMX Output Path,description=File path for the generated TMX document"`
	DowngradeIdenticalBestMatches bool   `json:"downgradeIdenticalBestMatches,omitempty" schema:"title=Downgrade Identical Exact Matches,description=Reduce score by 1%% when multiple identical exact matches are returned"`

	// Point is where this run's content sits in the project's context space,
	// injected by the flow's bindings. It is a location, not governance: it
	// never enters the context fingerprint, and it is passed to the content
	// memory so a source with more than one approved answer resolves to the
	// approval nearest here rather than to the one the corpus repeats most.
	Point string `json:"point,omitempty" schema:"-"`

	// Profile and TermRules carry the context governing the collection, injected
	// by the flow's bindings. Recycling consults neither for matching, but a
	// filled target is stamped with them so a recycled target is as attributable
	// as a freshly translated one. See recycleOrigin for the fill-time decision.
	Profile   *coreprofile.VoiceProfile `json:"-" schema:"-"`
	TermRules []coreprofile.TermRule    `json:"term_rules,omitempty" schema:"-"`

	// Governing context resolved once at construction from Profile + TermRules
	// and stamped onto every filled target's Origin.
	profileID      string
	profileVersion string
	contextFP      string
}

// recycleOrigin describes a target filled from content memory: how it was made
// (this tool) and — resolved at fill time from the context governing the
// collection now, not inherited from the matched entry — what governed it.
//
// The fingerprint is the CURRENT context's, deliberately. A recycled fill
// asserts the matched entry as a valid target under the governance in force now,
// so it must fall stale when that governance moves; carrying the entry's
// original approving fingerprint would make the recycled target immune to drift
// against the profile that actually governs it. That fingerprint is also not
// available here — a memory lookup returns a source/target/score, not the Origin
// of the decision that first approved the entry — so resolving at fill time is
// both the correct semantics and the only reachable one.
func (c *MemoryLeverageConfig) recycleOrigin() model.Origin {
	return model.Origin{
		Kind:               model.OriginMemory,
		Tool:               "recycle",
		Profile:            c.profileID,
		ProfileVersion:     c.profileVersion,
		ContextFingerprint: c.contextFP,
	}
}

// ToolName returns the tool name this config applies to.
func (c *MemoryLeverageConfig) ToolName() string { return "recycle" }

// Reset restores default values.
func (c *MemoryLeverageConfig) Reset() {
	c.TargetLocale = ""
	c.SourceLocale = ""
	c.Memory = nil
	// 70/95, not 100/100 — for now.
	//
	// Fuzzy fill is retiring and its replacement is better, but the replacement
	// is not connected yet: nothing fills prompt.Context.Prior in a live run,
	// because the translate tool has no point plumbed through its flow
	// bindings. Flipping these first would drop a real behaviour for nothing.
	//
	// Measured rather than assumed. At realistic sentence length the cosmetic
	// edits an author actually makes — a trailing period, an added comma, a
	// capitalised word, a hyphen becoming a dash — score 96.7 to 98.3, inside
	// the band 95 catches and 100 does not. The 70-94 band below is the inert
	// one: recorded as candidates and read by nothing. The edit ladder in
	// scripts/coordinatereport measures this on every run.
	//
	// Flip both to 100 in the change that wires the reference, not before.
	c.FuzzyThreshold = 70
	c.FillTarget = true
	c.FillTargetThreshold = 95
	c.FillIfTargetIsEmpty = false
	c.NoQueryThreshold = 101
	c.MakeTMX = false
	c.TMXPath = ""
	c.DowngradeIdenticalBestMatches = false
	c.Point = ""
	c.Profile = nil
	c.TermRules = nil
	c.profileID = ""
	c.profileVersion = ""
	c.contextFP = ""
}

// Validate checks configuration validity.
func (c *MemoryLeverageConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return errors.New("recycle: TargetLocale is required")
	}
	if c.FuzzyThreshold < 0 || c.FuzzyThreshold > 100 {
		return errors.New("recycle: FuzzyThreshold must be between 0 and 100")
	}
	if c.Memory == nil {
		return errors.New("recycle: Memory is required")
	}
	return nil
}

// NewMemoryLeverageFromConfig creates a content-memory leverage tool from a config map.
func NewMemoryLeverageFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	cfg := &MemoryLeverageConfig{}
	cfg.Reset()
	// The voice profile is injected by the flow bindings as a live pointer, not a
	// serializable value, so it is lifted out before ApplyConfig's JSON round-trip.
	// The term rules bind directly.
	// Lifted before ApplyConfig's JSON round trip, like the voice profile below:
	// both are live values rather than data, so neither survives it.
	if m, ok := config[corememory.ConfigKey].(corememory.Provider); ok {
		cfg.Memory = m
		delete(config, corememory.ConfigKey)
	}
	// SourceLocale is schema:"-" so ApplyConfig will not populate it, and a
	// lookup with an empty source locale matches nothing (WHERE locale = ?).
	// The host knows it; this is where it arrives.
	if src, ok := config[corememory.SourceLocaleKey].(string); ok && src != "" {
		cfg.SourceLocale = model.LocaleID(src)
		delete(config, corememory.SourceLocaleKey)
	}

	var profile *coreprofile.VoiceProfile
	if pf, ok := config["profile"].(*coreprofile.VoiceProfile); ok {
		profile = pf
		delete(config, "profile")
	}
	if err := schema.ApplyConfig(config, cfg); err != nil {
		return nil, fmt.Errorf("recycle config: %w", err)
	}
	cfg.Profile = profile
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	if cfg.FuzzyThreshold == 0 {
		cfg.FuzzyThreshold = 70
	}
	if cfg.Memory == nil {
		// A run with no content memory. The null provider answers every
		// question with "no", so the tool needs no special case for it.
		cfg.Memory = corememory.NullProvider{}
	}
	return NewMemoryLeverageTool(cfg), nil
}

// NewMemoryLeverageTool creates a content memory leveraging tool that pre-fills translations
// from a content memory. It first attempts exact matches, then falls back
// to fuzzy matching if a threshold is configured.
func NewMemoryLeverageTool(cfg *MemoryLeverageConfig) *tool.BaseTool {
	if cfg.FuzzyThreshold == 0 {
		cfg.FuzzyThreshold = 70
	}
	// Default FillTarget to true if not explicitly configured (backward compat).
	if !cfg.FillTarget && cfg.FillTargetThreshold == 0 {
		cfg.FillTarget = true
		cfg.FillTargetThreshold = 0 // 0 means accept any score
	}
	cfg.profileID, cfg.profileVersion, cfg.contextFP = coreprofile.GovernanceContext(cfg.Profile, cfg.TermRules)

	t := &tool.BaseTool{
		ToolName:        "recycle",
		ToolDescription: "Pre-fills translations from content memory using exact and fuzzy matching",
		Cfg:             cfg,
	}
	// Translate: recycle writes a target translation from content memory; source is read-only.
	t.Produce = func(v tool.VariantView) error {
		if !v.Translatable() {
			return nil
		}

		// Source-gate hold (epic 019): skip a block whose source ranks below the
		// active source gate — the leading source-gate stage marked it, and an
		// un-settled source must not be recycled into a target either. No-op when
		// the gate is off or the source cleared (marker absent).
		if v.Property(model.PropSourceHeld) == "1" {
			return nil
		}

		conf := t.Cfg.(*MemoryLeverageConfig)
		if conf.Memory == nil {
			return nil
		}

		sourceText := v.SourceText()
		if sourceText == "" {
			return nil
		}

		// Check no-query threshold: skip content memory query if an existing match scores at/above.
		if conf.NoQueryThreshold <= 101 {
			if existing, ok := v.Annotations()[string(model.AnnoMemoryMatch)].(*MemoryMatchAnnotation); ok && existing.Score >= conf.NoQueryThreshold {
				return nil
			}
		}

		// Segment-aware path: when the block carries a multi-segment source
		// segmentation overlay, leverage the content memory sentence by sentence (content-memory stores
		// segment pairs) and assemble the block target from the per-segment
		// translations. This recovers leverage for multi-sentence (prose) blocks
		// that would never match the sentence-keyed content memory as one unit. Handled and
		// filled here only when every segment matches; otherwise it records
		// partial leverage and falls through so a later stage translates the
		// whole block. Single-segment blocks (most software-localization
		// strings) skip this and use the whole-block path below unchanged.
		if leverageSegments(conf, v) {
			return nil
		}

		// Structure-aware path first: a block form carries the source's inline
		// codes, so a block whose source has icon or markup runs only scores
		// 100 against an entry with the same code structure, the fill preserves
		// the entry's runs (tokens stay model objects, never literal text), and
		// the answer can be classified against the source it was approved for.
		if leverageBlock(conf, v) {
			return nil
		}

		// Flattened fallback, for entries stored without inline codes. It keys
		// differently, so it can reach what the block form cannot — and it
		// cannot classify, so it fills on the score alone.
		leverageText(conf, v, sourceText)
		return nil
	}
	return t
}

// recordWholeBlockMatch records a whole-block content-memory hit the same auditable way as
// the segment path: the match is attached as an AltTranslation annotation
// (source/target runs, score, match type, provenance) regardless of fill, and
// when the block target is filled it is committed as a real Target carrying
// `tm` provenance, the score, and draft status — not an opaque string. The
// summary properties remain for quick gating and backward compatibility.
//
// The text path keys on the *flattened* source (inline codes contribute no
// characters), so a code-bearing block can match a legacy entry that was stored
// without codes. Its translation is a bare text run, so filling it would drop
// every placeholder the source carries. Such a match is recorded as a candidate
// for review but never filled — see fillWouldDropCodes.
func recordWholeBlockMatch(v tool.VariantView, conf *MemoryLeverageConfig, translation string, score int, mt model.MatchType, propType string) {
	targetRuns := []model.Run{{Text: &model.TextRun{Text: translation}}}
	v.AddAltTranslation(&model.AltTranslation{
		Source:    v.SourceRuns(),
		Target:    targetRuns,
		Locale:    conf.TargetLocale,
		Origin:    "tm",
		Score:     float64(score) / 100,
		MatchType: mt,
		ToolID:    "recycle",
	})
	// The plain-text path has no matched source in hand — LookupExact and
	// LookupFuzzy return a translation and nothing to compare against — so it
	// cannot classify and falls back to the score. It is the legacy path for
	// entries stored without inline codes; the structure-aware path above is
	// the one that classifies.
	if shouldFillTarget(conf, v, score, "") && !fillWouldDropCodes(v, targetRuns) {
		v.SetTarget(conf.TargetLocale, &model.Target{
			Runs:   targetRuns,
			Status: model.TargetStatusDraft,
			Origin: conf.recycleOrigin(),
			Score:  float64(score) / 100,
		})
	}
	v.Annotate(string(model.AnnoMemoryMatch), &MemoryMatchAnnotation{Score: score, Type: propType})
}

// fillWouldDropCodes reports whether committing candidate as v's target would
// lose inline codes the source carries — a dropped `{count}`, `{name}` or icon
// placeholder.
//
// A recycled target whose placeholder set does not match its source's is not a
// translation, it is content loss: the string renders with a hole where a
// number or a filename belongs, and nothing downstream signals it. Leaving the
// block untranslated (so the locale falls back to source) is the honest
// outcome, so every leverage fill is gated on this.
func fillWouldDropCodes(v tool.VariantView, candidate []model.Run) bool {
	return model.DiffRunCodes(v.SourceRuns(), candidate).Lossy()
}

// leverageBlockRuns performs the structure-aware whole-block leverage. It
// returns true when it has handled the block — the match was filled, or
// it was Ambiguous (recorded but deliberately not filled) — so the caller
// must not run the text-based path on top of it. It returns false when
// the provider has no block-level match, when the matched target's inline
// codes do not line up with the block's source codes, or when the match
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
//
// Every candidate is recorded as an alt-translation before the fill decision,
// so a rejected match is visible to a reviewer instead of vanishing.
func leverageBlock(conf *MemoryLeverageConfig, v tool.VariantView) bool {
	block := &model.Block{
		ID:           v.ID(),
		Translatable: true,
		SourceLocale: v.SourceLocale(),
		Source:       v.SourceRuns(),
	}
	for key, payload := range v.Annotations() {
		block.SetAnno(key, payload)
	}

	m, found := conf.Memory.Lookup(v.Context(), corememory.Request{
		Block:    block,
		Source:   conf.SourceLocale,
		Target:   conf.TargetLocale,
		Point:    conf.Point,
		MinScore: conf.FuzzyThreshold,
	})
	if !found || len(m.TargetRuns) == 0 {
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
		ToolID:    "recycle",
	})
	if !model.DiffRunCodes(v.SourceRuns(), targetRuns).Balanced() {
		// The entry's inline codes do not line up with the block's source.
		// Either direction disqualifies the fill: extra codes would inject
		// foreign markup, missing codes would silently drop a placeholder the
		// reader needs (the plain tier matches on flattened text, so an entry
		// stored without codes matches a code-bearing source at near-exact
		// score). The candidate stays recorded above so a reviewer can repair
		// it; the block itself falls through to the text path, which keys
		// differently and may recover a legacy entry — under the same guard.
		v.Annotate(string(model.AnnoMemoryMatch), &MemoryMatchAnnotation{Score: m.Score, Type: propType})
		return false
	}
	if m.Ambiguous {
		// Recorded as a candidate only. Handled: the text path would
		// resolve the same tie by arbitrary pick.
		v.Annotate(string(model.AnnoMemoryMatch), &MemoryMatchAnnotation{Score: m.Score, Type: propType})
		return true
	}
	if !shouldFillTarget(conf, v, m.Score, m.Edit) {
		// Below the fill policy: keep the candidate recorded, but let the
		// text path try its differently-keyed lookup (it may overwrite the
		// tm-match annotation with a better match).
		v.Annotate(string(model.AnnoMemoryMatch), &MemoryMatchAnnotation{Score: m.Score, Type: propType})
		// A substantive edit is a refusal, and a refusal is final. The text
		// path asks the same corpus, keyed differently, and gets back a
		// translation with no source to compare against — so it fills on the
		// score alone, which is the number that just said yes to a meaning
		// inversion. Returning false here would refuse at the front door and
		// let the same answer in through the back one, at the same score.
		//
		// Nothing better is lost: a block path that returned a substantive
		// match returned the corpus's BEST match, so there is no exact for the
		// text path to find.
		return m.Edit == edit.Substantive
	}
	v.SetTarget(conf.TargetLocale, &model.Target{
		Runs:   targetRuns,
		Status: model.TargetStatusDraft,
		Origin: conf.recycleOrigin(),
		Score:  float64(m.Score) / 100,
	})
	v.Annotate(string(model.AnnoMemoryMatch), &MemoryMatchAnnotation{Score: m.Score, Type: propType})
	return true
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

// leverageSegments attempts sentence-level content-memory leverage over a multi-segment
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
func leverageSegments(conf *MemoryLeverageConfig, v tool.VariantView) bool {
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
		if m, found := conf.Memory.Lookup(v.Context(), corememory.Request{
			Text:     segTexts[i],
			Source:   conf.SourceLocale,
			Target:   conf.TargetLocale,
			Point:    conf.Point,
			MinScore: 100,
			Verbatim: true,
		}); found {
			tr := model.RunsText(m.TargetRuns)
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
		if conf.FuzzyThreshold >= 100 {
			allExact = false
			continue
		}
		if m, found := conf.Memory.Lookup(v.Context(), corememory.Request{
			Text:     segTexts[i],
			Source:   conf.SourceLocale,
			Target:   conf.TargetLocale,
			Point:    conf.Point,
			MinScore: conf.FuzzyThreshold,
		}); found {
			tr, score := model.RunsText(m.TargetRuns), m.Score
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
	v.Annotate(string(model.AnnoMemoryMatch), &MemoryMatchAnnotation{SegmentMatches: segMatches})

	if matched < n {
		// Partial leverage: leave the block for a later whole-block translation
		// stage. We deliberately do not write a half-translated target here;
		// per-segment fill (mixing content memory and MT within one block) needs
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
	assembled := []model.Run{{Text: &model.TextRun{Text: strings.Join(translations, "")}}}
	// The per-segment TM lookups key on flattened segment text, so the
	// assembled target is plain text: it cannot carry any inline code the
	// source has. Filling it would drop them (see fillWouldDropCodes); the
	// segment matches stay recorded as alt-translations either way.
	if shouldFillTarget(conf, v, minScore, "") && !fillWouldDropCodes(v, assembled) {
		// Commit a real Target carrying provenance and score, not an opaque
		// string: a content memory pre-fill is a reviewable draft assembled from segment
		// matches, so a reviewer/tool can see it came from content memory and at what score.
		v.SetTarget(conf.TargetLocale, &model.Target{
			Runs:   assembled,
			Status: model.TargetStatusDraft,
			Origin: conf.recycleOrigin(),
			Score:  float64(minScore) / 100,
		})
	}
	v.Annotate(string(model.AnnoMemoryMatch), &MemoryMatchAnnotation{Score: minScore, Type: matchType, SegmentMatches: segMatches})
	return true
}

// annotateSegmentMatch records a per-segment content-memory hit as an AltTranslation
// annotation anchored by its source runs and segment index, so the leverage is
// auditable and partial matches survive even when the block target is not
// filled. The annotation key carries the segment index; the annotation itself
// carries the matched source/target, score (0-1), match type, and provenance.
func annotateSegmentMatch(v tool.VariantView, conf *MemoryLeverageConfig, idx int, srcRuns []model.Run, translation string, score int, mt model.MatchType) {
	v.AppendAltUnder(PropMemorySegmentAlts, &model.AltTranslation{
		Source:       srcRuns,
		Target:       []model.Run{{Text: &model.TextRun{Text: translation}}},
		Locale:       conf.TargetLocale,
		Origin:       "tm",
		Score:        float64(score) / 100,
		MatchType:    mt,
		ToolID:       "recycle",
		SegmentIndex: idx,
	})
}

// shouldFillTarget decides whether to copy the translation into the target.
//
// The edit kind is the gate that matters, and it OVERRIDES the score in both
// directions. A substantive change is refused however high it scores — a
// negation on a long sentence scores 95, and filling it writes the translation
// of the opposite meaning under a badge that stops anyone looking. A cosmetic
// change is accepted however low it scores — a full stop on a button label
// scores 91, and the words have not moved.
//
// The score still governs when the provider classified nothing, which is the
// behaviour that existed before and the one an older provider still gets.
func shouldFillTarget(conf *MemoryLeverageConfig, v tool.VariantView, score int, kind edit.Kind) bool {
	if !conf.FillTarget {
		return false
	}
	switch kind {
	case edit.None, edit.Cosmetic:
		// The words stand. Length decided nothing here, which is the point.
	case edit.Substantive:
		return false
	default:
		if score < conf.FillTargetThreshold {
			return false
		}
	}
	if conf.FillIfTargetIsEmpty {
		// Only fill if target is empty.
		if v.HasTarget(conf.TargetLocale) && v.TargetText(conf.TargetLocale) != "" {
			return false
		}
	}
	return true
}

// leverageText is the flattened fallback: the corpus asked by text rather than
// by block.
//
// It exists for entries stored without inline codes, which a block form cannot
// reach because it matches over runs. It is the weaker path by construction —
// asked by text, the corpus answers with a target and no source, so nothing can
// classify the edit and the fill falls back to the score alone.
//
// It now asks at the run's point, which it did not before. An answer approved
// somewhere else is still an answer from somewhere else, and this path was the
// last one that could not say where it was.
func leverageText(conf *MemoryLeverageConfig, v tool.VariantView, sourceText string) bool {
	// Verbatim first: the same text, matched plainly. This is not a discount on
	// labour but a guarantee that one string does not render two ways.
	if m, found := conf.Memory.Lookup(v.Context(), corememory.Request{
		Text:     sourceText,
		Source:   conf.SourceLocale,
		Target:   conf.TargetLocale,
		Point:    conf.Point,
		MinScore: 100,
		Verbatim: true,
	}); found {
		score := 100
		if conf.DowngradeIdenticalBestMatches {
			score = 99
		}
		recordWholeBlockMatch(v, conf, model.RunsText(m.TargetRuns), score, model.MatchExact, "exact")
		return true
	}

	// Below an exact match, only a recipe that lowered the threshold asks. The
	// band between is not offered as a draft and is not sent to a model either,
	// which is what made it bookkeeping rather than leverage.
	if conf.FuzzyThreshold >= 100 {
		return false
	}
	m, found := conf.Memory.Lookup(v.Context(), corememory.Request{
		Text:     sourceText,
		Source:   conf.SourceLocale,
		Target:   conf.TargetLocale,
		Point:    conf.Point,
		MinScore: conf.FuzzyThreshold,
	})
	if !found {
		return false
	}
	recordWholeBlockMatch(v, conf, model.RunsText(m.TargetRuns), m.Score, model.MatchFuzzy, "fuzzy")
	return true
}
