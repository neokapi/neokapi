package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/providers/ai"
)

// BrandVoiceCheckTool uses an LLM to check text against brand voice guidelines.
type BrandVoiceCheckTool struct {
	tool.BaseTool
	usageAccumulator
	provider aiprovider.LLMProvider
	profile  *brand.VoiceProfile
	resolver brand.ProfileResolver // optional: lazy profile resolution
	rc       brand.ResolveContext  // context for resolver
	resolved bool                  // true after first resolution attempt
}

// BrandVoiceCheckConfig holds configuration for the brand voice check tool.
type BrandVoiceCheckConfig struct {
	Provider  string `json:"provider,omitempty"  schema:"title=AI Provider,description=AI provider,default=anthropic,group=provider"`
	APIKey    string `json:"apiKey,omitempty"    schema:"title=API Key,description=API key for the AI provider,group=provider"`
	Model     string `json:"model,omitempty"     schema:"title=Model,description=AI model name,group=provider"`
	ProfileID string `json:"profileId,omitempty" schema:"title=Profile ID,description=Brand voice profile to resolve from the store"`
	// Profile is the resolved voice profile, supplied programmatically (e.g. by
	// the kapi brand command or a .kapi brand binding). Not serialized.
	Profile *brand.VoiceProfile `json:"-" schema:"-"`
}

func (c *BrandVoiceCheckConfig) ToolName() string { return "brand-voice-check" }
func (c *BrandVoiceCheckConfig) Reset()           {}
func (c *BrandVoiceCheckConfig) Validate() error  { return nil }

// BrandVoiceCheckSchema returns the auto-generated schema for the tool.
func BrandVoiceCheckSchema() *schema.ComponentSchema {
	s := schema.FromStruct(&BrandVoiceCheckConfig{}, schema.ToolMeta{
		ID:                    "brand-voice-check",
		Category:              schema.CategoryQuality,
		DisplayName:           "AI Brand Voice Check",
		Description:           "Check text against a brand voice profile using an LLM provider",
		Tags:                  []string{"ai-powered", "brand"},
		DefaultParallelBlocks: 5,
		Requires:              []string{schema.RequiresCredentials},
		Cardinality:           schema.Monolingual,
		Produces:              []schema.IOFacet{{Type: model.FacetBrandVoice, Side: model.SideTarget}},
		SideEffects:           []schema.SideEffect{schema.SideEffectAPICall},
	})
	injectProviderOptions(s)
	return s
}

// NewBrandVoiceCheckFromConfig creates a brand voice check tool from a config map.
// The map may carry a non-serializable "profile" (*brand.VoiceProfile) — already
// resolved by the caller — or a "profileResolver" (brand.ProfileResolver) plus
// "resolveContext" (brand.ResolveContext) for lazy hierarchical resolution.
func NewBrandVoiceCheckFromConfig(config map[string]any, _ string) (tool.Tool, error) {
	var profile *brand.VoiceProfile
	if pf, ok := config["profile"].(*brand.VoiceProfile); ok {
		profile = pf
		delete(config, "profile")
	}
	var resolver brand.ProfileResolver
	if r, ok := config["profileResolver"].(brand.ProfileResolver); ok {
		resolver = r
		delete(config, "profileResolver")
	}
	var rc brand.ResolveContext
	if c, ok := config["resolveContext"].(brand.ResolveContext); ok {
		rc = c
		delete(config, "resolveContext")
	}

	var cfg BrandVoiceCheckConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("brand-voice-check config: %w", err)
	}

	p, err := ProviderFromConfig(cfg.Provider, aiprovider.Config{APIKey: cfg.APIKey, Model: cfg.Model})
	if err != nil {
		return nil, err
	}
	if resolver != nil {
		return NewBrandVoiceCheckToolWithResolver(p, resolver, rc), nil
	}
	if profile == nil {
		profile = cfg.Profile
	}
	return NewBrandVoiceCheckTool(p, profile), nil
}

// NewBrandVoiceCheckTool creates a new LLM-based brand voice check tool.
func NewBrandVoiceCheckTool(p aiprovider.LLMProvider, profile *brand.VoiceProfile) *BrandVoiceCheckTool {
	t := &BrandVoiceCheckTool{
		provider: p,
		profile:  profile,
	}
	t.ToolName = "brand-voice-check"
	t.ToolDescription = "Checks text against brand voice guidelines using AI/LLM"
	t.Cfg = &BrandVoiceCheckConfig{Profile: profile}
	t.Annotate = t.annotate
	return t
}

// NewBrandVoiceCheckToolWithResolver creates a brand voice check tool that
// lazily resolves its profile from the organizational context hierarchy.
// The resolver is called once on first use and the result is cached.
func NewBrandVoiceCheckToolWithResolver(p aiprovider.LLMProvider, resolver brand.ProfileResolver, rc brand.ResolveContext) *BrandVoiceCheckTool {
	t := &BrandVoiceCheckTool{
		provider: p,
		resolver: resolver,
		rc:       rc,
	}
	t.ToolName = "brand-voice-check"
	t.ToolDescription = "Checks text against brand voice guidelines using AI/LLM"
	t.Cfg = &BrandVoiceCheckConfig{}
	t.Annotate = t.annotate
	return t
}

// brandVoiceSchema returns the JSON schema for structured brand voice findings.
func brandVoiceSchema() aiprovider.JSONSchema {
	return aiprovider.JSONSchema{
		Name:        "brand_voice_findings",
		Description: "Brand voice compliance findings for the given text",
		Strict:      true,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"findings": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"dimension":  map[string]any{"type": "string", "enum": []string{"tone", "style", "clarity", "brand_compliance"}},
							"severity":   map[string]any{"type": "string", "enum": []string{"neutral", "minor", "major", "critical"}},
							"message":    map[string]any{"type": "string"},
							"suggestion": map[string]any{"type": "string"},
						},
						"required":             []string{"dimension", "severity", "message", "suggestion"},
						"additionalProperties": false,
					},
				},
			},
			"required":             []string{"findings"},
			"additionalProperties": false,
		},
	}
}

// brandVoiceLLMFinding is the JSON structure for a single finding from the LLM.
type brandVoiceLLMFinding struct {
	Dimension  string `json:"dimension"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

// brandVoiceLLMResult is the JSON structure returned by the LLM.
type brandVoiceLLMResult struct {
	Findings []brandVoiceLLMFinding `json:"findings"`
}

func (t *BrandVoiceCheckTool) resolveOnce(ctx context.Context) {
	if t.resolved || t.resolver == nil {
		return
	}
	t.resolved = true
	profile, err := t.resolver.ResolveProfile(ctx, t.rc)
	if err == nil && profile != nil {
		t.profile = profile
	}
}

func (t *BrandVoiceCheckTool) annotate(v tool.BlockView) error {
	ctx := v.Context()
	t.resolveOnce(ctx)

	sourceText := v.SourceText()
	if strings.TrimSpace(sourceText) == "" {
		return nil
	}

	prompt := t.buildPrompt(sourceText)

	resp, err := t.provider.ChatStructured(ctx, []aiprovider.Message{
		{Role: "user", Content: prompt},
	}, brandVoiceSchema())
	if err != nil {
		return fmt.Errorf("brand-voice-check: %w", err)
	}
	t.addUsage(resp.Usage)

	var result brandVoiceLLMResult
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		result.Findings = nil
	}

	// Convert LLM findings to BrandVoiceFinding structs.
	var findings []brand.BrandVoiceFinding
	for _, f := range result.Findings {
		findings = append(findings, brand.BrandVoiceFinding{
			Category:   f.Dimension,
			Severity:   brand.Severity(f.Severity),
			Message:    f.Message,
			Suggestion: f.Suggestion,
		})
	}

	// Calculate brand compliance score.
	score := brand.CalculateScore(findings)
	profileID := ""
	if t.profile != nil {
		profileID = t.profile.ID
	}
	score.ProfileID = profileID

	scoreJSON, _ := json.Marshal(score)
	v.SetProperty("brand-voice-score", string(scoreJSON))

	if len(findings) > 0 {
		findingsJSON, _ := json.Marshal(findings)
		v.SetProperty("brand-voice-findings", string(findingsJSON))
	}

	// Add annotation.
	v.Annotate("brand-voice", &brand.BrandVoiceAnnotation{
		ProfileID: profileID,
		Score:     score.Overall,
		Findings:  findings,
	})

	return nil
}

// buildPrompt constructs the LLM prompt from the voice profile and text. The
// guidelines section is rendered by brand.RenderVoiceGuide — the single source
// of truth shared with the translate prompt and the bowrain MCP voice guide.
func (t *BrandVoiceCheckTool) buildPrompt(text string) string {
	var b strings.Builder

	b.WriteString("You are a brand voice compliance checker. Analyze the following text against brand voice guidelines and report any issues.\n\n")

	if t.profile != nil {
		b.WriteString(brand.RenderVoiceGuide(t.profile))
		b.WriteString("\n")
	}

	b.WriteString(fmt.Sprintf("## Text to check\n\n%s\n\n", text))
	b.WriteString("Return findings for any issues with tone, style, clarity, or brand compliance. ")
	b.WriteString("Return an empty findings array if the text fully complies.")

	return b.String()
}
