package tools

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// RewriteConfig configures the rewrite tool. The provider plumbing matches the
// other AI tools so a recipe / CLI can pick a backend and key the same way.
type RewriteConfig struct {
	// Instruction is the plain-language description of how to rewrite the text
	// (e.g. "make it more concise", "use UK spelling", "rephrase for a 5th-grade
	// reading level").
	Instruction string `json:"instruction,omitempty" schema:"title=Instruction,description=Plain-language instruction describing how to rewrite the text"`
	Provider    string `json:"provider,omitempty"    schema:"title=AI Provider,description=AI provider,default=anthropic,group=provider"`
	APIKey      string `json:"apiKey,omitempty"      schema:"title=API Key,description=API key for the AI provider,group=provider"`
	Model       string `json:"model,omitempty"       schema:"title=Model,description=AI model name,group=provider"`
}

// RewriteSchema returns the auto-generated schema for the rewrite tool.
func RewriteSchema() *schema.ComponentSchema {
	s := schema.FromStruct(&RewriteConfig{}, schema.ToolMeta{
		ID:          "rewrite",
		Category:    schema.CategoryTextProcessing,
		DisplayName: "Rewrite",
		Description: "Rewrite the text inside a file following an instruction, preserving structure and inline codes",
		Tags:        []string{"ai-powered"},
		Cardinality: schema.Monolingual,
		Requires:    []string{schema.RequiresCredentials},
		SideEffects: []schema.SideEffect{schema.SideEffectAPICall, schema.SideEffectRemoteSourceEgress},
	})
	injectProviderOptions(s)
	return s
}

// NewRewriteFromConfig is the config-factory entry point.
func NewRewriteFromConfig(config map[string]any, _ string) (tool.Tool, error) {
	var cfg RewriteConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("rewrite config: %w", err)
	}
	p, err := ProviderFromConfig(cfg.Provider, aiprovider.Config{APIKey: cfg.APIKey, Model: cfg.Model})
	if err != nil {
		return nil, err
	}
	return NewRewriteTool(p, cfg), nil
}

// rewriteSystemPrompt builds the system prompt: the user's instruction plus the
// hard constraints that make the rewrite faithful — return only the text, and
// preserve every inline-code placeholder tag verbatim.
func rewriteSystemPrompt(instruction string) string {
	var b strings.Builder
	b.WriteString("Rewrite the user's text following this instruction: ")
	b.WriteString(strings.TrimSpace(instruction))
	b.WriteString(".\n\n")
	b.WriteString("Return ONLY the rewritten text, with no commentary, labels, or quotation. ")
	b.WriteString(`Preserve every placeholder tag (e.g. <x id="1"/>) exactly as it appears — `)
	b.WriteString("do not add, remove, reorder, or modify any tag.")
	return b.String()
}

// NewRewriteTool builds the rewrite tool: a source Transform that rewrites the
// human-readable text of every translatable Block with an LLM, following the
// configured instruction, while preserving the document's structure and the
// inline codes (placeholders, markup) around the change.
//
// This is the moat tool — "let your AI safely edit the content inside a file,
// faithfully, with a reviewable diff". It is a read-only edit producer (AD-006):
// it renders the block's runs as placeholder-tagged text, asks the model to
// rewrite that text, then returns the rewrite in an EditPlan. The framework
// applier is the sole mutator; the bespoke `kapi rewrite` command drives it
// through the byte-faithful round-trip path (editDocument).
//
// Returns a *tool.BaseTool so callers (the CLI command, MCP, flows) can drive it
// through the shared dispatch the same way ksed / search-replace are driven.
func NewRewriteTool(p aiprovider.LLMProvider, cfg RewriteConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "rewrite",
		ToolDescription: "Rewrites the source text inside a file following an instruction, preserving structure",
	}
	instruction := cfg.Instruction
	system := rewriteSystemPrompt(instruction)

	// Transform producer: returns the rewrite as an edit plan; the framework
	// applier rewrites the block (AD-006). Read-only — it never mutates v.
	t.Transform = func(v tool.BlockView) (tool.EditPlan, error) {
		var plan tool.EditPlan
		if !v.Translatable() {
			return plan, nil
		}

		oldRuns := v.SourceRuns()
		// Render inline codes as <x id="…"/> placeholders so the model sees flat
		// text it must echo with the tags intact.
		text := model.RunsPlaceholderText(oldRuns)
		if strings.TrimSpace(text) == "" {
			return plan, nil
		}

		resp, err := p.Chat(v.Context(), []aiprovider.Message{
			aiprovider.TextMessage("system", system),
			aiprovider.TextMessage("user", text),
		})
		if err != nil {
			return plan, fmt.Errorf("rewrite: %w", err)
		}
		newText := strings.TrimSpace(resp.Content)
		if newText == "" || newText == text {
			return plan, nil // nothing to do — model declined or returned the input
		}

		// Reconstruct the rewritten run sequence, matching each placeholder tag
		// back to its source run so inline codes survive the rewrite.
		newRuns := model.ParseRunsPlaceholderText(newText, oldRuns)

		if model.HasStructuredRuns(oldRuns) {
			// Plural/select runs have no linear text mapping, so a per-span edit
			// can't be derived: replace the whole source opaquely with the
			// rewritten plain text (the applier drops the stale source overlays).
			plain := model.RunsText(newRuns)
			plan.ReplaceAll = &plain
			return plan, nil
		}

		// Structured rewrite: keep the run / inline-code structure and describe
		// the change with a whole-span edit so the applier rebases overlays
		// (FullSpanEdit drops only the overlays overlapping the rewritten span —
		// a rewritten block's term/entity/segmentation overlays are stale).
		plan.NewRuns = newRuns
		plan.Edits = tool.FullSpanEdit(oldRuns, newRuns)
		return plan, nil
	}
	return t
}
