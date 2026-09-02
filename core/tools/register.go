package tools

import (
	corememory "github.com/neokapi/neokapi/core/memory"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// toolMeta creates a ToolMeta with common tool fields.
func toolMeta(id, displayName, category string, opts ...func(*schema.ToolMeta)) schema.ToolMeta {
	m := schema.ToolMeta{
		ID:          id,
		Category:    category,
		DisplayName: displayName,
	}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

func withTags(tags ...string) func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.Tags = tags }
}

// IO-contract shorthands for tool registration. Generic over ~string so overlay
// types, annotation keys, and pseudo-port constants all pass without string().
func srcF[T ~string](t T) schema.IOPort  { return schema.Port(t, model.SideSource) }
func tgtF[T ~string](t T) schema.IOPort  { return schema.Port(t, model.SideTarget) }
func optF(f schema.IOPort) schema.IOPort { f.Optional = true; return f }

func withConsumes(fs ...schema.IOPort) func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.Consumes = fs }
}

func withRequires(reqs ...string) func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.Requires = reqs }
}

func withCardinality(c schema.LocaleCardinality) func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.Cardinality = c }
}

func withDefaultLocale(locale model.LocaleID) func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.DefaultLocale = locale }
}

func withProduces(fs ...schema.IOPort) func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.Produces = fs }
}

func withSideEffects(effects ...schema.SideEffect) func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.SideEffects = effects }
}

func withWritesOutput() func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.WritesOutput = true }
}

// No withParallelBlocks builder: the tools that want a parallel-block default
// are in core/ai/tools and set schema.ToolMeta.DefaultParallelBlocks directly
// in their meta literals. A builder existed here for the tools registered
// through this file, kept alive by a `var _ =` anchor "until the first tool
// adopts it" — none did, and the anchor is precisely the mechanism that stops
// the compiler from ever saying so. Add it back when a tool in this file needs
// it, not before.

func withAliases(aliases ...string) func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.Aliases = aliases }
}

// withInternal withholds a tool from `kapi exec` / `kapi tools list` / the MCP
// surface. It says nothing about configurability — an internal tool still needs
// a ConfigFactory if it declares settable schema fields, because a flow may name
// it as a step. See schema.ToolMeta.Internal for why the two are separate.
func withInternal() func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.Internal = true }
}

// toolSchema is a shorthand for generating a tool schema from a config struct.
func toolSchema(cfg any, meta schema.ToolMeta) *schema.ComponentSchema {
	return schema.FromStruct(cfg, meta)
}

// RegisterAll registers all built-in tools in the given ToolRegistry.
// Each registration includes a factory and an auto-generated parameter schema.
func RegisterAll(reg *registry.ToolRegistry) {

	// ── Validate ────────────────────────────────────────────────────

	reg.RegisterWithSchema("qa", func() tool.Tool {
		return NewRuleCheckTool(NewRuleCheckConfig(model.LocaleEnglish))
	}, toolSchema(NewRuleCheckConfig(model.LocaleEnglish), toolMeta("qa", "Quality Check", schema.CategoryQuality,
		withTags("quality", schema.TagL10n), withWritesOutput(), withRequires("target-language"), withCardinality(schema.Bilingual), withConsumes(tgtF(schema.PortTarget)), withProduces(tgtF(model.OverlayCheck)))))

	reg.RegisterWithSchema("dnt-check", func() tool.Tool {
		return NewDNTCheckTool(NewDNTCheckConfig(model.LocaleEnglish))
	}, toolSchema(NewDNTCheckConfig(model.LocaleEnglish), toolMeta("dnt-check", "Do-Not-Translate Check", schema.CategoryQuality,
		withTags("quality", schema.TagL10n), withAliases("dnt"), withRequires("target-language"), withCardinality(schema.Bilingual), withConsumes(tgtF(schema.PortTarget)), withProduces(tgtF(model.OverlayCheck)))))

	reg.RegisterWithSchema("placeholder-check", func() tool.Tool {
		return NewPlaceholderCheckTool(NewPlaceholderCheckConfig(model.LocaleEnglish))
	}, toolSchema(NewPlaceholderCheckConfig(model.LocaleEnglish), toolMeta("placeholder-check", "Placeholder Check", schema.CategoryQuality,
		withTags("quality", schema.TagL10n), withRequires("target-language"), withCardinality(schema.Bilingual), withConsumes(tgtF(schema.PortTarget)), withProduces(tgtF(model.OverlayCheck)))))

	reg.RegisterWithSchema("term-check", func() tool.Tool {
		return NewTermCheckTool(&TermCheckConfig{TargetLocale: model.LocaleEnglish})
	}, toolSchema(&TermCheckConfig{}, toolMeta("term-check", "Terminology Check", schema.CategoryQuality,
		withTags("quality", schema.TagL10n), withRequires("target-language", schema.RequiresTerms), withCardinality(schema.Bilingual), withConsumes(tgtF(schema.PortTarget)), withProduces(srcF(model.OverlayTerm)), withSideEffects(schema.SideEffectTermsRead))))

	reg.RegisterWithSchema("xml-validation", func() tool.Tool {
		return NewXMLValidationTool(NewXMLValidationConfig(""))
	}, toolSchema(NewXMLValidationConfig(""), toolMeta("xml-validation", "XML Validation", schema.CategoryQuality,
		withTags("quality"), withCardinality(schema.Monolingual), withProduces(tgtF(model.OverlayCheck)))))

	// ── Transform ───────────────────────────────────────────────────

	reg.RegisterWithSchema("pseudo-translate", func() tool.Tool {
		return NewPseudoTranslateTool(&PseudoConfig{Prefix: "\u2592 ", Suffix: " \u2592", TargetLocale: "qps"})
	}, toolSchema(&PseudoConfig{Prefix: "\u2592 ", Suffix: " \u2592"}, toolMeta("pseudo-translate", "Pseudo Translate", schema.CategoryTranslation,
		withTags("translation", schema.TagL10n), withAliases("pseudo"), withWritesOutput(), withRequires("target-language"), withCardinality(schema.Bilingual), withDefaultLocale(model.LocaleID("qps")), withProduces(tgtF(schema.PortTarget)))))

	reg.RegisterWithSchema("search-replace", func() tool.Tool {
		return NewSearchReplaceTool(&SearchReplaceConfig{})
	}, toolSchema(&SearchReplaceConfig{}, toolMeta("search-replace", "Search and Replace", schema.CategoryTextProcessing,
		withTags("regex", "configurable"), withWritesOutput(), withCardinality(schema.Monolingual))))

	reg.RegisterWithSchema("case-transform", func() tool.Tool {
		return NewCaseTransformTool(&CaseTransformConfig{Mode: CaseLower, ApplySource: true})
	}, toolSchema(&CaseTransformConfig{Mode: CaseLower, ApplySource: true}, toolMeta("case-transform", "Case Transform", schema.CategoryTextProcessing,
		withTags("text-processing"), withWritesOutput(), withCardinality(schema.Monolingual))))

	// source-gate is the leading source-transform stage of source-first
	// convergence (epic 019): it settles the source authoring status and holds
	// blocks below the configured source gate so downstream producers skip an
	// un-settled source. Monolingual (it reasons about the source only) and a
	// source-content transformer, so it sits at the head of a flow's leading
	// source-transform stage.
	reg.RegisterWithSchema("source-gate", func() tool.Tool {
		return NewSourceGateTool(model.DefaultSourceGate)
	}, toolSchema(&SourceGateConfig{Gate: string(model.DefaultSourceGate)}, toolMeta("source-gate", "Source Gate", schema.CategoryTranslation,
		withTags(schema.TagL10n), withCardinality(schema.Monolingual))))

	RegisterSegmentation(reg)

	// create-target / remove-target write and clear the target container, so
	// they declare WritesOutput: `kapi exec` grows -o / --output-dir only for
	// tools that do, and without it the exec run mutates the content in memory,
	// exits 0, and writes nothing (#1471 §3).
	reg.RegisterWithSchema("create-target", func() tool.Tool {
		return NewCreateTargetTool(NewCreateTargetConfig(""))
	}, toolSchema(NewCreateTargetConfig(""), toolMeta("create-target", "Create Target", schema.CategoryTextProcessing,
		withTags(schema.TagL10n), withWritesOutput(), withRequires("target-language"), withCardinality(schema.Bilingual))))

	reg.RegisterWithSchema("remove-target", func() tool.Tool {
		return NewRemoveTargetTool(NewRemoveTargetConfig(""))
	}, toolSchema(NewRemoveTargetConfig(""), toolMeta("remove-target", "Remove Target", schema.CategoryTextProcessing,
		withTags(schema.TagL10n), withWritesOutput(), withRequires("target-language"), withCardinality(schema.Bilingual))))

	reg.RegisterWithSchema("inline-codes-remove", func() tool.Tool {
		return NewInlineCodesRemoveTool(NewInlineCodesRemoveConfig(""))
	}, toolSchema(NewInlineCodesRemoveConfig(""), toolMeta("inline-codes-remove", "Inline Codes Remove", schema.CategoryTextProcessing,
		withTags("text-processing"), withWritesOutput(), withCardinality(schema.Monolingual))))

	// properties-set writes arbitrary block properties. No writer serializes
	// them — they exist for the steps downstream in the same flow — so it is a
	// flow-internal step, not an `exec` command that could show a result.
	reg.RegisterWithSchema("properties-set", func() tool.Tool {
		return NewPropertiesSetTool(NewPropertiesSetConfig())
	}, toolSchema(NewPropertiesSetConfig(), toolMeta("properties-set", "Properties Set", schema.CategoryTextProcessing,
		withTags("configurable"), withCardinality(schema.Monolingual), withInternal())))

	// withWritesOutput is what gives `kapi exec whitespace-correct` its -o /
	// --output-dir flags. The tool rewrites the target, so without it the exec
	// run had nowhere to put the result: it corrected the content in memory,
	// exited 0, and wrote nothing.
	reg.RegisterWithSchema("whitespace-correct", func() tool.Tool {
		return NewWhitespaceCorrectTool(NewWhitespaceCorrectConfig(model.LocaleEnglish))
	}, toolSchema(&WhitespaceCorrectConfig{NormalizeSpaces: true, MatchSourceWhitespace: true, RemoveZeroWidthChars: true, CorrectFullStop: true, CorrectComma: true, CorrectExclamation: true, CorrectQuestion: true, IncludeVerticalWS: true, IncludeHorizontalWS: true},
		toolMeta("whitespace-correct", "Whitespace Correct", schema.CategoryTextProcessing,
			withTags("text-processing", schema.TagL10n), withWritesOutput(), withRequires("target-language"), withCardinality(schema.Bilingual))))

	// tag-protect marks spans for the steps downstream (an MT connector that must
	// keep them). Like properties-set, its whole output is a block annotation no
	// writer serializes, so it is a flow-internal step.
	reg.RegisterWithSchema("tag-protect", func() tool.Tool {
		return NewTagProtectTool(&TagProtectConfig{})
	}, toolSchema(&TagProtectConfig{}, toolMeta("tag-protect", "Tag Protect", schema.CategoryTextProcessing,
		withTags("regex", "configurable"), withCardinality(schema.Monolingual), withInternal())))

	reg.RegisterWithSchema("redact", func() tool.Tool {
		t, _ := NewRedactTool(&RedactConfig{Detectors: []string{DetectRules}})
		return t
	}, RedactSchema())

	reg.RegisterWithSchema("unredact", func() tool.Tool {
		t, _ := NewUnredactTool(&UnredactConfig{})
		return t
	}, UnredactSchema())

	// ── Enrich ──────────────────────────────────────────────────────

	// "recycle" is the canonical id for content-memory leverage (pre-fill from translation
	// memory).
	reg.RegisterWithSchema("recycle", func() tool.Tool {
		return NewMemoryLeverageTool(&MemoryLeverageConfig{FuzzyThreshold: 70, Memory: corememory.NullProvider{}})
	}, toolSchema(&MemoryLeverageConfig{FuzzyThreshold: 70}, toolMeta("recycle", "Recycle", schema.CategoryTranslation,
		withTags("translation", schema.TagL10n), withWritesOutput(), withRequires("target-language", schema.RequiresMemory), withCardinality(schema.Bilingual), withConsumes(optF(srcF(model.OverlaySegmentation))), withProduces(srcF(model.AnnoMemoryMatch), srcF(model.AnnoAltTranslation), tgtF(schema.PortTarget)), withSideEffects(schema.SideEffectMemoryRead))))

	reg.RegisterWithSchema("diff-leverage", func() tool.Tool {
		return NewDiffLeverageTool(&DiffLeverageConfig{CaseSensitive: true, PreviousTexts: map[string]PreviousBlock{}})
	}, toolSchema(&DiffLeverageConfig{CaseSensitive: true}, toolMeta("diff-leverage", "Diff Leverage", schema.CategoryTranslation,
		withTags("translation", schema.TagL10n), withWritesOutput(), withCardinality(schema.Bilingual), withProduces(srcF(model.AnnoAltTranslation), tgtF(schema.PortTarget)))))

	// ── Convert ─────────────────────────────────────────────────────

	reg.RegisterWithSchema("encoding-detect", func() tool.Tool {
		return NewEncodingDetectTool(&EncodingDetectConfig{})
	}, toolSchema(&EncodingDetectConfig{}, toolMeta("encoding-detect", "Encoding Detect", schema.CategoryAnalysis,
		withCardinality(schema.Monolingual))))

	// ── Pipeline ────────────────────────────────────────────────────

	// span-classify normalizes inline-code types after a bridge reader. It has no
	// settable parameters at all (SpanClassifyConfig is an empty struct), so it
	// needs no config factory — and it is pipeline plumbing, so it is internal.
	reg.RegisterWithSchema("span-classify", func() tool.Tool {
		return NewSpanClassifyTool(&SpanClassifyConfig{})
	}, toolSchema(&SpanClassifyConfig{}, toolMeta("span-classify", "Span Classify", schema.CategoryTextProcessing,
		withTags("text-processing"), withCardinality(schema.Monolingual), withInternal())))

	// layer-processor's only parameter is a map of format → []tool.Tool, which no
	// YAML can express (its own ApplyMap refuses), so it has no schema properties
	// and no config factory. Programmatic, and internal.
	reg.RegisterWithSchema("layer-processor", func() tool.Tool {
		return NewLayerProcessorTool(&LayerProcessorConfig{})
	}, &schema.ComponentSchema{ToolMeta: &schema.ToolMeta{
		ID: "layer-processor", DisplayName: "Layer Processor", Category: schema.CategoryTextProcessing,
		Cardinality: schema.Monolingual,
		Internal:    true,
	}})

	// external-command rewrites the text with the output of the program it runs,
	// so it declares WritesOutput.
	reg.RegisterWithSchema("external-command", func() tool.Tool {
		return NewExternalCommandTool(NewExternalCommandConfig(""))
	}, toolSchema(NewExternalCommandConfig(""),
		toolMeta("external-command", "External Command", schema.CategoryTextProcessing,
			withTags("configurable"), withWritesOutput(), withCardinality(schema.Monolingual))))

	// voice-vocab-check's input is a voice profile the host resolves from
	// the project/starter pack, never from step config — hence no schema properties
	// and no config factory. A recipe names it as a step (`kapi init` scaffolds
	// exactly that); the standalone command is `kapi check --profile`.
	//
	// Monolingual, and it must stay that way: the tool reads the block's SOURCE
	// text (annotateBlock → v.SourceText()) and scores it against the voice
	// vocabulary, exactly as its LLM sibling voice-check does. A bilingual
	// declaration makes flow.ResolveFlowLocales yield no pass at all on a
	// project with no target languages — which is precisely the project
	// `kapi init` scaffolds around this step.
	reg.RegisterWithSchema("voice-vocab-check", func() tool.Tool {
		return NewVoiceVocabCheckTool(nil, nil)
	}, &schema.ComponentSchema{ToolMeta: &schema.ToolMeta{
		ID: "voice-vocab-check", DisplayName: "Voice Vocabulary Check", Category: schema.CategoryQuality,
		Cardinality: schema.Monolingual,
		Produces:    []schema.IOPort{{Type: model.AnnoVoice, Side: model.SideTarget}},
		Internal:    true,
	}})

	// ── Utility ─────────────────────────────────────────────────────

	// batch regroups parts for a downstream batching tool — plumbing, so internal,
	// but its size is still a step's to choose (see NewBatchFromConfig).
	reg.RegisterWithSchema("batch", func() tool.Tool {
		return NewBatchTool(&BatchConfig{Size: 10})
	}, toolSchema(&BatchConfig{Size: 10}, toolMeta("batch", "Batch Collector", schema.CategoryTextProcessing,
		withTags("batch"), withCardinality(schema.Monolingual), withInternal())))

	reg.RegisterWithSchema("script", func() tool.Tool {
		return NewScriptTool(&ScriptConfig{})
	}, toolSchema(&ScriptConfig{}, toolMeta("script", "Script", schema.CategoryTextProcessing,
		withTags("configurable"), withWritesOutput(), withCardinality(schema.Monolingual))))

	// Register config factories for all tools that support NewToolFromConfig.
	// This enables project flows to create tools with step-level config.
	registerConfigFactories(reg)
}

// registerConfigFactories attaches the config-map factory for every built-in
// tool that declares settable schema fields.
//
// This list is not optional decoration. ToolRegistry.NewToolWithConfig applies a
// step's `config:` map only through a ConfigFactory; with just the zero-arg
// Factory it calls that and throws the map away without a word, and any locale
// the zero-arg factory hardcoded stands whatever the run's --target-lang says.
// A missing entry here also removes the tool from `kapi tools list`,
// `kapi exec` and the MCP surface, because registry.CLITools requires a config
// factory. Thirteen tools sat in that state (#1476).
//
// TestEveryConfigurableBuiltInToolHasAConfigFactory asserts this list is
// complete over the populated registry, so the omission cannot recur silently.
func registerConfigFactories(reg *registry.ToolRegistry) {
	reg.SetConfigFactory("qa", NewRuleCheckFromConfig)
	reg.SetConfigFactory("dnt-check", NewDNTCheckFromConfig)
	reg.SetConfigFactory("placeholder-check", NewPlaceholderCheckFromConfig)
	reg.SetConfigFactory("term-check", NewTermCheckFromConfig)
	reg.SetConfigFactory("xml-validation", NewXMLValidationFromConfig)
	reg.SetConfigFactory("encoding-detect", NewEncodingDetectFromConfig)
	reg.SetConfigFactory("pseudo-translate", NewPseudoTranslateFromConfig)
	reg.SetConfigFactory("redact", NewRedactFromConfig)
	// Entity detection makes the upstream entity overlay a required input (so a
	// misconfigured flow fails fast instead of silently leaving PII unredacted).
	reg.SetContractResolver("redact", ResolveRedactContract)
	reg.SetConfigFactory("unredact", NewUnredactFromConfig)
	reg.SetConfigFactory("search-replace", NewSearchReplaceFromConfig)
	reg.SetConfigFactory("case-transform", NewCaseTransformFromConfig)
	reg.SetConfigFactory("whitespace-correct", NewWhitespaceCorrectFromConfig)
	reg.SetConfigFactory("create-target", NewCreateTargetFromConfig)
	reg.SetConfigFactory("remove-target", NewRemoveTargetFromConfig)
	reg.SetConfigFactory("inline-codes-remove", NewInlineCodesRemoveFromConfig)
	reg.SetConfigFactory("tag-protect", NewTagProtectFromConfig)
	reg.SetConfigFactory("external-command", NewExternalCommandFromConfig)
	// segmentation's ConfigFactory is set by RegisterGroup (it's a ToolGroup).
	reg.SetConfigFactory("source-gate", NewSourceGateFromConfig)
	reg.SetConfigFactory("recycle", NewMemoryLeverageFromConfig)
	reg.SetConfigFactory("diff-leverage", NewDiffLeverageFromConfig)
	reg.SetConfigFactory("script", NewScriptFromConfig)
	// Internal tools (withInternal) still need their step config applied — being
	// withheld from the CLI is not the same as being unconfigurable.
	reg.SetConfigFactory("properties-set", NewPropertiesSetFromConfig)
	reg.SetConfigFactory("batch", NewBatchFromConfig)
}
