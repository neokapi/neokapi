package tools_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
)

// Per-tool proof that each newly-registered config factory is load-bearing.
//
// Every test here builds the tool through registry.NewToolWithConfig — the exact
// path a flow step takes — and asserts on what the tool *did*, not on the config
// struct. A factory that reads a step's map into a field nothing consumes is the
// same defect one layer in, so "the config arrived" is not the claim; "the
// behaviour changed" is. Where a knob has a default, it is exercised against
// that default in both directions, so "config applied" and "config dropped, so
// the defaults ran" cannot produce the same answer.

// stepTool builds a tool exactly as a flow step does.
func stepTool(t *testing.T, name registry.ToolID, config map[string]any, targetLang string) tool.Tool {
	t.Helper()
	reg := registry.NewToolRegistry()
	tools.RegisterAll(reg)
	tl, err := reg.NewToolWithConfig(name, config, targetLang)
	require.NoError(t, err)
	require.NotNil(t, tl)
	return tl
}

// stepBlock is a bilingual block: an `nb` target for an `en` source.
func stepBlock(source, target string) *model.Block {
	b := &model.Block{ID: "u1", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: source}}}}
	tool.NewVariantView(b).SetTargetText(model.LocaleID("nb"), target)
	return b
}

// runStep sends one block through a registry-built tool and returns the block as
// the pipeline left it.
func runStep(t *testing.T, name registry.ToolID, config map[string]any, targetLang string, b *model.Block) *model.Block {
	t.Helper()
	tl := stepTool(t, name, config, targetLang)
	out := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: b})
	blk, ok := out.Resource.(*model.Block)
	require.True(t, ok)
	return blk
}

// findings reads the unified check annotation off a block.
func findings(b *model.Block) []check.Finding {
	ann, ok := model.AnnoAs[*check.FindingsAnnotation](b, check.AnnotationKey)
	if !ok {
		return nil
	}
	return ann.Findings
}

// ── dnt-check ───────────────────────────────────────────────────────────────

// The headline case. dnt-check's entire behaviour is its configured Terms list,
// and Terms can only arrive through the config map — so with no config factory
// the tool ran with no terms, on an `en` target locale, and found nothing to
// report. A do-not-translate guardrail that cannot fail is worse than none,
// because a green run is read as "the product name survived".
func TestDNTCheck_StepConfigMakesTheGuardrailCapableOfFailing(t *testing.T) {
	b := runStep(t, "dnt-check", map[string]any{
		"terms": []any{"Acme Cloud"},
	}, "nb", stepBlock("Save now with Acme Cloud", "Lagre nå med Toppskyen"))

	f := findings(b)
	require.Len(t, f, 1, "the translated do-not-translate term must be reported")
	assert.Equal(t, "do-not-translate", f[0].Category)
	assert.Equal(t, check.SeverityCritical, f[0].Severity)
	assert.Equal(t, "Acme Cloud", f[0].OriginalText)
	assert.Contains(t, f[0].Message, "nb", "the finding names the locale the run asked for")
}

// The same term, preserved verbatim, must still pass — otherwise the test above
// would be satisfied by a check that flags everything.
func TestDNTCheck_StepConfigStillPassesAPreservedTerm(t *testing.T) {
	b := runStep(t, "dnt-check", map[string]any{
		"terms": []any{"Acme Cloud"},
	}, "nb", stepBlock("Save now with Acme Cloud", "Lagre nå med Acme Cloud"))
	assert.Empty(t, findings(b), "a term that survived verbatim is not a finding")
}

// The locale comes from the run: the zero-arg factory pinned model.LocaleEnglish,
// so a tool built for an `nb` run never looked at the `nb` target at all.
func TestDNTCheck_LocaleComesFromTheRunNotTheFactory(t *testing.T) {
	tl := stepTool(t, "dnt-check", map[string]any{"terms": []any{"Acme Cloud"}}, "nb")
	cfg, ok := tl.(*tool.BaseTool).Cfg.(*tools.DNTCheckConfig)
	require.True(t, ok)
	assert.Equal(t, model.LocaleID("nb"), cfg.TargetLocale)
	assert.Equal(t, []string{"Acme Cloud"}, cfg.Terms)

	// A locale written into the step config must not outrank the run.
	tl = stepTool(t, "dnt-check", map[string]any{"targetLocale": "de"}, "nb")
	cfg, ok = tl.(*tool.BaseTool).Cfg.(*tools.DNTCheckConfig)
	require.True(t, ok)
	assert.Equal(t, model.LocaleID("nb"), cfg.TargetLocale)
}

// caseInsensitive is the tool's one schema-visible knob, and it flips the verdict
// in both directions — so the assertion cannot pass on defaults alone.
func TestDNTCheck_StepConfigCaseInsensitiveFlipsTheVerdict(t *testing.T) {
	strict := runStep(t, "dnt-check", map[string]any{
		"terms": []any{"iPhone"},
	}, "nb", stepBlock("Use iPhone now", "Bruk Iphone nå"))
	require.Len(t, findings(strict), 1, "exact case is required by default")

	relaxed := runStep(t, "dnt-check", map[string]any{
		"terms":           []any{"iPhone"},
		"caseInsensitive": true,
	}, "nb", stepBlock("Use iPhone now", "Bruk Iphone nå"))
	assert.Empty(t, findings(relaxed), "the step turned case-insensitive preservation on")
}

// ── placeholder-check ───────────────────────────────────────────────────────

// #1456 shipped a Norwegian digest email without its settings link because the
// recycle path dropped the placeholders. placeholder-check is the check meant to
// catch that class, and from a flow it was pinned to `en`.
func TestPlaceholderCheck_StepRunCatchesADroppedPlaceholder(t *testing.T) {
	b := runStep(t, "placeholder-check", nil, "nb",
		stepBlock("You have {count} items", "Du har varer"))

	f := findings(b)
	require.Len(t, f, 1)
	assert.Equal(t, "placeholder", f[0].Category)
	assert.Equal(t, check.SeverityCritical, f[0].Severity)
	assert.Contains(t, f[0].Message, "{count}")
	assert.Contains(t, f[0].Message, "nb", "the locale checked is the run's")
}

// flagExtra defaults to true and is the one knob a step can turn off.
func TestPlaceholderCheck_StepConfigFlagExtra(t *testing.T) {
	on := runStep(t, "placeholder-check", nil, "nb",
		stepBlock("You have items", "Du har {count} varer"))
	require.Len(t, findings(on), 1, "an extra placeholder is reported by default")
	assert.Equal(t, check.SeverityMajor, findings(on)[0].Severity)

	off := runStep(t, "placeholder-check", map[string]any{"flagExtra": false}, "nb",
		stepBlock("You have items", "Du har {count} varer"))
	assert.Empty(t, findings(off), "the step turned extra-placeholder reporting off")
}

// ── xml-validation ──────────────────────────────────────────────────────────

// checkTarget is off by default and needs a locale, so a step that turns it on
// was doing nothing at all: both halves of the config were dropped.
func TestXMLValidation_StepConfigChecksTheTarget(t *testing.T) {
	b := runStep(t, "xml-validation", map[string]any{
		"checkSource": false,
		"checkTarget": true,
	}, "nb", stepBlock("<b>Save</b>", "<b>Lagre</i>"))
	assert.Equal(t, "false", b.Properties[tools.PropXMLValid],
		"the mismatched target tags must be reported invalid")
	assert.Contains(t, b.Properties[tools.PropXMLValidError], "target:")

	// Defaults: source only, and this source is well-formed.
	clean := runStep(t, "xml-validation", nil, "nb", stepBlock("<b>Save</b>", "<b>Lagre</i>"))
	assert.Equal(t, "true", clean.Properties[tools.PropXMLValid],
		"with checkTarget at its default the broken target is not looked at")
}

// ── create-target ───────────────────────────────────────────────────────────

// The zero-arg factory left TargetLocale empty, so create-target wrote its
// container under the empty locale — a target for no language.
func TestCreateTarget_StepConfigCopySourceIntoTheRunLocale(t *testing.T) {
	b := &model.Block{ID: "u1", Translatable: true,
		Source: []model.Run{{Text: &model.TextRun{Text: "Save now"}}}}
	out := runStep(t, "create-target", map[string]any{"copySource": true}, "nb", b)

	assert.Equal(t, "Save now", out.TargetText(model.LocaleID("nb")),
		"copySource seeds the run's locale with the source text")
	assert.Empty(t, out.TargetText(model.LocaleID("")),
		"nothing may be written under the empty locale")

	// copySource defaults to false: an empty container, not a copy.
	empty := runStep(t, "create-target", nil, "nb", &model.Block{ID: "u1", Translatable: true,
		Source: []model.Run{{Text: &model.TextRun{Text: "Save now"}}}})
	require.True(t, empty.HasTarget(model.LocaleID("nb")))
	assert.Empty(t, empty.TargetText(model.LocaleID("nb")))
}

// overwrite defaults to false, so an existing translation survives unless the
// step says otherwise — the dropped config made the destructive answer
// unreachable *and* the protective one accidental.
func TestCreateTarget_StepConfigOverwrite(t *testing.T) {
	kept := runStep(t, "create-target", map[string]any{"copySource": true}, "nb",
		stepBlock("Save now", "Lagre nå"))
	assert.Equal(t, "Lagre nå", kept.TargetText(model.LocaleID("nb")),
		"an existing target is kept when overwrite is at its default")

	replaced := runStep(t, "create-target", map[string]any{"copySource": true, "overwrite": true}, "nb",
		stepBlock("Save now", "Lagre nå"))
	assert.Equal(t, "Save now", replaced.TargetText(model.LocaleID("nb")),
		"the step asked for the existing target to be overwritten")
}

// ── remove-target ───────────────────────────────────────────────────────────

// The destructive one. Defaults are FilterByIDs with an empty TextUnitIDs (which
// narrows nothing) and an empty TargetLocale (which means every locale), so a
// step that named the ids to clear cleared everything instead.
func TestRemoveTarget_StepConfigNarrowsByTextUnitID(t *testing.T) {
	keep := runStep(t, "remove-target", map[string]any{"textUnitIDs": "other"}, "nb",
		stepBlock("Save now", "Lagre nå"))
	assert.Equal(t, "Lagre nå", keep.TargetText(model.LocaleID("nb")),
		"a block outside the configured id list must be left alone")

	hit := runStep(t, "remove-target", map[string]any{"textUnitIDs": "u1, other"}, "nb",
		stepBlock("Save now", "Lagre nå"))
	assert.False(t, hit.HasTarget(model.LocaleID("nb")),
		"a block named in the id list has its target removed")
}

// The run's target language pins the locale, so a run for `nb` cannot wipe the
// locales it was never pointed at.
func TestRemoveTarget_RunLocalePinsWhatIsRemoved(t *testing.T) {
	b := stepBlock("Save now", "Lagre nå")
	tool.NewVariantView(b).SetTargetText(model.LocaleID("de"), "Jetzt speichern")

	out := runStep(t, "remove-target", nil, "nb", b)
	assert.False(t, out.HasTarget(model.LocaleID("nb")), "the run's locale is removed")
	assert.Equal(t, "Jetzt speichern", out.TargetText(model.LocaleID("de")),
		"a locale the run was not pointed at survives")
}

// ── inline-codes-remove ─────────────────────────────────────────────────────

// TargetLocale was empty from the zero-arg factory, so the tool stripped codes
// from nothing; and applySource / replaceWithSpace were unreachable.
// lineBreakBlock is a bilingual block whose target carries a line-break inline
// code between two words — the shape replaceWithSpace exists for.
func lineBreakBlock() *model.Block {
	b := &model.Block{ID: "u1", Translatable: true,
		Source: []model.Run{{Text: &model.TextRun{Text: "Save"}}}}
	b.SetTargetRuns(model.LocaleID("nb"), []model.Run{
		{Text: &model.TextRun{Text: "Lagre"}},
		{Ph: &model.PlaceholderRun{ID: "1", Type: "html:br"}},
		{Text: &model.TextRun{Text: "straks"}},
	})
	return b
}

func TestInlineCodesRemove_StepConfigStripsTheRunLocaleTarget(t *testing.T) {
	out := runStep(t, "inline-codes-remove", nil, "nb", lineBreakBlock())
	assert.Equal(t, []string{"text:Lagrestraks"}, runShapes(out.TargetRuns(model.LocaleID("nb"))),
		"the target's inline code is stripped for the run's locale, and the "+
			"surrounding text runs merge")
}

// replaceWithSpace defaults to false, and flips the result in both directions.
func TestInlineCodesRemove_StepConfigReplaceWithSpace(t *testing.T) {
	spaced := runStep(t, "inline-codes-remove", map[string]any{"replaceWithSpace": true}, "nb", lineBreakBlock())
	assert.Equal(t, "Lagre straks", model.RunsText(spaced.TargetRuns(model.LocaleID("nb"))),
		"replaceWithSpace puts a space where the line break was")

	joined := runStep(t, "inline-codes-remove", nil, "nb", lineBreakBlock())
	assert.Equal(t, "Lagrestraks", model.RunsText(joined.TargetRuns(model.LocaleID("nb"))),
		"at its default the code is removed outright")
}

// applySource is off by default, so a step that asked for the source was ignored.
func TestInlineCodesRemove_StepConfigApplySource(t *testing.T) {
	withCodedSource := func() *model.Block {
		b := &model.Block{ID: "u1", Translatable: true, Source: []model.Run{
			{Text: &model.TextRun{Text: "Save"}},
			{Ph: &model.PlaceholderRun{ID: "1", Type: "html:br"}},
			{Text: &model.TextRun{Text: "now"}},
		}}
		tool.NewVariantView(b).SetTargetText(model.LocaleID("nb"), "Lagre nå")
		return b
	}
	untouched := runStep(t, "inline-codes-remove", nil, "nb", withCodedSource())
	assert.Equal(t, []string{"text:Save", "ph:", "text:now"}, runShapes(untouched.Source),
		"the source keeps its codes at the default")

	stripped := runStep(t, "inline-codes-remove", map[string]any{"applySource": true}, "nb", withCodedSource())
	assert.Equal(t, []string{"text:Savenow"}, runShapes(stripped.Source),
		"the step asked for the source too")
}

// ── tag-protect ─────────────────────────────────────────────────────────────

// Patterns is read once, at construction, when the regexes are compiled — so it
// can only arrive through a config factory. Without one every flow got
// defaultTagPatterns however the step was written.
func TestTagProtect_StepConfigPatternsReplaceTheDefaults(t *testing.T) {
	// The default patterns match <b> and {name}; this step asks for neither.
	b := runStep(t, "tag-protect", map[string]any{
		"patterns": []any{`\[\[[^\]]+\]\]`},
	}, "", stepBlock("Save <b>{name}</b> [[TOKEN]]", "Lagre nå"))
	assert.Equal(t, "1", b.Properties[tools.PropTagProtectCount],
		"only the step's own pattern must match — the defaults are replaced, not added to")

	def := runStep(t, "tag-protect", nil, "", stepBlock("Save <b>{name}</b> [[TOKEN]]", "Lagre nå"))
	assert.NotEqual(t, "1", def.Properties[tools.PropTagProtectCount],
		"the default patterns match the markup and the placeholder too")
}

// An invalid pattern must fail at construction, naming the tool — not compile to
// nothing and silently protect nothing.
func TestTagProtect_StepConfigRejectsAnInvalidPattern(t *testing.T) {
	reg := registry.NewToolRegistry()
	tools.RegisterAll(reg)
	_, err := reg.NewToolWithConfig("tag-protect", map[string]any{"patterns": []any{"("}}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tag-protect")
}

// ── properties-set ──────────────────────────────────────────────────────────

// The Properties map is the tool's entire behaviour and can only come from the
// step, so with the config dropped the tool set nothing at all.
func TestPropertiesSet_StepConfigSetsTheProperties(t *testing.T) {
	b := runStep(t, "properties-set", map[string]any{
		"properties": map[string]any{"reviewed": "yes"},
	}, "", stepBlock("Save now", "Lagre nå"))
	assert.Equal(t, "yes", b.Properties["reviewed"])
}

// overwrite defaults to true; a step can turn it off to keep what is there.
func TestPropertiesSet_StepConfigOverwrite(t *testing.T) {
	withExisting := func() *model.Block {
		b := stepBlock("Save now", "Lagre nå")
		b.Properties = map[string]string{"reviewed": "no"}
		return b
	}
	replaced := runStep(t, "properties-set", map[string]any{
		"properties": map[string]any{"reviewed": "yes"},
	}, "", withExisting())
	assert.Equal(t, "yes", replaced.Properties["reviewed"], "overwrite is on by default")

	kept := runStep(t, "properties-set", map[string]any{
		"properties": map[string]any{"reviewed": "yes"},
		"overwrite":  false,
	}, "", withExisting())
	assert.Equal(t, "no", kept.Properties["reviewed"], "the step turned overwrite off")
}

// ── external-command ────────────────────────────────────────────────────────

// Command and Args are the tool's entire behaviour. With the config dropped the
// tool was built with an empty Command, so every block failed with "no command"
// into a block property nothing reads.
func TestExternalCommand_StepConfigRunsTheConfiguredProgram(t *testing.T) {
	b := runStep(t, "external-command", map[string]any{
		"command":     "tr",
		"args":        []any{"a-z", "A-Z"},
		"sendAsStdin": true,
	}, "nb", stepBlock("Save now", "lagre straks"))

	assert.Equal(t, "LAGRE STRAKS", b.TargetText(model.LocaleID("nb")),
		"the target is replaced by the configured program's stdout, for the run's locale")
	assert.Equal(t, "0", b.Properties[tools.PropExternalCommandExitCode])
}

// A step with no command fails at construction, naming the tool, instead of
// recording a per-block failure nothing surfaces.
func TestExternalCommand_StepConfigRequiresACommand(t *testing.T) {
	reg := registry.NewToolRegistry()
	tools.RegisterAll(reg)
	_, err := reg.NewToolWithConfig("external-command", nil, "nb")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "external-command")
}

// ── batch ───────────────────────────────────────────────────────────────────

// batch is Internal — withheld from `kapi exec` — but a flow still names it as a
// step, and its size is the step's to choose. Internal and unconfigurable are
// different things.
func TestBatch_StepConfigSizeIsHonoured(t *testing.T) {
	tl := stepTool(t, "batch", map[string]any{"size": 3}, "")
	cfg, ok := tl.Config().(*tools.BatchConfig)
	require.True(t, ok)
	assert.Equal(t, 3, cfg.Size, "the step's batch size must reach the tool, not the default 10")

	_, err := registry.NewToolRegistry().NewToolWithConfig("batch", nil, "")
	require.Error(t, err, "an unregistered tool is still an error, not a silent default")
}

func TestBatch_StepConfigRejectsAnInvalidSize(t *testing.T) {
	reg := registry.NewToolRegistry()
	tools.RegisterAll(reg)
	_, err := reg.NewToolWithConfig("batch", map[string]any{"size": 0}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "batch")
}
