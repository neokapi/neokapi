package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	fschema "github.com/neokapi/neokapi/core/format/schema"
	coreschema "github.com/neokapi/neokapi/core/schema"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLocales(t *testing.T) {
	assert.Nil(t, parseLocales(""))
	assert.Nil(t, parseLocales("  , "))
	assert.Equal(t, []string{"nb", "qps"}, parseLocales("qps nb"))
	assert.Equal(t, []string{"nb", "qps"}, parseLocales("qps,nb,qps"))
	assert.Equal(t, []string{"qps"}, parseLocales("qps "), "the compile stage passes an empty target list after the probe")
}

func TestDiscoverLocales(t *testing.T) {
	core := t.TempDir()
	cli := t.TempDir()
	writeFile(t, core, "qps.json", "{}")
	writeFile(t, core, "nb.json", "{}")
	writeFile(t, core, "README.md", "not a catalog")
	writeFile(t, cli, "qps.json", "{}")
	writeFile(t, cli, "de.json", "{}")
	require.NoError(t, os.Mkdir(filepath.Join(cli, "sub.json"), 0o755))

	got, err := discoverLocales(core, cli, filepath.Join(core, "absent"))
	require.NoError(t, err)
	assert.Equal(t, []string{"de", "nb", "qps"}, got)
}

func TestLookup(t *testing.T) {
	cat := map[string]any{"tools": map[string]any{"segment": map[string]any{"displayName": "Šéĝ", "description": ""}}}
	v, ok := lookup(cat, "tools", "segment", "displayName")
	assert.True(t, ok)
	assert.Equal(t, "Šéĝ", v)
	_, ok = lookup(cat, "tools", "segment", "description")
	assert.False(t, ok, "an empty leaf is nothing to translate")
	_, ok = lookup(cat, "tools", "missing", "displayName")
	assert.False(t, ok)
	_, ok = lookup(cat, "tools", "segment", "displayName", "deeper")
	assert.False(t, ok, "a string is a leaf")
}

func TestLocalizeEntries(t *testing.T) {
	ds := Dataset{Kind: KindTool, Entries: []Entry{
		{ID: "segment", Source: SourceBuiltIn, DisplayName: "Segment", Description: "Split blocks."},
		{ID: "bare", Source: SourceBuiltIn, DisplayName: "Bare"},
		{ID: "okf_x", Source: SourceOkapi, DisplayName: "Bridge", Description: "From the bridge."},
	}}
	cat := map[string]any{"tools": map[string]any{
		"segment": map[string]any{"displayName": "Šéĝḿéñţ", "description": "Šþļîţ."},
		"okf_x":   map[string]any{"displayName": "never"},
	}}
	var cov localeCoverage
	got, err := localizeEntries(ds, cat, "tools", "qps", &cov)
	require.NoError(t, err)

	assert.Equal(t, "Šéĝḿéñţ", got.Entries[0].DisplayName)
	assert.Equal(t, "Šþļîţ.", got.Entries[0].Description)
	assert.Equal(t, "Bare", got.Entries[1].DisplayName, "keeps English where the catalog has nothing")
	assert.Equal(t, "Bridge", got.Entries[2].DisplayName, "a bridge entry has no catalog and is left alone")
	assert.Equal(t, 2, cov.translated)
	assert.Equal(t, 1, cov.fallback(), "an empty English description is not counted")
	assert.True(t, cov.missing["tools.bare.displayName"])
	assert.Equal(t, "Segment", ds.Entries[0].DisplayName, "the English dataset is not mutated")
}

func TestLocalizeCommands(t *testing.T) {
	ds := CommandDataset{Commands: []CommandEntry{
		{ID: "add", Path: []string{"add"}, Short: "Add patterns", Long: "Long add"},
		{ID: "config.get", Path: []string{"config", "get"}, Short: "Get a value"},
	}}
	cat := map[string]any{"cli": map[string]any{"commands": map[string]any{"kapi": map[string]any{
		"add":    map[string]any{"short": "Àđđ", "long": "Ļöñĝ àđđ"},
		"config": map[string]any{"short": "Çöñƒîĝ", "get": map[string]any{"short": "Ĝéţ"}},
	}}}}
	var cov localeCoverage
	got := localizeCommands(ds, cat, &cov)

	assert.Equal(t, "Àđđ", got.Commands[0].Short)
	assert.Equal(t, "Ļöñĝ àđđ", got.Commands[0].Long)
	assert.Equal(t, "Ĝéţ", got.Commands[1].Short, "a subcommand is keyed by its path")
	assert.Equal(t, 3, cov.translated)
	assert.Equal(t, 0, cov.fallback())
}

func TestLocalizeModels(t *testing.T) {
	ds := ModelDataset{Models: []ModelEntry{
		{ID: "gpt-5.6", Label: "GPT-5.6", Note: "The default id resolves here."},
		{ID: "quiet", Label: "Quiet"},
	}}
	cat := map[string]any{"models": map[string]any{"gpt-5.6": map[string]any{"note": "Ţĥé đéƒàüļţ."}}}
	var cov localeCoverage
	got := localizeModels(ds, cat, &cov)

	assert.Equal(t, "Ţĥé đéƒàüļţ.", got.Models[0].Note)
	assert.Equal(t, "GPT-5.6", got.Models[0].Label, "labels are names")
	assert.Empty(t, got.Models[1].Note)
	assert.Equal(t, 1, cov.translated)
	assert.Equal(t, 0, cov.fallback())
}

// A workspace with a small English dataset and one catalog per locale.
func localeWorkspace(t *testing.T) (outDir, coreDir, cliDir string) {
	t.Helper()
	outDir, coreDir, cliDir = t.TempDir(), t.TempDir(), t.TempDir()
	require.NoError(t, writeJSON(filepath.Join(outDir, "tools.json"), Dataset{GeneratedAt: "2026-09-05T00:00:00Z", Kind: KindTool, Entries: []Entry{
		{ID: "segment", Source: SourceBuiltIn, Kind: KindTool, DisplayName: "Segment", Description: "Split blocks."},
	}}))
	require.NoError(t, writeJSON(filepath.Join(outDir, "formats.json"), Dataset{GeneratedAt: "2026-09-05T00:00:00Z", Kind: KindFormat, Entries: []Entry{
		{ID: "markdown", Source: SourceBuiltIn, Kind: KindFormat, DisplayName: "Markdown", Description: "Read Markdown."},
	}}))
	require.NoError(t, writeJSON(filepath.Join(outDir, "commands.json"), CommandDataset{GeneratedAt: "2026-09-05T00:00:00Z", Commands: []CommandEntry{
		{ID: "add", Path: []string{"add"}, Use: "add", Short: "Add patterns"},
	}}))
	require.NoError(t, writeJSON(filepath.Join(outDir, "models.json"), ModelDataset{GeneratedAt: "2026-09-05T00:00:00Z", Models: []ModelEntry{
		{ID: "m", Provider: "p", Label: "M", Note: "A note."},
	}}))
	writeFile(t, coreDir, "qps.json", `{"tools":{"segment":{"displayName":"Šéĝḿéñţ","description":"Šþļîţ ƀļöçķš."}},"formats":{"markdown":{"displayName":"Ḿàŕķđöŵñ","description":"Ŕéàđ Ḿàŕķđöŵñ."}},"models":{"m":{"note":"À ñöţé."}}}`)
	writeFile(t, cliDir, "qps.json", `{"cli":{"commands":{"kapi":{"add":{"short":"Àđđ þàţţéŕñš"}}}}}`)
	writeFile(t, coreDir, "nb.json", `{"tools":{"segment":{"displayName":"Segmenter"}}}`)
	return outDir, coreDir, cliDir
}

func TestWriteLocaleVariants(t *testing.T) {
	outDir, coreDir, cliDir := localeWorkspace(t)
	require.NoError(t, os.MkdirAll(filepath.Join(outDir, "stale", "x"), 0o755))

	require.NoError(t, writeLocaleVariants(outDir, coreDir, cliDir, "", nil))

	var tools Dataset
	require.NoError(t, readJSON(filepath.Join(outDir, "qps", "tools.json"), &tools))
	assert.Equal(t, "Šéĝḿéñţ", tools.Entries[0].DisplayName)
	assert.Equal(t, "2026-09-05T00:00:00Z", tools.GeneratedAt, "a variant carries its English dataset's timestamp")
	var cmds CommandDataset
	require.NoError(t, readJSON(filepath.Join(outDir, "qps", "commands.json"), &cmds))
	assert.Equal(t, "Àđđ þàţţéŕñš", cmds.Commands[0].Short)
	var models ModelDataset
	require.NoError(t, readJSON(filepath.Join(outDir, "qps", "models.json"), &models))
	assert.Equal(t, "À ñöţé.", models.Models[0].Note)

	// nb has a core catalog only, and a partial one: everything else keeps its English.
	var nbTools Dataset
	require.NoError(t, readJSON(filepath.Join(outDir, "nb", "tools.json"), &nbTools))
	assert.Equal(t, "Segmenter", nbTools.Entries[0].DisplayName)
	assert.Equal(t, "Split blocks.", nbTools.Entries[0].Description)
	var nbCmds CommandDataset
	require.NoError(t, readJSON(filepath.Join(outDir, "nb", "commands.json"), &nbCmds))
	assert.Equal(t, "Add patterns", nbCmds.Commands[0].Short)

	_, err := os.Stat(filepath.Join(outDir, "stale"))
	assert.True(t, os.IsNotExist(err), "a variant directory with no catalog behind it is removed")
	_, err = os.Stat(filepath.Join(outDir, "tools.json"))
	require.NoError(t, err, "the English dataset stays")
}

func TestWriteLocaleVariants_NamedLocales(t *testing.T) {
	outDir, coreDir, cliDir := localeWorkspace(t)
	require.NoError(t, os.MkdirAll(filepath.Join(outDir, "nb"), 0o755))
	writeFile(t, outDir, "nb/tools.json", "kept")

	require.NoError(t, writeLocaleVariants(outDir, coreDir, cliDir, "", []string{"qps", "xx"}))

	_, err := os.Stat(filepath.Join(outDir, "qps", "tools.json"))
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(outDir, "nb", "tools.json"))
	require.NoError(t, err)
	assert.Equal(t, "kept", string(data), "a locale not named is not rewritten")
	_, err = os.Stat(filepath.Join(outDir, "xx"))
	assert.True(t, os.IsNotExist(err), "a locale with no catalog is skipped with a warning")
}

func TestLocaleVariantDrift(t *testing.T) {
	outDir, coreDir, cliDir := localeWorkspace(t)
	require.NoError(t, writeLocaleVariants(outDir, coreDir, cliDir, "", nil))
	assert.Empty(t, localeVariantDrift(outDir, coreDir, cliDir, ""), "fresh variants are not drift")

	// A regenerated English dataset carries a new timestamp; that alone is not drift.
	var tools Dataset
	require.NoError(t, readJSON(filepath.Join(outDir, "tools.json"), &tools))
	tools.GeneratedAt = "2026-09-06T00:00:00Z"
	require.NoError(t, writeJSON(filepath.Join(outDir, "tools.json"), tools))
	assert.Empty(t, localeVariantDrift(outDir, coreDir, cliDir, ""))

	// The loop wrote a new catalog and the variant did not follow.
	writeFile(t, coreDir, "nb.json", `{"tools":{"segment":{"displayName":"Segmentering"}}}`)
	problems := localeVariantDrift(outDir, coreDir, cliDir, "")
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], "nb/tools.json")

	// A locale with a catalog and no variant at all.
	writeFile(t, coreDir, "de.json", `{}`)
	problems = localeVariantDrift(outDir, coreDir, cliDir, "")
	assert.Len(t, problems, 5)
}

// The variants of the committed dataset round-trip its bytes: a locale with
// nothing translated reproduces the English file exactly, which is what lets
// the compile stage and a full regeneration agree.
func TestLocaleVariant_RoundTripsCommittedDataset(t *testing.T) {
	committed := filepath.Join("..", "..", "packages", "reference-data", "data")
	if _, err := os.Stat(filepath.Join(committed, "tools.json")); err != nil {
		t.Skip("committed dataset not present")
	}
	outDir := t.TempDir()
	for _, name := range localeFiles {
		data, err := os.ReadFile(filepath.Join(committed, name))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(outDir, name), data, 0o644))
	}
	coreDir, cliDir := t.TempDir(), t.TempDir()
	writeFile(t, coreDir, "xx.json", "{}")

	require.NoError(t, writeLocaleVariants(outDir, coreDir, cliDir, "", nil))

	for _, name := range localeFiles {
		want, err := os.ReadFile(filepath.Join(outDir, name))
		require.NoError(t, err)
		got, err := os.ReadFile(filepath.Join(outDir, "xx", name))
		require.NoError(t, err)
		assert.Equal(t, string(want), string(got), "%s does not round-trip", name)
	}
}

func TestWriteLocaleVariants_OverlaysTranslatedDossiers(t *testing.T) {
	outDir, coreDir, cliDir := localeWorkspace(t)
	docs := t.TempDir()
	writeFile(t, docs, "tools/segment.yaml", "description: English dossier description\noverview: English overview\n")
	writeFile(t, docs, "qps/tools/segment.yaml", "description: Ðöššîéŕ đéšçŕîþţîöñ\noverview: Ṕšéüđö övéŕvîéŵ\nlimitations:\n  - Öñé\n")
	writeFile(t, docs, "qps/tools/renamed.yaml", "overview: documents nothing\n")
	writeFile(t, docs, "qps/formats/markdown.yaml", "overview: Ḿàŕķđöŵñ övéŕvîéŵ\n")

	require.NoError(t, writeLocaleVariants(outDir, coreDir, cliDir, docs, []string{"qps", "nb"}))

	var tools Dataset
	require.NoError(t, readJSON(filepath.Join(outDir, "qps", "tools.json"), &tools))
	assert.Equal(t, "Ðöššîéŕ đéšçŕîþţîöñ", tools.Entries[0].Description, "a dossier's description outranks the catalog's, as it does in English")
	require.NotNil(t, tools.Entries[0].Doc)
	assert.Equal(t, "Ṕšéüđö övéŕvîéŵ", tools.Entries[0].Doc.Overview)
	assert.Equal(t, []string{"Öñé"}, tools.Entries[0].Doc.Limitations)
	var formats Dataset
	require.NoError(t, readJSON(filepath.Join(outDir, "qps", "formats.json"), &formats))
	assert.Equal(t, "Ḿàŕķđöŵñ övéŕvîéŵ", formats.Entries[0].Doc.Overview)

	// nb has no translated dossiers: the variant keeps whatever the English
	// dataset carried.
	var nbTools Dataset
	require.NoError(t, readJSON(filepath.Join(outDir, "nb", "tools.json"), &nbTools))
	assert.Equal(t, "Split blocks.", nbTools.Entries[0].Description)
	assert.Nil(t, nbTools.Entries[0].Doc)

	assert.Empty(t, localeVariantDrift(outDir, coreDir, cliDir, docs))
	writeFile(t, docs, "qps/tools/segment.yaml", "overview: changed\n")
	problems := localeVariantDrift(outDir, coreDir, cliDir, docs)
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], "qps/tools.json")
}

func TestOverlayLocaleDocs_RejectsBrokenYAML(t *testing.T) {
	docs := t.TempDir()
	writeFile(t, docs, "tools/segment.yaml", "overview: [unclosed\n")
	entries := []Entry{{ID: "segment", Source: SourceBuiltIn}}
	err := overlayLocaleDocs(docs, KindTool, entries, &localeCoverage{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

func TestOverlayLocaleDocs_SuppliesWhatTheCatalogLacked(t *testing.T) {
	docs := t.TempDir()
	writeFile(t, docs, "tools/segment.yaml", "description: Ðöššîéŕ\n")
	entries := []Entry{{ID: "segment", Source: SourceBuiltIn, DisplayName: "Segment", Description: "Split."}}
	var cov localeCoverage
	_, err := localizeEntries(Dataset{Entries: entries}, map[string]any{}, "tools", "qps", &cov)
	require.NoError(t, err)
	require.Equal(t, 2, cov.fallback())

	require.NoError(t, overlayLocaleDocs(docs, KindTool, entries, &cov))

	assert.Equal(t, 1, cov.fallback(), "the description came from the dossier; the name is still missing")
	assert.True(t, cov.missing["tools.segment.displayName"])
	assert.Equal(t, 1, cov.dossiers)
}

func TestLookupScoped(t *testing.T) {
	cat := map[string]any{"formats": map[string]any{"markdown": map[string]any{"properties": map[string]any{
		"parser.preserveWhitespace": map[string]any{"title": "Ķééþ"},
		"parser":                    map[string]any{"title": "Ṕàŕšéŕ"},
	}}}}
	v, ok := lookupScoped(cat, []string{"formats", "markdown", "properties", "parser.preserveWhitespace", "title"})
	assert.True(t, ok)
	assert.Equal(t, "Ķééþ", v, "a dotted property name is matched whole")
	v, ok = lookupScoped(cat, []string{"formats", "markdown", "properties", "parser", "title"})
	assert.True(t, ok)
	assert.Equal(t, "Ṕàŕšéŕ", v)
	_, ok = lookupScoped(cat, []string{"formats", "markdown", "properties", "parser.other", "title"})
	assert.False(t, ok)
}

func TestLocalizeSchema(t *testing.T) {
	// Marshalled from the struct, as the dataset's schemas are.
	tool, err := json.Marshal(&coreschema.ComponentSchema{
		Title: "Segment", Description: "Split blocks.",
		ToolMeta: &coreschema.ToolMeta{ID: "segment", DisplayName: "Segment"},
		Properties: map[string]coreschema.PropertySchema{
			"engine": {Type: "string", Title: "Engine", Description: "Which engine", Options: []coreschema.OptionItem{{Value: "srx", Label: "SRX"}}},
			"plain":  {Type: "boolean", Title: "Plain"},
		},
		Groups: []coreschema.ParameterGroup{{ID: "main", Label: "Main"}},
	})
	require.NoError(t, err)
	cat := map[string]any{"tools": map[string]any{"segment": map[string]any{
		"displayName": "Šéĝḿéñţ",
		"properties": map[string]any{
			"engine": map[string]any{"title": "Éñĝîñé", "description": "Ŵĥîçĥ", "options": map[string]any{"srx": map[string]any{"label": "ŠŔẊ"}}},
		},
		"groups": map[string]any{"main": map[string]any{"label": "Ḿàîñ"}},
	}}}
	var cov localeCoverage
	out, err := localizeSchema(tool, KindTool, "segment", cat, "qps", &cov)
	require.NoError(t, err)
	var got coreschema.ComponentSchema
	require.NoError(t, json.Unmarshal(out, &got))
	assert.Equal(t, "Éñĝîñé", got.Properties["engine"].Title)
	assert.Equal(t, "Ŵĥîçĥ", got.Properties["engine"].Description)
	assert.Equal(t, "ŠŔẊ", got.Properties["engine"].Options[0].Label)
	assert.Equal(t, "Ḿàîñ", got.Groups[0].Label)
	assert.Equal(t, "Plain", got.Properties["plain"].Title, "keeps English where the catalog has nothing")
	assert.Equal(t, "Šéĝḿéñţ", got.ToolMeta.DisplayName)
	assert.Equal(t, "Šéĝḿéñţ", got.Title, "the schema's title is the display name")
	assert.True(t, cov.missing["tools.segment.properties.plain.title"])
	assert.False(t, cov.missing["tools.segment.title"])

	// Nothing translated: the bytes come back exactly as they went in.
	same, err := localizeSchema(tool, KindTool, "segment", map[string]any{}, "qps", &localeCoverage{})
	require.NoError(t, err)
	assert.Equal(t, string(tool), string(same))
}

func TestLocalizeSchema_Format(t *testing.T) {
	format, err := json.Marshal(&fschema.FormatSchema{
		ID: "markdown", Version: "1", Title: "Markdown", Description: "Read Markdown.", Type: "object",
		FormatMeta: fschema.FormatMeta{ID: "markdown", Extensions: []string{".md"}},
		Properties: map[string]fschema.PropertySchema{
			"parser": {
				PropertySchema: coreschema.PropertySchema{Type: "object", Title: "Parser"},
				FlattenPath:    "parser",
				Properties: map[string]fschema.PropertySchema{
					"preserveWhitespace": {PropertySchema: coreschema.PropertySchema{Type: "boolean", Title: "Keep whitespace"}},
				},
			},
		},
	})
	require.NoError(t, err)
	cat := map[string]any{"formats": map[string]any{"markdown": map[string]any{
		"properties": map[string]any{"parser": map[string]any{"title": "Ṕàŕšéŕ", "properties": map[string]any{"preserveWhitespace": map[string]any{"title": "Ķééþ"}}}},
	}}}
	var cov localeCoverage
	out, err := localizeSchema(format, KindFormat, "markdown", cat, "qps", &cov)
	require.NoError(t, err)
	var got fschema.FormatSchema
	require.NoError(t, json.Unmarshal(out, &got))
	assert.Equal(t, "Ṕàŕšéŕ", got.Properties["parser"].Title)
	assert.Equal(t, "Ķééþ", got.Properties["parser"].Properties["preserveWhitespace"].Title)
	assert.Equal(t, "Markdown", got.Title, "the format's own title stays: it is the name")
	assert.Equal(t, ".md", got.FormatMeta.Extensions[0], "data fields pass through")
	assert.Equal(t, "parser", got.Properties["parser"].FlattenPath, "format-level fields survive the crossing")

	same, err := localizeSchema(format, KindFormat, "markdown", map[string]any{}, "qps", &localeCoverage{})
	require.NoError(t, err)
	assert.Equal(t, string(format), string(same))
}
