package tools_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
)

// The registration invariants for built-in tools, asserted over the populated
// registry rather than trusted to a reviewer.
//
// Thirteen built-in tools were registered with a parameter schema and no
// ConfigFactory (#1476). Each lost three things at once, silently: its step
// `config:` block was discarded by NewToolWithConfig, any locale its zero-arg
// factory hardcoded outranked the run's --target-lang, and it vanished from
// `kapi tools list` / `kapi exec` / the MCP surface, because registry.CLITools
// requires a config factory. Nothing distinguished "forgot to register one" from
// "deliberately not a command" — so both were spelled the same way, and the
// first hid behind the second.
//
// These tests are the guard that closes the class: they fail on the *next*
// omission, at registration, rather than when a user notices a configured tool
// did nothing.

// builtInRegistry is the registry every kapi binary runs with: the framework's
// built-in tools plus the AI tools.
func builtInRegistry(t *testing.T) *registry.ToolRegistry {
	t.Helper()
	reg := registry.NewToolRegistry()
	tools.RegisterAll(reg)
	aitools.RegisterAll(reg)
	return reg
}

// builtInNames returns every built-in tool id in the registry, sorted.
func builtInNames(t *testing.T, reg *registry.ToolRegistry) []registry.ToolID {
	t.Helper()
	var names []registry.ToolID
	for _, info := range reg.ListWithSchemas() {
		if info.Source == registry.SourceBuiltIn {
			names = append(names, info.Name)
		}
	}
	slices.Sort(names)
	require.NotEmpty(t, names)
	return names
}

// INVARIANT 1 — a built-in tool that declares settable schema fields must
// register a ConfigFactory.
//
// The schema is the tool's public promise that those fields can be set. Without
// a config factory the promise is void: ToolRegistry.NewToolWithConfig calls the
// zero-arg factory and throws the step's map away without a word.
//
// "Settable" is exactly the schema's Properties — the fields FromStruct
// projected, so `schema:"-"`, interfaces, funcs and channels are already
// excluded. A tool with no settable fields (span-classify, layer-processor,
// voice-vocab-check) has nothing a step could configure and needs no factory.
func TestEveryConfigurableBuiltInToolHasAConfigFactory(t *testing.T) {
	reg := builtInRegistry(t)
	checked := 0
	for _, name := range builtInNames(t, reg) {
		s := reg.Schema(name)
		if s == nil || len(s.Properties) == 0 {
			continue
		}
		checked++
		assert.True(t, reg.HasConfigFactory(name),
			"tool %q declares %d settable schema field(s) but registers no ConfigFactory: "+
				"a flow step's config: block would be silently discarded, and the tool would be "+
				"absent from `kapi tools list`, `kapi exec` and MCP. Register one in "+
				"registerConfigFactories (see NewWhitespaceCorrectFromConfig for the shape).",
			name, len(s.Properties))
	}
	assert.Greater(t, checked, 20, "the invariant should be covering the whole built-in set")
}

// INVARIANT 2 — a tool is withheld from the CLI by declaration, never by
// omission.
//
// The internal set is spelled out here so it can be read: adding a tool to it is
// a decision someone makes on purpose. Before this, "internal" was expressed by
// *not* registering a config factory, which is also how a tool ends up
// misconfigured by accident — the two were indistinguishable.
func TestInternalToolsAreWithheldByDeclarationNotByOmission(t *testing.T) {
	// Pipeline plumbing (batch, layer-processor, span-classify), tools whose
	// whole output is consumed by later steps in the same flow and that no writer
	// serializes (properties-set, tag-protect), and tools whose input the host
	// resolves rather than the step (voice-vocab-check).
	wantInternal := map[registry.ToolID]bool{
		"batch":             true,
		"voice-vocab-check": true,
		"layer-processor":   true,
		"properties-set":    true,
		"span-classify":     true,
		"tag-protect":       true,
	}

	reg := builtInRegistry(t)
	gotInternal := map[registry.ToolID]bool{}
	for _, name := range builtInNames(t, reg) {
		if info := reg.ToolInfo(name); info != nil && info.Internal {
			gotInternal[name] = true
		}
	}
	assert.Equal(t, wantInternal, gotInternal,
		"the internal set is a declaration: add or remove withInternal() deliberately, and say why")

	cliVisible := map[registry.ToolID]bool{}
	for _, entry := range reg.CLITools() {
		cliVisible[entry.Info.Name] = true
	}
	for name := range wantInternal {
		assert.False(t, cliVisible[name], "internal tool %q must not reach the CLI/MCP surface", name)
	}
	// The other half of the contract: a built-in that is configurable and not
	// declared internal IS on the surface. Without this the test could pass by
	// hiding everything.
	for _, name := range builtInNames(t, reg) {
		if wantInternal[name] || !reg.HasConfigFactory(name) {
			continue
		}
		assert.True(t, cliVisible[name],
			"built-in %q has a config factory and is not internal, so it must be CLI-visible", name)
	}
}

// INVARIANT 3 — a bilingual tool's target locale comes from the run.
//
// The zero-arg factories hardcoded a locale (model.LocaleEnglish, or the empty
// locale). A config factory that forgets to let --target-lang win leaves the
// tool processing a locale the run never asked for and reporting success:
// #1471's whitespace-correct corrected the `en` target of every `nb` run.
//
// Asserted structurally over every CLI-visible built-in: build the tool the way
// a flow step does, for a run targeting `nb`, and require that a config field
// named TargetLocale holds `nb`.
func TestBilingualToolsTakeTheirLocaleFromTheRun(t *testing.T) {
	const runLocale = model.LocaleID("nb")
	reg := builtInRegistry(t)
	checked := 0
	for _, entry := range reg.CLITools() {
		name := entry.Info.Name
		tl, err := reg.NewToolWithConfig(name, nil, string(runLocale))
		if err != nil {
			// A tool that refuses to build with no config at all (external-command
			// needs a command) cannot be asked about its locale here; its own test
			// covers it.
			continue
		}
		cfg, ok := tl.(interface{ Config() tool.ToolConfig })
		if !ok || cfg.Config() == nil {
			continue
		}
		v := reflect.ValueOf(cfg.Config())
		if v.Kind() == reflect.Pointer {
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			continue
		}
		f := v.FieldByName("TargetLocale")
		if !f.IsValid() || f.Type() != reflect.TypeFor[model.LocaleID]() {
			continue
		}
		checked++
		assert.Equal(t, runLocale, model.LocaleID(f.String()),
			"tool %q built for a run targeting %q has TargetLocale %q: the run's "+
				"--target-lang must win, or the tool silently processes the wrong locale",
			name, runLocale, f.String())
	}
	assert.Greater(t, checked, 5, "the invariant should be covering the bilingual built-ins")
}

// INVARIANT 4 — a CLI-visible tool that rewrites content must declare
// WritesOutput.
//
// `kapi exec` grows -o / --output-dir only for tools that do (cli/toolcmds.go),
// so without it an exec run rewrites the content in memory, exits 0, and writes
// nothing — #1471 §3, found only by running the command. Capability comes from
// the tool's own handler (Produce writes a target, Transform rewrites source),
// not from a hand-maintained list, so a new tool is covered the day it is
// registered.
func TestContentRewritingCLIToolsDeclareWritesOutput(t *testing.T) {
	reg := builtInRegistry(t)
	checked := 0
	for _, entry := range reg.CLITools() {
		name := entry.Info.Name
		tl, err := reg.NewTool(name)
		if err != nil || tl == nil {
			continue
		}
		c, ok := tl.(tool.Capable)
		if !ok {
			continue
		}
		switch c.Capability() {
		case tool.CapProduce, tool.CapTransform:
		default:
			continue
		}
		checked++
		assert.True(t, entry.Info.WritesOutput,
			"tool %q rewrites content (capability %v) and is CLI-visible, so it must declare "+
				"withWritesOutput(): `kapi exec %s` has no -o without it and would write nothing",
			name, c.Capability(), name)
	}
	assert.Greater(t, checked, 5, "the invariant should be covering the content-rewriting built-ins")
}
