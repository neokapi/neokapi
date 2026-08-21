package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/ai/prompt"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// VoiceCheckTool uses an LLM to check text against voice guidelines.
type VoiceCheckTool struct {
	tool.BaseTool
	usageAccumulator
	provider aiprovider.LLMProvider
	profile  *coreprofile.VoiceProfile
	resolver coreprofile.ProfileResolver // optional: lazy profile resolution
	rc       coreprofile.ResolveContext  // context for resolver
	resolved bool                        // true after first resolution attempt
}

// VoiceCheckConfig holds configuration for the voice profile check tool.
type VoiceCheckConfig struct {
	Provider  string `json:"provider,omitempty"  schema:"title=AI Provider,description=AI provider,default=anthropic,group=provider"`
	APIKey    string `json:"apiKey,omitempty"    schema:"title=API Key,description=API key for the AI provider,group=provider"`
	Model     string `json:"model,omitempty"     schema:"title=Model,description=AI model name,group=provider"`
	ProfileID string `json:"profileId,omitempty" schema:"title=Profile ID,description=Voice profile to resolve from the store"`
	// Profile is the resolved voice profile, supplied programmatically (e.g. by
	// the kapi voice command or a .kapi voice binding). Not serialized.
	Profile *coreprofile.VoiceProfile `json:"-" schema:"-"`
}

func (c *VoiceCheckConfig) ToolName() string { return "voice-check" }
func (c *VoiceCheckConfig) Reset()           {}
func (c *VoiceCheckConfig) Validate() error  { return nil }

// VoiceCheckSchema returns the auto-generated schema for the tool.
func VoiceCheckSchema() *schema.ComponentSchema {
	s := schema.FromStruct(&VoiceCheckConfig{}, schema.ToolMeta{
		ID:                    "voice-check",
		Category:              schema.CategoryQuality,
		DisplayName:           "AI Voice Check",
		Description:           "Check text against a voice profile using an LLM provider",
		Tags:                  []string{"ai-powered", "voice"},
		DefaultParallelBlocks: 5,
		Requires:              []string{schema.RequiresCredentials},
		Cardinality:           schema.Monolingual,
		Produces:              []schema.IOPort{{Type: model.AnnoVoice, Side: model.SideTarget}},
		SideEffects:           []schema.SideEffect{schema.SideEffectAPICall, schema.SideEffectRemoteSourceEgress},
	})
	injectProviderOptions(s)
	return s
}

// NewVoiceCheckFromConfig creates a voice profile check tool from a config map.
// The map may carry a non-serializable "profile" (*coreprofile.VoiceProfile) — already
// resolved by the caller — or a "profileResolver" (coreprofile.ProfileResolver) plus
// "resolveContext" (coreprofile.ResolveContext) for lazy hierarchical resolution.
func NewVoiceCheckFromConfig(config map[string]any, _ string) (tool.Tool, error) {
	var profile *coreprofile.VoiceProfile
	if pf, ok := config["profile"].(*coreprofile.VoiceProfile); ok {
		profile = pf
		delete(config, "profile")
	}
	var resolver coreprofile.ProfileResolver
	if r, ok := config["profileResolver"].(coreprofile.ProfileResolver); ok {
		resolver = r
		delete(config, "profileResolver")
	}
	var rc coreprofile.ResolveContext
	if c, ok := config["resolveContext"].(coreprofile.ResolveContext); ok {
		rc = c
		delete(config, "resolveContext")
	}

	var cfg VoiceCheckConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("voice-check config: %w", err)
	}

	p, err := ProviderFromConfig(cfg.Provider, aiprovider.Config{APIKey: cfg.APIKey, Model: cfg.Model})
	if err != nil {
		return nil, err
	}
	if resolver != nil {
		return NewVoiceCheckToolWithResolver(p, resolver, rc), nil
	}
	if profile == nil {
		profile = cfg.Profile
	}
	return NewVoiceCheckTool(p, profile), nil
}

// NewVoiceCheckTool creates a new LLM-based voice profile check tool.
func NewVoiceCheckTool(p aiprovider.LLMProvider, profile *coreprofile.VoiceProfile) *VoiceCheckTool {
	t := &VoiceCheckTool{
		provider: p,
		profile:  profile,
	}
	t.ToolName = "voice-check"
	t.ToolDescription = "Checks text against voice guidelines using AI/LLM"
	t.Cfg = &VoiceCheckConfig{Profile: profile}
	t.Annotate = t.annotate
	return t
}

// NewVoiceCheckToolWithResolver creates a voice profile check tool that
// lazily resolves its profile from the organizational context hierarchy.
// The resolver is called once on first use and the result is cached.
func NewVoiceCheckToolWithResolver(p aiprovider.LLMProvider, resolver coreprofile.ProfileResolver, rc coreprofile.ResolveContext) *VoiceCheckTool {
	t := &VoiceCheckTool{
		provider: p,
		resolver: resolver,
		rc:       rc,
	}
	t.ToolName = "voice-check"
	t.ToolDescription = "Checks text against voice guidelines using AI/LLM"
	t.Cfg = &VoiceCheckConfig{}
	t.Annotate = t.annotate
	return t
}

// voiceFindingsSchema returns the JSON schema for structured voice profile findings.
func voiceFindingsSchema() aiprovider.JSONSchema {
	return aiprovider.JSONSchema{
		Name:        "brand_voice_findings",
		Description: "Voice profile compliance findings for the given text",
		Strict:      true,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"findings": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"dimension":  map[string]any{"type": "string", "enum": []string{"tone", "style", "clarity", "compliance"}},
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

// voiceLLMFinding is the JSON structure for a single finding from the LLM.
type voiceLLMFinding struct {
	Dimension  string `json:"dimension"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

// voiceLLMResult is the JSON structure returned by the LLM.
type voiceLLMResult struct {
	Findings []voiceLLMFinding `json:"findings"`
}

func (t *VoiceCheckTool) resolveOnce(ctx context.Context) {
	if t.resolved || t.resolver == nil {
		return
	}
	t.resolved = true
	profile, err := t.resolver.ResolveProfile(ctx, t.rc)
	if err == nil && profile != nil {
		t.profile = profile
	}
}

func (t *VoiceCheckTool) annotate(v tool.BlockView) error {
	ctx := v.Context()
	t.resolveOnce(ctx)

	sourceText := v.SourceText()
	if strings.TrimSpace(sourceText) == "" {
		return nil
	}

	p := prompt.VoiceCheck{VoiceGuide: coreprofile.RenderVoiceGuide(t.profile)}
	turns := p.Turns(sourceText)
	ctx = prompt.WithID(ctx, prompt.IDVoiceCheck)
	resp, err := t.provider.ChatStructured(ctx, aiprovider.MessagesFromTurns(turns), voiceFindingsSchema())
	if err != nil {
		return fmt.Errorf("voice-check: %w", err)
	}
	t.addUsage(resp.Usage)

	var result voiceLLMResult
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		result.Findings = nil
	}

	// Convert LLM findings to VoiceFinding structs.
	var findings []coreprofile.VoiceFinding
	for _, f := range result.Findings {
		findings = append(findings, coreprofile.VoiceFinding{
			Category:   f.Dimension,
			Severity:   coreprofile.Severity(f.Severity),
			Message:    f.Message,
			Suggestion: f.Suggestion,
		})
	}

	// Calculate voice compliance score.
	score := coreprofile.CalculateScore(findings)
	profileID := ""
	if t.profile != nil {
		profileID = t.profile.ID
	}
	score.ProfileID = profileID

	scoreJSON, _ := json.Marshal(score)
	v.SetProperty("voice-score", string(scoreJSON))

	if len(findings) > 0 {
		findingsJSON, _ := json.Marshal(findings)
		v.SetProperty("voice-findings", string(findingsJSON))
	}

	// Add annotation.
	v.Annotate("voice", &coreprofile.VoiceAnnotation{
		ProfileID: profileID,
		Score:     score.Overall,
		Findings:  findings,
	})

	return nil
}
