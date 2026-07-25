package tools

import (
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

func withParallelBlocks(n int) func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.DefaultParallelBlocks = n }
}

// Keep withParallelBlocks reachable until the first tool adopts it.
// Without this anchor, linters flag the helper as unused even though
// it's part of the builder chain that sits next to withWritesOutput
// / withSideEffects / withAliases.
var _ = withParallelBlocks

func withAliases(aliases ...string) func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.Aliases = aliases }
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
		return NewQACheckTool(NewQACheckConfig(model.LocaleEnglish))
	}, toolSchema(NewQACheckConfig(model.LocaleEnglish), toolMeta("qa", "Quality Check", schema.CategoryQuality,
		withTags("quality", schema.TagL10n), withWritesOutput(), withRequires("target-language"), withCardinality(schema.Bilingual), withConsumes(tgtF(schema.PortTarget)), withProduces(tgtF(model.OverlayQA)))))

	reg.RegisterWithSchema("dnt-check", func() tool.Tool {
		return NewDNTCheckTool(NewDNTCheckConfig(model.LocaleEnglish))
	}, toolSchema(NewDNTCheckConfig(model.LocaleEnglish), toolMeta("dnt-check", "Do-Not-Translate Check", schema.CategoryQuality,
		withTags("quality", schema.TagL10n), withAliases("dnt"), withRequires("target-language"), withCardinality(schema.Bilingual), withConsumes(tgtF(schema.PortTarget)), withProduces(tgtF(model.OverlayQA)))))

	reg.RegisterWithSchema("placeholder-check", func() tool.Tool {
		return NewPlaceholderCheckTool(NewPlaceholderCheckConfig(model.LocaleEnglish))
	}, toolSchema(NewPlaceholderCheckConfig(model.LocaleEnglish), toolMeta("placeholder-check", "Placeholder Check", schema.CategoryQuality,
		withTags("quality", schema.TagL10n), withRequires("target-language"), withCardinality(schema.Bilingual), withConsumes(tgtF(schema.PortTarget)), withProduces(tgtF(model.OverlayQA)))))

	reg.RegisterWithSchema("term-check", func() tool.Tool {
		return NewTermCheckTool(&TermCheckConfig{TargetLocale: model.LocaleEnglish})
	}, toolSchema(&TermCheckConfig{}, toolMeta("term-check", "Terminology Check", schema.CategoryQuality,
		withTags("quality", schema.TagL10n), withRequires("target-language", "termbase"), withCardinality(schema.Bilingual), withConsumes(tgtF(schema.PortTarget)), withProduces(srcF(model.OverlayTerm)), withSideEffects(schema.SideEffectTermbaseRead))))

	reg.RegisterWithSchema("xml-validation", func() tool.Tool {
		return NewXMLValidationTool(&XMLValidationConfig{CheckSource: true, WrapRoot: true})
	}, toolSchema(&XMLValidationConfig{CheckSource: true, WrapRoot: true}, toolMeta("xml-validation", "XML Validation", schema.CategoryQuality,
		withTags("quality"), withCardinality(schema.Monolingual), withProduces(tgtF(model.OverlayQA)))))

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

	reg.RegisterWithSchema("create-target", func() tool.Tool {
		return NewCreateTargetTool(&CreateTargetConfig{CreateOnNonTranslatable: true})
	}, toolSchema(&CreateTargetConfig{CreateOnNonTranslatable: true}, toolMeta("create-target", "Create Target", schema.CategoryTextProcessing,
		withTags(schema.TagL10n), withRequires("target-language"), withCardinality(schema.Bilingual))))

	reg.RegisterWithSchema("remove-target", func() tool.Tool {
		return NewRemoveTargetTool(&RemoveTargetConfig{FilterByIDs: true})
	}, toolSchema(&RemoveTargetConfig{FilterByIDs: true}, toolMeta("remove-target", "Remove Target", schema.CategoryTextProcessing,
		withTags(schema.TagL10n), withRequires("target-language"), withCardinality(schema.Bilingual))))

	reg.RegisterWithSchema("inline-codes-remove", func() tool.Tool {
		return NewInlineCodesRemoveTool(&InlineCodesRemoveConfig{ApplyTarget: true, IncludeNonTranslatable: true})
	}, toolSchema(&InlineCodesRemoveConfig{ApplyTarget: true, IncludeNonTranslatable: true}, toolMeta("inline-codes-remove", "Inline Codes Remove", schema.CategoryTextProcessing,
		withTags("text-processing"), withCardinality(schema.Monolingual))))

	reg.RegisterWithSchema("properties-set", func() tool.Tool {
		return NewPropertiesSetTool(&PropertiesSetConfig{Overwrite: true, OnlyTranslatable: true})
	}, toolSchema(&PropertiesSetConfig{Overwrite: true, OnlyTranslatable: true}, toolMeta("properties-set", "Properties Set", schema.CategoryTextProcessing,
		withTags("configurable"), withCardinality(schema.Monolingual))))

	// withWritesOutput is what gives `kapi exec whitespace-correct` its -o /
	// --output-dir flags. The tool rewrites the target, so without it the exec
	// run had nowhere to put the result: it corrected the content in memory,
	// exited 0, and wrote nothing.
	reg.RegisterWithSchema("whitespace-correct", func() tool.Tool {
		return NewWhitespaceCorrectTool(NewWhitespaceCorrectConfig(model.LocaleEnglish))
	}, toolSchema(&WhitespaceCorrectConfig{NormalizeSpaces: true, MatchSourceWhitespace: true, RemoveZeroWidthChars: true, CorrectFullStop: true, CorrectComma: true, CorrectExclamation: true, CorrectQuestion: true, IncludeVerticalWS: true, IncludeHorizontalWS: true},
		toolMeta("whitespace-correct", "Whitespace Correct", schema.CategoryTextProcessing,
			withTags("text-processing", schema.TagL10n), withWritesOutput(), withRequires("target-language"), withCardinality(schema.Bilingual))))

	reg.RegisterWithSchema("tag-protect", func() tool.Tool {
		return NewTagProtectTool(&TagProtectConfig{})
	}, toolSchema(&TagProtectConfig{}, toolMeta("tag-protect", "Tag Protect", schema.CategoryTextProcessing,
		withTags("regex", "configurable"), withCardinality(schema.Monolingual))))

	reg.RegisterWithSchema("redact", func() tool.Tool {
		t, _ := NewRedactTool(&RedactConfig{Detectors: []string{DetectRules}})
		return t
	}, RedactSchema())

	reg.RegisterWithSchema("unredact", func() tool.Tool {
		t, _ := NewUnredactTool(&UnredactConfig{})
		return t
	}, UnredactSchema())

	// ── Enrich ──────────────────────────────────────────────────────

	// "recycle" is the canonical id for TM leverage (pre-fill from translation
	// memory).
	reg.RegisterWithSchema("recycle", func() tool.Tool {
		return NewTMLeverageTool(&TMLeverageConfig{FuzzyThreshold: 70, Provider: NullTMProvider{}})
	}, toolSchema(&TMLeverageConfig{FuzzyThreshold: 70}, toolMeta("recycle", "Recycle", schema.CategoryTranslation,
		withTags("translation", schema.TagL10n), withWritesOutput(), withRequires("target-language", "tm"), withCardinality(schema.Bilingual), withConsumes(optF(srcF(model.OverlaySegmentation))), withProduces(srcF(model.AnnoTMMatch), srcF(model.AnnoAltTranslation), tgtF(schema.PortTarget)), withSideEffects(schema.SideEffectTMRead))))

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

	reg.RegisterWithSchema("span-classify", func() tool.Tool {
		return NewSpanClassifyTool(&SpanClassifyConfig{})
	}, toolSchema(&SpanClassifyConfig{}, toolMeta("span-classify", "Span Classify", schema.CategoryTextProcessing,
		withTags("text-processing"), withCardinality(schema.Monolingual))))

	reg.RegisterWithSchema("layer-processor", func() tool.Tool {
		return NewLayerProcessorTool(&LayerProcessorConfig{})
	}, &schema.ComponentSchema{ToolMeta: &schema.ToolMeta{
		ID: "layer-processor", DisplayName: "Layer Processor", Category: schema.CategoryTextProcessing,
		Cardinality: schema.Monolingual,
	}})

	reg.RegisterWithSchema("external-command", func() tool.Tool {
		return NewExternalCommandTool(&ExternalCommandConfig{ApplyTarget: true, SendAsStdin: true, Timeout: 30})
	}, toolSchema(&ExternalCommandConfig{ApplyTarget: true, SendAsStdin: true, Timeout: 30},
		toolMeta("external-command", "External Command", schema.CategoryTextProcessing,
			withTags("configurable"), withCardinality(schema.Monolingual))))

	reg.RegisterWithSchema("brand-vocab-check", func() tool.Tool {
		return NewBrandVocabCheckTool(nil, nil)
	}, &schema.ComponentSchema{ToolMeta: &schema.ToolMeta{
		ID: "brand-vocab-check", DisplayName: "Brand Vocabulary Check", Category: schema.CategoryQuality,
		Cardinality: schema.Bilingual,
		Consumes:    []schema.IOPort{{Type: schema.PortTarget, Side: model.SideTarget}},
		Produces:    []schema.IOPort{{Type: model.AnnoBrandVoice, Side: model.SideTarget}},
		Requires:    []string{"target-language"},
	}})

	// ── Utility ─────────────────────────────────────────────────────

	reg.RegisterWithSchema("batch", func() tool.Tool {
		return NewBatchTool(&BatchConfig{Size: 10})
	}, toolSchema(&BatchConfig{Size: 10}, toolMeta("batch", "Batch Collector", schema.CategoryTextProcessing,
		withTags("batch"), withCardinality(schema.Monolingual))))

	reg.RegisterWithSchema("script", func() tool.Tool {
		return NewScriptTool(&ScriptConfig{})
	}, toolSchema(&ScriptConfig{}, toolMeta("script", "Script", schema.CategoryTextProcessing,
		withTags("configurable"), withWritesOutput(), withCardinality(schema.Monolingual))))

	// Register config factories for all tools that support NewToolFromConfig.
	// This enables project flows to create tools with step-level config.
	registerConfigFactories(reg)
}

func registerConfigFactories(reg *registry.ToolRegistry) {
	reg.SetConfigFactory("qa", NewQACheckFromConfig)
	reg.SetConfigFactory("term-check", NewTermCheckFromConfig)
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
	// segmentation's ConfigFactory is set by RegisterGroup (it's a ToolGroup).
	reg.SetConfigFactory("source-gate", NewSourceGateFromConfig)
	reg.SetConfigFactory("recycle", NewTMLeverageFromConfig)
	reg.SetConfigFactory("diff-leverage", NewDiffLeverageFromConfig)
	reg.SetConfigFactory("script", NewScriptFromConfig)
}
