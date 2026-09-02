package tools

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/ai/prompt"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// AIReviewTool reviews translations with explanations using an LLM.
//
// Output contract: for every block that has a target in the configured
// locale the tool sets these block properties:
//
//   - "review" — the review payload. When the model returns the requested
//     JSON contract
//
//     {"score": <0-100>, "findings": [{"severity": "critical|major|minor|info",
//     "message": "...", "suggestion": "..."}]}
//
//     the property holds that result re-marshalled canonically (parsed via
//     [ParseReviewResult]: score clamped to 0–100, severities normalized,
//     empty-message findings dropped). When the model output is not
//     parseable as JSON the raw prose is stored unchanged — the lenient
//     fallback that keeps older prose-style model output working.
//
//   - "review-score" — the integer score as a decimal string. Only set when
//     the JSON contract parsed.
//
//   - "review-provider" — the provider id that produced the review. Always set.
type AIReviewTool struct {
	tool.BaseTool
	usageAccumulator
	provider     aiprovider.LLMProvider
	sourceLocale model.LocaleID
	targetLocale model.LocaleID
	// voiceGuide is the governing voice profile rendered for the prompt, so a
	// score answers the question the voice checks ask rather than a generic one
	// about accuracy and fluency.
	voiceGuide string
	// termMap is the governing terminology projected to term → replacement,
	// through the one projection a producer's prompt takes.
	termMap map[string]string
}

// AIReviewConfig holds configuration for the review tool.
type AIReviewConfig struct {
	SourceLocale model.LocaleID `json:"sourceLocale,omitempty" schema:"-"`
	TargetLocale model.LocaleID `json:"targetLocale,omitempty" schema:"-"`
	Provider     string         `json:"provider,omitempty"     schema:"title=AI Provider,description=AI provider,default=anthropic,group=provider"`
	APIKey       string         `json:"apiKey,omitempty"       schema:"title=API Key,description=API key for the AI provider,group=provider"`
	Model        string         `json:"model,omitempty"        schema:"title=Model,description=AI model name,group=provider"`

	// TermRules are the terminology constraints governing the translation under
	// review: for each, when the source says Term the target should say
	// Replacement. The same list translate renders into its prompt, so a
	// reviewer scores against the vocabulary the producer was given.
	TermRules []coreprofile.TermRule `json:"term_rules,omitempty" schema:"-"`

	// Profile is the voice profile governing the translation under review. Its
	// guidance is rendered into the review prompt, so a target that reads
	// correctly but off-voice is reported rather than scored clean. Not
	// serializable via the schema/CLI; supplied by the project bindings.
	Profile *coreprofile.VoiceProfile `json:"-" schema:"-"`
}

// AIReviewSchema returns the auto-generated schema for the AI review tool.
func AIReviewSchema() *schema.ComponentSchema {
	s := schema.FromStruct(&AIReviewConfig{}, schema.ToolMeta{
		ID:                    "review",
		Category:              schema.CategoryQuality,
		DisplayName:           "Review",
		Description:           "Review translations with scoring using an LLM provider",
		Tags:                  []string{"ai-powered"},
		WritesOutput:          true,
		DefaultParallelBlocks: 5,
		Requires:              []string{schema.RequiresTargetLanguage, schema.RequiresCredentials},
		Cardinality:           schema.Bilingual,
		// review writes the "review" block property (the JSON contract
		// documented on AIReviewTool, or raw prose as fallback), not a
		// registered findings annotation, so it declares no Produces type.
		SideEffects: []schema.SideEffect{schema.SideEffectAPICall, schema.SideEffectRemoteSourceEgress},
	})
	injectProviderOptions(s)
	return s
}

// NewAIReviewFromConfig creates an AI review tool from a config map.
func NewAIReviewFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	// The voice profile is a live handle: it does not survive the JSON round
	// trip the rest of the config takes, so it comes out first, exactly as the
	// translate factory takes it.
	var profile *coreprofile.VoiceProfile
	if pf, ok := config["profile"].(*coreprofile.VoiceProfile); ok {
		profile = pf
		delete(config, "profile")
	}

	var cfg AIReviewConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("review config: %w", err)
	}
	cfg.Profile = profile
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	p, err := ProviderFromConfig(cfg.Provider, aiprovider.Config{APIKey: cfg.APIKey, Model: cfg.Model})
	if err != nil {
		return nil, err
	}
	return NewAIReviewTool(p, cfg), nil
}

// NewAIReviewTool creates a new AI translation review tool.
func NewAIReviewTool(p aiprovider.LLMProvider, cfg AIReviewConfig) *AIReviewTool {
	t := &AIReviewTool{
		provider:     p,
		sourceLocale: cfg.SourceLocale,
		targetLocale: cfg.TargetLocale,
		voiceGuide:   coreprofile.RenderVoiceGuideCompact(cfg.Profile),
		termMap:      coreprofile.TermRuleMap(cfg.TermRules),
	}
	t.ToolName = "review"
	t.ToolDescription = "Reviews translations with explanations using AI/LLM"
	t.Annotate = t.annotate
	return t
}

// ReviewFinding is one issue reported by the review tool.
type ReviewFinding struct {
	// Severity is one of "critical", "major", "minor", or "info".
	// Unknown or missing values normalize to "info".
	Severity string `json:"severity"`
	// Message describes the issue. Findings without a message are dropped.
	Message string `json:"message"`
	// Suggestion is an optional improved translation or fix.
	Suggestion string `json:"suggestion,omitempty"`
}

// ReviewResult is the structured output contract of the review tool:
// an overall 0–100 quality score plus zero or more findings.
type ReviewResult struct {
	Score    int             `json:"score"`
	Findings []ReviewFinding `json:"findings"`
}

// reviewSeverities are the severity values the contract admits.
var reviewSeverities = map[string]bool{
	"critical": true, "major": true, "minor": true, "info": true,
}

// ParseReviewResult parses a model response against the review output
// contract {score: 0-100, findings: [{severity, message, suggestion}]}.
// It is lenient about transport noise — a markdown code fence, leading
// prose, or trailing commentary around the JSON value are tolerated (see
// decodeFirstJSON) — and normalizes the payload: the score is clamped to
// [0, 100], severities are lower-cased and default to "info" when unknown
// or absent, and findings without a message are dropped. It returns an
// error only when no JSON object can be found at all; callers fall back
// to treating the response as prose.
func ParseReviewResult(content string) (*ReviewResult, error) {
	var raw struct {
		Score    float64         `json:"score"`
		Findings []ReviewFinding `json:"findings"`
	}
	if err := decodeFirstJSON(content, &raw); err != nil {
		return nil, fmt.Errorf("review: parse response: %w", err)
	}

	res := &ReviewResult{Score: int(raw.Score)}
	if res.Score < 0 {
		res.Score = 0
	}
	if res.Score > 100 {
		res.Score = 100
	}
	res.Findings = make([]ReviewFinding, 0, len(raw.Findings))
	for _, f := range raw.Findings {
		if strings.TrimSpace(f.Message) == "" {
			continue
		}
		f.Severity = strings.ToLower(strings.TrimSpace(f.Severity))
		if !reviewSeverities[f.Severity] {
			f.Severity = "info"
		}
		res.Findings = append(res.Findings, f)
	}
	return res, nil
}

func (t *AIReviewTool) annotate(v tool.BlockView) error {
	if !v.HasTarget(t.targetLocale) {
		return nil
	}

	sourceText := v.SourceText()
	targetText := v.TargetText(t.targetLocale)

	turns := prompt.Review{
		SourceLocale:   t.sourceLocale,
		TargetLocale:   t.targetLocale,
		VoiceGuide:     t.voiceGuide,
		PreferredTerms: t.termMap,
	}.Turns(sourceText, targetText)

	ctx := prompt.WithID(v.Context(), prompt.IDReview)
	resp, err := t.provider.Chat(ctx, aiprovider.MessagesFromTurns(turns))
	if err != nil {
		return fmt.Errorf("review: %w", err)
	}
	t.addUsage(resp.Usage)

	// Structured contract when the model honored it; raw prose otherwise
	// (backward-lenient — older models answered in free text).
	if result, perr := ParseReviewResult(resp.Content); perr == nil {
		payload, merr := json.Marshal(result)
		if merr != nil {
			return fmt.Errorf("review: encode result: %w", merr)
		}
		v.SetProperty("review", string(payload))
		v.SetProperty("review-score", strconv.Itoa(result.Score))
	} else {
		v.SetProperty("review", resp.Content)
	}
	v.SetProperty("review-provider", string(t.provider.Name()))

	return nil
}
