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
	// PropTMSegmentAltPrefix is the annotation-key prefix under which each
	// per-segment TM match is stored as an AltTranslation (the segment index is
	// appended, e.g. "tm-segment-alt:2").
	PropTMSegmentAltPrefix = "tm-segment-alt:"
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
		Inputs:      []string{schema.PartTypeBlock},
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
		if existingScore := v.Property(PropTMMatchScore); existingScore != "" && conf.NoQueryThreshold <= 101 {
			if score, err := strconv.Atoi(existingScore); err == nil && score >= conf.NoQueryThreshold {
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
	v.Annotate(PropTMAltKey, &model.AltTranslation{
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
	v.SetProperty(PropTMMatchScore, strconv.Itoa(score))
	v.SetProperty(PropTMMatchType, propType)
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
	for i := range n {
		segRuns := v.SourceSegmentRuns(i)
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
	v.SetProperty(PropTMSegmentMatches, strconv.Itoa(matched)+"/"+strconv.Itoa(n))

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
	v.SetProperty(PropTMMatchScore, strconv.Itoa(minScore))
	v.SetProperty(PropTMMatchType, matchType)
	return true
}

// annotateSegmentMatch records a per-segment TM hit as an AltTranslation
// annotation anchored by its source runs and segment index, so the leverage is
// auditable and partial matches survive even when the block target is not
// filled. The annotation key carries the segment index; the annotation itself
// carries the matched source/target, score (0-1), match type, and provenance.
func annotateSegmentMatch(v tool.TargetView, conf *TMLeverageConfig, idx int, srcRuns []model.Run, translation string, score int, mt model.MatchType) {
	v.Annotate(PropTMSegmentAltPrefix+strconv.Itoa(idx), &model.AltTranslation{
		Source:    srcRuns,
		Target:    []model.Run{{Text: &model.TextRun{Text: translation}}},
		Locale:    conf.TargetLocale,
		Origin:    "tm",
		Score:     float64(score) / 100,
		MatchType: mt,
		ToolID:    "tm-leverage",
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
