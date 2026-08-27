package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/ai/prompt"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// ErrEmptyCorpus is returned by InferVoiceProfile when the corpus contains no
// analyzable text. Callers that treat an empty stream as "nothing to infer
// from" (rather than a failure) match it with errors.Is.
var ErrEmptyCorpus = errors.New("voice-infer: empty corpus")

// DefaultMaxCorpusChars bounds how much corpus text one inference sends. Named
// so a caller accumulating the corpus can stop at the same point rather than
// hold text the inference will truncate.
const DefaultMaxCorpusChars = 24000

// InferOptions configures a voice-profile inference run.
type InferOptions struct {
	// ProfileName names the draft profile. Defaults to "Inferred Voice".
	ProfileName string
	// Domain is an optional subject-domain hint (e.g. "medical", "developer
	// tools") woven into the prompt.
	Domain string
	// MaxExamples caps the before/after examples requested from and kept out
	// of the model's response. Defaults to 3.
	MaxExamples int
	// MaxCorpusChars bounds how much corpus text is sent to the provider; a
	// longer corpus is truncated on a rune boundary. Defaults to 24000.
	MaxCorpusChars int
}

// withDefaults returns a copy of the options with zero values filled in.
func (o InferOptions) withDefaults() InferOptions {
	if strings.TrimSpace(o.ProfileName) == "" {
		o.ProfileName = "Inferred Voice"
	}
	if o.MaxExamples <= 0 {
		o.MaxExamples = 3
	}
	if o.MaxCorpusChars <= 0 {
		o.MaxCorpusChars = DefaultMaxCorpusChars
	}
	return o
}

// FieldEvidence records the model's confidence and source rationale for one
// inferred top-level profile field.
type FieldEvidence struct {
	// Confidence is the model's 0–1 confidence in the inference for the field.
	Confidence float64 `json:"confidence"`
	// Source is a short note describing the corpus evidence the inference
	// rests on.
	Source string `json:"source"`
}

// DraftEvidence is the sidecar accompanying an inferred draft VoiceProfile:
// per-top-level-field confidence and source notes, keyed by field name
// ("tone", "style", "vocabulary", "examples"). It stays out of the profile
// itself so the draft round-trips through the ordinary profile.yaml shape.
type DraftEvidence struct {
	Fields map[string]FieldEvidence `json:"fields"`
}

// InferVoiceProfile infers a draft voice profile from a text corpus
// using the given LLM provider — the inverse of the voice-check tool,
// which scores text against an existing profile. It returns the draft profile
// (mapped into the canonical profile.VoiceProfile shape) plus a DraftEvidence
// sidecar carrying the model's per-field confidence and source notes.
//
// An empty (or whitespace-only) corpus returns ErrEmptyCorpus; a tiny corpus
// is analyzed as-is and simply yields a low-evidence draft.
func InferVoiceProfile(ctx context.Context, provider aiprovider.LLMProvider, corpus string, opts InferOptions) (*profile.VoiceProfile, *DraftEvidence, error) {
	draft, evidence, _, err := inferVoiceProfile(ctx, provider, corpus, opts)
	return draft, evidence, err
}

// inferVoiceProfile is InferVoiceProfile plus the provider token usage, for
// callers (the tool) that account usage.
func inferVoiceProfile(ctx context.Context, provider aiprovider.LLMProvider, corpus string, opts InferOptions) (*profile.VoiceProfile, *DraftEvidence, aiprovider.TokenUsage, error) {
	var usage aiprovider.TokenUsage
	if provider == nil {
		return nil, nil, usage, errors.New("voice-infer: nil provider")
	}
	corpus = strings.TrimSpace(corpus)
	if corpus == "" {
		return nil, nil, usage, ErrEmptyCorpus
	}
	opts = opts.withDefaults()
	corpus = truncateRunes(corpus, opts.MaxCorpusChars)

	turns := prompt.VoiceInfer{Domain: opts.Domain, MaxExamples: opts.MaxExamples}.Turns(corpus)
	ctx = prompt.WithID(ctx, prompt.IDVoiceInfer)
	resp, err := provider.ChatStructured(ctx, aiprovider.MessagesFromTurns(turns), voiceInferLLMSchema())
	if err != nil {
		return nil, nil, usage, fmt.Errorf("voice-infer: %w", err)
	}
	usage = resp.Usage

	var result inferLLMResult
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		return nil, nil, usage, fmt.Errorf("voice-infer: decode structured response: %w", err)
	}

	draft, evidence := mapInferResult(result, opts)
	return draft, evidence, usage, nil
}

// voiceInferLLMSchema returns the JSON schema for structured voice-profile
// inference output. The shape maps 1:1 into a profile.VoiceProfile subset plus
// the per-field evidence sidecar.
func voiceInferLLMSchema() aiprovider.JSONSchema {
	pattern := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"regex":       map[string]any{"type": "string"},
			"description": map[string]any{"type": "string"},
			"severity":    map[string]any{"type": "string", "enum": []string{"minor", "major", "critical"}},
		},
		"required":             []string{"regex", "description", "severity"},
		"additionalProperties": false,
	}
	term := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"term":        map[string]any{"type": "string"},
			"replacement": map[string]any{"type": "string"},
			"note":        map[string]any{"type": "string"},
		},
		"required":             []string{"term", "replacement", "note"},
		"additionalProperties": false,
	}
	termArray := func() map[string]any {
		return map[string]any{"type": "array", "items": term}
	}
	return aiprovider.JSONSchema{
		Name:        "brand_voice_inference",
		Description: "Draft voice profile inferred from a content corpus, with per-field evidence",
		Strict:      true,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tone": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"personality": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"formality":   map[string]any{"type": "string", "enum": []string{"casual", "neutral", "formal", "technical"}},
						"emotion":     map[string]any{"type": "string"},
						"humor":       map[string]any{"type": "string", "enum": []string{"none", "light", "frequent"}},
						"guidelines":  map[string]any{"type": "string"},
					},
					"required":             []string{"personality", "formality", "emotion", "humor", "guidelines"},
					"additionalProperties": false,
				},
				"style": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"active_voice":        map[string]any{"type": "boolean"},
						"sentence_length":     map[string]any{"type": "string", "enum": []string{"short", "medium", "varied"}},
						"person_pov":          map[string]any{"type": "string", "enum": []string{"first_plural", "second", "third"}},
						"contractions":        map[string]any{"type": "string", "enum": []string{"always", "sometimes", "never"}},
						"prohibited_patterns": map[string]any{"type": "array", "items": pattern},
					},
					"required":             []string{"active_voice", "sentence_length", "person_pov", "contractions", "prohibited_patterns"},
					"additionalProperties": false,
				},
				"vocabulary": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"preferred":  termArray(),
						"forbidden":  termArray(),
						"competitor": termArray(),
					},
					"required":             []string{"preferred", "forbidden", "competitor"},
					"additionalProperties": false,
				},
				"examples": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"before":      map[string]any{"type": "string"},
							"after":       map[string]any{"type": "string"},
							"explanation": map[string]any{"type": "string"},
						},
						"required":             []string{"before", "after", "explanation"},
						"additionalProperties": false,
					},
				},
				"evidence": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"field":      map[string]any{"type": "string", "enum": []string{"tone", "style", "vocabulary", "examples"}},
							"confidence": map[string]any{"type": "number"},
							"source":     map[string]any{"type": "string"},
						},
						"required":             []string{"field", "confidence", "source"},
						"additionalProperties": false,
					},
				},
			},
			"required":             []string{"tone", "style", "vocabulary", "examples", "evidence"},
			"additionalProperties": false,
		},
	}
}

// inferLLMResult is the JSON structure returned by structured inference.
type inferLLMResult struct {
	Tone       inferToneResult       `json:"tone"`
	Style      inferStyleResult      `json:"style"`
	Vocabulary inferVocabularyResult `json:"vocabulary"`
	Examples   []inferExampleResult  `json:"examples"`
	Evidence   []inferEvidenceResult `json:"evidence"`
}

type inferToneResult struct {
	Personality []string `json:"personality"`
	Formality   string   `json:"formality"`
	Emotion     string   `json:"emotion"`
	Humor       string   `json:"humor"`
	Guidelines  string   `json:"guidelines"`
}

type inferStyleResult struct {
	ActiveVoice        bool                 `json:"active_voice"`
	SentenceLength     string               `json:"sentence_length"`
	PersonPOV          string               `json:"person_pov"`
	Contractions       string               `json:"contractions"`
	ProhibitedPatterns []inferPatternResult `json:"prohibited_patterns"`
}

type inferPatternResult struct {
	Regex       string `json:"regex"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}

type inferVocabularyResult struct {
	Preferred  []inferTermResult `json:"preferred"`
	Forbidden  []inferTermResult `json:"forbidden"`
	Competitor []inferTermResult `json:"competitor"`
}

type inferTermResult struct {
	Term        string `json:"term"`
	Replacement string `json:"replacement"`
	Note        string `json:"note"`
}

type inferExampleResult struct {
	Before      string `json:"before"`
	After       string `json:"after"`
	Explanation string `json:"explanation"`
}

type inferEvidenceResult struct {
	Field      string  `json:"field"`
	Confidence float64 `json:"confidence"`
	Source     string  `json:"source"`
}

// mapInferResult maps the structured LLM result into the canonical
// profile.VoiceProfile shape plus the DraftEvidence sidecar. Confidences are
// clamped to [0,1]; empty terms, patterns, and examples are dropped.
func mapInferResult(res inferLLMResult, opts InferOptions) (*profile.VoiceProfile, *DraftEvidence) {
	draft := &profile.VoiceProfile{
		Name:        opts.ProfileName,
		Description: "Draft voice profile inferred from a content corpus. Review and adjust before adopting.",
		Tone: profile.ToneProfile{
			Personality: res.Tone.Personality,
			Formality:   res.Tone.Formality,
			Emotion:     res.Tone.Emotion,
			Humor:       res.Tone.Humor,
			Guidelines:  res.Tone.Guidelines,
		},
		Style: profile.StyleRules{
			ActiveVoice:    res.Style.ActiveVoice,
			SentenceLength: res.Style.SentenceLength,
			PersonPOV:      res.Style.PersonPOV,
			Contractions:   res.Style.Contractions,
		},
		Vocabulary: profile.VocabularyRules{
			PreferredTerms:  mapInferTermRules(res.Vocabulary.Preferred),
			ForbiddenTerms:  mapInferTermRules(res.Vocabulary.Forbidden),
			CompetitorTerms: mapInferTermRules(res.Vocabulary.Competitor),
		},
	}
	for _, p := range res.Style.ProhibitedPatterns {
		if strings.TrimSpace(p.Regex) == "" {
			continue
		}
		draft.Style.ProhibitedPatterns = append(draft.Style.ProhibitedPatterns, profile.Pattern{
			Regex:       p.Regex,
			Description: p.Description,
			Severity:    p.Severity,
		})
	}
	for _, ex := range res.Examples {
		if len(draft.Examples) >= opts.MaxExamples {
			break
		}
		if strings.TrimSpace(ex.Before) == "" && strings.TrimSpace(ex.After) == "" {
			continue
		}
		draft.Examples = append(draft.Examples, profile.VoiceExample{
			Before:      ex.Before,
			After:       ex.After,
			Explanation: ex.Explanation,
		})
	}

	evidence := &DraftEvidence{Fields: make(map[string]FieldEvidence, len(res.Evidence))}
	for _, e := range res.Evidence {
		field := strings.TrimSpace(e.Field)
		if field == "" {
			continue
		}
		evidence.Fields[field] = FieldEvidence{
			Confidence: clamp01(e.Confidence),
			Source:     strings.TrimSpace(e.Source),
		}
	}
	return draft, evidence
}

// mapInferTermRules maps LLM term entries into profile.TermRule, dropping
// entries with no term.
func mapInferTermRules(in []inferTermResult) []profile.TermRule {
	var out []profile.TermRule
	for _, t := range in {
		if strings.TrimSpace(t.Term) == "" {
			continue
		}
		out = append(out, profile.TermRule{Term: t.Term, Replacement: t.Replacement, Note: t.Note})
	}
	return out
}

// clamp01 clamps v into [0,1]; NaN clamps to 0.
func clamp01(v float64) float64 {
	if v != v || v < 0 { // v != v catches NaN
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// truncateRunes cuts s to at most n runes (on a rune boundary).
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return s
	}
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}

// ---------------------------------------------------------------------------
// Tool wrapper
// ---------------------------------------------------------------------------

// VoiceInferTool infers a draft voice profile from the content
// stream — the inverse of VoiceCheckTool. It accumulates every block's
// source text as the corpus, passes all parts through unchanged, and runs a
// single inference when the stream ends. The draft and its evidence sidecar
// are read from Draft() after the flow completes.
type VoiceInferTool struct {
	tool.BaseTool
	usageAccumulator
	provider aiprovider.LLMProvider
	opts     InferOptions

	mu       sync.Mutex
	draft    *profile.VoiceProfile
	evidence *DraftEvidence
}

// VoiceInferConfig holds configuration for the voice profile infer tool.
type VoiceInferConfig struct {
	Provider    string `json:"provider,omitempty"    schema:"title=AI Provider,description=AI provider,default=anthropic,group=provider"`
	APIKey      string `json:"apiKey,omitempty"      schema:"title=API Key,description=API key for the AI provider,group=provider"`
	Model       string `json:"model,omitempty"       schema:"title=Model,description=AI model name,group=provider"`
	ProfileName string `json:"profileName,omitempty" schema:"title=Profile Name,description=Name for the inferred draft profile"`
	Domain      string `json:"domain,omitempty"      schema:"title=Domain,description=Subject domain hint for the analysis (e.g. medical legal technology)"`
	MaxExamples int    `json:"maxExamples,omitempty" schema:"title=Max Examples,description=Maximum before/after examples to infer,default=3,min=1"`
}

func (c *VoiceInferConfig) ToolName() string { return "voice-infer" }
func (c *VoiceInferConfig) Reset()           {}
func (c *VoiceInferConfig) Validate() error  { return nil }

// VoiceInferSchema returns the auto-generated schema for the tool.
func VoiceInferSchema() *schema.ComponentSchema {
	s := schema.FromStruct(&VoiceInferConfig{}, schema.ToolMeta{
		ID:          "voice-infer",
		Category:    schema.CategoryAnalysis,
		DisplayName: "AI Voice Inference",
		Description: "Infer a draft voice profile from a content corpus using an LLM provider",
		Tags:        []string{"ai-powered", "voice"},
		Requires:    []string{schema.RequiresCredentials},
		Cardinality: schema.Monolingual,
		SideEffects: []schema.SideEffect{schema.SideEffectAPICall, schema.SideEffectRemoteSourceEgress},
	})
	injectProviderOptions(s)
	return s
}

// NewVoiceInferFromConfig creates a voice profile infer tool from a config map.
func NewVoiceInferFromConfig(config map[string]any, _ string) (tool.Tool, error) {
	var cfg VoiceInferConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("voice-infer config: %w", err)
	}
	p, err := ProviderFromConfig(cfg.Provider, aiprovider.Config{APIKey: cfg.APIKey, Model: cfg.Model})
	if err != nil {
		return nil, err
	}
	return NewVoiceInferTool(p, cfg), nil
}

// NewVoiceInferTool creates a new LLM-based voice profile inference tool.
func NewVoiceInferTool(p aiprovider.LLMProvider, cfg VoiceInferConfig) *VoiceInferTool {
	t := &VoiceInferTool{
		provider: p,
		opts: InferOptions{
			ProfileName: cfg.ProfileName,
			Domain:      cfg.Domain,
			MaxExamples: cfg.MaxExamples,
		},
	}
	t.ToolName = "voice-infer"
	t.ToolDescription = "Infers a draft voice profile from content using AI/LLM"
	t.Cfg = &cfg
	return t
}

// Process overrides BaseTool.Process: inference is corpus-level, not
// per-block, so the tool forwards every part unchanged while accumulating
// block source text, then infers once when the input stream ends. An empty
// stream is not an error — there is simply nothing to infer from and Draft()
// stays nil.
func (t *VoiceInferTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	// inferVoiceProfile truncates the corpus to MaxCorpusChars runes, so stop
	// accumulating once the builder is guaranteed to hold at least that many
	// (a rune is at most 4 bytes, so byte length ≥ 4×cap ⇒ rune count ≥ cap).
	// Parts keep streaming through; only the collection stops. This keeps
	// memory bounded on arbitrarily large inputs.
	corpusByteCap := 4 * t.opts.withDefaults().MaxCorpusChars
	var corpus strings.Builder
	for part := range in {
		if corpus.Len() < corpusByteCap && part.Type == model.PartBlock {
			if block, ok := part.Resource.(*model.Block); ok && block != nil {
				if text := strings.TrimSpace(block.SourceText()); text != "" {
					if corpus.Len() > 0 {
						corpus.WriteString("\n")
					}
					corpus.WriteString(text)
				}
			}
		}
		select {
		case out <- part:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	draft, evidence, usage, err := inferVoiceProfile(ctx, t.provider, corpus.String(), t.opts)
	if err != nil {
		if errors.Is(err, ErrEmptyCorpus) {
			return nil
		}
		return err
	}
	t.addUsage(usage)

	t.mu.Lock()
	t.draft, t.evidence = draft, evidence
	t.mu.Unlock()
	return nil
}

// InferFrom drafts a profile from a corpus given whole, outside the streaming
// pass.
//
// Process accumulates one file's blocks and infers from those, which is the
// right shape for a flow step and the wrong one for a run over a directory: a
// tool is built per file, so `kapi exec voice-infer docs/*.md` would infer once
// per document and produce a stack of partial profiles rather than the profile
// of the corpus. The exec path collects the text across every file and calls
// this once. See host/voiceinfer.go.
func (t *VoiceInferTool) InferFrom(ctx context.Context, corpus string) (*profile.VoiceProfile, *DraftEvidence, error) {
	draft, evidence, usage, err := inferVoiceProfile(ctx, t.provider, corpus, t.opts)
	if err != nil {
		return nil, nil, err
	}
	t.addUsage(usage)
	t.mu.Lock()
	t.draft, t.evidence = draft, evidence
	t.mu.Unlock()
	return draft, evidence, nil
}

// Draft returns the inferred draft profile and its evidence sidecar. Both are
// nil until Process has completed over a stream containing translatable text.
func (t *VoiceInferTool) Draft() (*profile.VoiceProfile, *DraftEvidence) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.draft, t.evidence
}
