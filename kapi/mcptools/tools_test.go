package mcptools

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/cli"
	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testApp() *cli.App {
	a := &cli.App{}
	a.FormatReg = registry.NewFormatRegistry()
	formats.RegisterAll(a.FormatReg)
	a.ToolReg = registry.NewToolRegistry()
	tools.RegisterAll(a.ToolReg)
	aitools.RegisterAll(a.ToolReg)
	return a
}

func TestHandleListFormats(t *testing.T) {
	a := testApp()
	_, out, err := handleListFormats(a)
	require.NoError(t, err)
	assert.NotEmpty(t, out.Formats)
	assert.Equal(t, len(out.Formats), out.Total)

	// Verify json format is present.
	var found bool
	for _, e := range out.Formats {
		if e.Name == "json" {
			found = true
			assert.True(t, e.HasReader)
			assert.True(t, e.HasWriter)
			assert.Equal(t, registry.SourceBuiltIn, e.Source)
			break
		}
	}
	assert.True(t, found, "json format should be in the list")
}

func TestHandleDetectFormat(t *testing.T) {
	a := testApp()

	tests := []struct {
		name       string
		path       string
		wantFormat string
		wantErr    bool
	}{
		{"json file", "test.json", "json", false},
		{"html file", "page.html", "html", false},
		{"no extension", "noext", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, out, err := handleDetectFormat(a, DetectFormatInput{Path: tt.path})
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantFormat, out.Format)
			}
		})
	}
}

func TestHandleExtractContent(t *testing.T) {
	a := testApp()
	ctx := t.Context()

	// Use the JSON testdata fixture.
	fixturePath := filepath.Join("..", "..", "core", "formats", "json", "testdata", "simple.json")
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		t.Skip("test fixture not found:", fixturePath)
	}

	_, out, err := handleExtractContent(ctx, a, ExtractContentInput{
		Path:       fixturePath,
		SourceLang: "en",
	})
	require.NoError(t, err)
	assert.Equal(t, "json", out.Format)
	assert.NotEmpty(t, out.Blocks)
	assert.Greater(t, out.WordCount, 0)

	// Verify we have the expected blocks from simple.json.
	var texts []string
	for _, b := range out.Blocks {
		texts = append(texts, b.SourceText)
		assert.Greater(t, b.WordCount, 0)
		assert.NotEmpty(t, b.ID)
	}
	assert.Contains(t, texts, "Hello World")
}

// Outside a project, list_flows lists the composed built-in flows `kapi flows`
// lists.
func TestHandleListFlows(t *testing.T) {
	t.Setenv(project.NoProjectEnvVar, "1")
	_, out, err := handleListFlows(testApp(), ListFlowsInput{})
	require.NoError(t, err)
	assert.NotEmpty(t, out.Flows)
	assert.Equal(t, len(out.Flows), out.Total)
	assert.Empty(t, out.Warning)

	sources := map[string]string{}
	for _, f := range out.Flows {
		sources[f.Name] = f.Source
	}
	assert.Equal(t, "builtin", sources["translate"])
	assert.Equal(t, "builtin", sources["translate-qa"])
}

// For a project, list_flows lists what `kapi flows` lists: the recipe's inline
// flows and its flows_dir files, each name once, for the flow run_flow
// resolves it to (host.ResolveProjectFlow).
func TestHandleListFlows_ListsInlineAndFileFlows(t *testing.T) {
	t.Setenv(project.NoProjectEnvVar, "1")
	root := t.TempDir()
	recipe := filepath.Join(root, project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipe, []byte(`version: v1
name: FlowsTest
flows_dir: flows
defaults:
  source_language: en
flows:
  inline-check:
    steps:
      - tool: qa
  guard:
    steps:
      - tool: qa
  translate:
    steps:
      - tool: qa
`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".kapi"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "flows"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "flows", "file-check.yaml"),
		[]byte("description: Check from a file\nsteps:\n  - tool: qa\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "flows", "guard.yaml"),
		[]byte("steps:\n  - tool: qa\n"), 0o644))

	_, out, err := handleListFlows(testApp(), ListFlowsInput{Project: root})
	require.NoError(t, err)
	assert.Empty(t, out.Warning)

	byName := map[string][]FlowEntry{}
	for _, f := range out.Flows {
		byName[f.Name] = append(byName[f.Name], f)
	}
	for name, entries := range byName {
		assert.Len(t, entries, 1, "%s is listed once", name)
	}
	require.Contains(t, byName, "inline-check")
	assert.Equal(t, "inline", byName["inline-check"][0].Source)
	require.Contains(t, byName, "file-check")
	assert.Equal(t, "file", byName["file-check"][0].Source)
	assert.Equal(t, "Check from a file", byName["file-check"][0].Description)
	assert.Equal(t, "inline", byName["guard"][0].Source, "the inline flow wins over the file of its name")
	assert.Equal(t, "inline", byName["translate"][0].Source, "the recipe's flow replaces the built-in of its name")
	assert.Contains(t, byName, "translate-qa")
}

func TestHandleListTools(t *testing.T) {
	a := testApp()
	_, out, err := handleListTools(a)
	require.NoError(t, err)
	assert.NotEmpty(t, out.Tools)
	assert.Equal(t, len(out.Tools), out.Total)

	var names []string
	for _, tool := range out.Tools {
		names = append(names, tool.Name)
		assert.NotEmpty(t, tool.Description)
		assert.NotEmpty(t, tool.Source)
	}
	assert.Contains(t, names, "translate")
	assert.Contains(t, names, "pseudo-translate")
}

func TestHandlePseudoTranslate(t *testing.T) {
	a := testApp()
	ctx := t.Context()

	fixturePath := filepath.Join("..", "..", "core", "formats", "json", "testdata", "simple.json")
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		t.Skip("test fixture not found:", fixturePath)
	}

	// Use a temp dir for output.
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output_qps.json")

	_, out, err := handlePseudoTranslate(ctx, a, PseudoTranslateInput{
		Path:       fixturePath,
		OutputPath: outputPath,
	})
	require.NoError(t, err)
	assert.Equal(t, "pseudo-translate", out.FlowName)
	assert.Equal(t, fixturePath, out.InputPath)
	assert.Equal(t, outputPath, out.OutputPath)

	// Verify output file was written.
	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.NotEmpty(t, content)
}

func TestHandleRunFlow(t *testing.T) {
	a := testApp()
	ctx := t.Context()

	fixturePath := filepath.Join("..", "..", "core", "formats", "json", "testdata", "simple.json")
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		t.Skip("test fixture not found:", fixturePath)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.json")

	_, out, err := handleRunFlow(ctx, a, RunFlowInput{
		FlowName:   "pseudo-translate",
		Path:       fixturePath,
		TargetLang: "qps",
		OutputPath: outputPath,
	})
	require.NoError(t, err)
	assert.Equal(t, "pseudo-translate", out.FlowName)
	assert.Equal(t, outputPath, out.OutputPath)

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.NotEmpty(t, content)
}

// TestHandlePseudoTranslateAutoOutput verifies that pseudo_translate works
// when no explicit output_path is given (MCP default path — issue #636).
// The handler must compute the output path as <base>_<lang><ext> sibling
// to the input file and successfully write it.
func TestHandlePseudoTranslateAutoOutput(t *testing.T) {
	a := testApp()
	ctx := t.Context()

	// Write a small JSON file into a temp dir so the auto-generated sibling
	// output lands there too (avoids polluting the source tree).
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "content.json")
	require.NoError(t, os.WriteFile(inputPath, []byte(`{"hero.title":"Plan less. Ship more.","cta.start":"Start for free"}`), 0o644))

	// No output_path — handler must auto-generate it.
	_, out, err := handlePseudoTranslate(ctx, a, PseudoTranslateInput{
		Path:       inputPath,
		TargetLang: "qps",
	})
	require.NoError(t, err, "pseudo_translate must succeed without explicit output_path (issue #636)")
	assert.Equal(t, "pseudo-translate", out.FlowName)
	assert.Equal(t, inputPath, out.InputPath)
	assert.Equal(t, filepath.Join(tmpDir, "content_qps.json"), out.OutputPath)

	// Output file must exist and contain pseudo-translated content.
	content, err := os.ReadFile(out.OutputPath)
	require.NoError(t, err)
	assert.NotEmpty(t, content)
}

func TestHandleRunFlowUnknown(t *testing.T) {
	a := testApp()
	ctx := t.Context()

	fixturePath := filepath.Join("..", "..", "core", "formats", "json", "testdata", "simple.json")
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		t.Skip("test fixture not found:", fixturePath)
	}

	_, _, err := handleRunFlow(ctx, a, RunFlowInput{
		FlowName:   "nonexistent",
		Path:       fixturePath,
		TargetLang: "fr",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown flow")
}

func TestHandleRunFlowWithProject(t *testing.T) {
	a := testApp()
	ctx := t.Context()

	// Create a temp project with a content pattern and a flow.
	dir := t.TempDir()
	inputDir := filepath.Join(dir, "input")
	require.NoError(t, os.MkdirAll(inputDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(inputDir, "test.json"),
		[]byte(`{"greeting": "Hello World"}`),
		0o644,
	))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"qps"},
		},
		Collections: []project.Collection{
			{Path: "input/*.json"},
		},
	}
	kapiPath := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(kapiPath, proj))

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.json")

	// Run pseudo-translate via project mode — should resolve content from project.
	_, out, err := handleRunFlow(ctx, a, RunFlowInput{
		FlowName:   "pseudo-translate",
		Project:    kapiPath,
		OutputPath: outputPath,
	})
	require.NoError(t, err)
	assert.Equal(t, "pseudo-translate", out.FlowName)
	assert.NotEmpty(t, out.InputPath)
	assert.Equal(t, outputPath, out.OutputPath)

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.NotEmpty(t, content)
}

func TestHandleRunFlowWithProjectDefaults(t *testing.T) {
	a := testApp()
	ctx := t.Context()

	dir := t.TempDir()
	inputDir := filepath.Join(dir, "input")
	require.NoError(t, os.MkdirAll(inputDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(inputDir, "test.json"),
		[]byte(`{"msg": "Test"}`),
		0o644,
	))

	// Project provides target language — no need to pass in input.
	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"qps"},
		},
	}
	kapiPath := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(kapiPath, proj))

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "out.json")

	_, out, err := handleRunFlow(ctx, a, RunFlowInput{
		FlowName:   "pseudo-translate",
		Path:       filepath.Join(inputDir, "test.json"),
		Project:    kapiPath,
		OutputPath: outputPath,
	})
	require.NoError(t, err)
	assert.Equal(t, "pseudo-translate", out.FlowName)
}

// TestHandleRunFlowProjectDefinedFlow verifies that MCP can execute a flow
// from the project recipe, including flows outside the built-in registry.
func TestHandleRunFlowProjectDefinedFlow(t *testing.T) {
	a := testApp()
	ctx := t.Context()

	dir := t.TempDir()
	inputDir := filepath.Join(dir, "input")
	require.NoError(t, os.MkdirAll(inputDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(inputDir, "test.json"),
		[]byte(`{"greeting": "Hello World"}`),
		0o644,
	))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"qps"},
		},
		Collections: []project.Collection{{Path: "input/*.json"}},
		Flows: map[string]*flow.StepsSpec{
			"pseudo": {Steps: []flow.FlowStep{{Tool: "pseudo-translate"}}},
		},
	}
	kapiPath := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(kapiPath, proj))

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "out.json")

	_, out, err := handleRunFlow(ctx, a, RunFlowInput{
		FlowName:   "pseudo",
		Project:    kapiPath,
		OutputPath: outputPath,
	})
	require.NoError(t, err)
	assert.Equal(t, "pseudo", out.FlowName)
	assert.NotEmpty(t, out.InputPath)

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.NotEmpty(t, content)
}

// TestHandleRunFlowPlacementRefused verifies the AD-006 transformer placement
// gate runs on MCP flow runs: a project flow that rewrites source after a
// target-producing step is refused, not silently executed. This is the
// redaction-safety hole the porcelain's private tool-building loop left open.
func TestHandleRunFlowPlacementRefused(t *testing.T) {
	a := testApp()
	ctx := t.Context()

	dir := t.TempDir()
	inputDir := filepath.Join(dir, "input")
	require.NoError(t, os.MkdirAll(inputDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(inputDir, "test.json"),
		[]byte(`{"greeting": "Hello World"}`),
		0o644,
	))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"qps"},
		},
		Collections: []project.Collection{{Path: "input/*.json"}},
		Flows: map[string]*flow.StepsSpec{
			// redact rewrites source; placing it after translate — which produces
			// targets and egresses source to a remote sink — is exactly what the
			// gate must reject.
			"leaky": {Steps: []flow.FlowStep{{Tool: "translate"}, {Tool: "redact"}}},
		},
	}
	kapiPath := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(kapiPath, proj))

	_, _, err := handleRunFlow(ctx, a, RunFlowInput{
		FlowName:   "leaky",
		Project:    kapiPath,
		TargetLang: "qps",
		Path:       filepath.Join(inputDir, "test.json"),
	})
	require.Error(t, err, "MCP run of a placement-violating flow must be refused")
	assert.Contains(t, err.Error(), "transformer")
}

func TestHandleExtractContentWithProject(t *testing.T) {
	a := testApp()
	ctx := t.Context()

	// Create a project without okapi plugin and a JSON file.
	dir := t.TempDir()
	inputDir := filepath.Join(dir, "input")
	require.NoError(t, os.MkdirAll(inputDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(inputDir, "test.json"),
		[]byte(`{"msg": "Hello"}`),
		0o644,
	))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		// No plugins — should use built-in "json" format, not "okf_json".
	}
	kapiPath := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(kapiPath, proj))

	_, out, err := handleExtractContent(ctx, a, ExtractContentInput{
		Path:    filepath.Join(inputDir, "test.json"),
		Project: kapiPath,
	})
	require.NoError(t, err)
	assert.Equal(t, "json", out.Format, "should use built-in json, not okf_json")
	assert.NotEmpty(t, out.Blocks)
}
