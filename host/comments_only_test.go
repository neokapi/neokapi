package host

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// An item declared with `comments: {only: true}` contributes its comments to
// the checks and nothing else. Each test here holds one surface that reads
// values to leaving such a file's values alone, beside a must-fail case where
// `comments: true` keeps the same values as content.

const (
	onlyComments   = "{only: true}"
	besideComments = "true"
)

// spelledComments decodes an item's `comments:` from its recipe spelling.
func spelledComments(t *testing.T, spelled string) project.ContentComments {
	t.Helper()
	var c project.ContentComments
	require.NoError(t, yaml.Unmarshal([]byte(spelled), &c))
	return c
}

// appendYAMLCollection declares config/*.yaml in a recipe written as text, with
// `comments:` spelled as given, and writes config/app.yaml. The value holds a
// retired term, a prohibited assurance and a doubled word; the comment holds a
// doubled word.
func appendYAMLCollection(t *testing.T, root, spelled, tail string) string {
	t.Helper()
	recipe := filepath.Join(root, "kapi.yaml")
	data, err := os.ReadFile(recipe)
	require.NoError(t, err)
	data = append(data, []byte(`  - name: config
    source_only: true
    content:
      - path: "config/*.yaml"
        comments: `+spelled+"\n"+tail)...)
	require.NoError(t, os.WriteFile(recipe, data, 0o644))
	path := filepath.Join(root, "config", "app.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("# Greets the the reader.\ngreeting: ForbiddenName is guaranteed safe for the the reader.\n"), 0o644))
	return path
}

// valueFindings are a gate's findings on the YAML file's values.
func valueFindings(g verifyGateResult) []verifyFinding {
	var out []verifyFinding
	for _, f := range g.Findings {
		if filepath.Base(f.File) == "app.yaml" && !strings.HasPrefix(f.Block, "comment/") {
			out = append(out, f)
		}
	}
	return out
}

// The ship gates read a comments-only item's comments and none of its values,
// over the project's content and over the file named on its own.
func TestShipGatesReadNoValueOfACommentsOnlyItem(t *testing.T) {
	ship := func(t *testing.T, spelled string, named bool) verifyOutput {
		t.Helper()
		root, _ := sourceShipFixture(t)
		path := appendYAMLCollection(t, root, spelled, "")
		var args []string
		if named {
			args = []string{filepath.Join(root, "content.json"), path}
		}
		out, err := (&App{}).computeVerify(sourceShipCommand(t, root), args)
		require.NoError(t, err)
		return out
	}
	gate := func(t *testing.T, out verifyOutput, name string) verifyGateResult {
		t.Helper()
		g, ok := gateByName(out, name)
		require.True(t, ok, "the %s gate ran", name)
		return g
	}

	for name, named := range map[string]bool{"the project's content": false, "named files": true} {
		t.Run(name, func(t *testing.T) {
			only, beside := ship(t, onlyComments, named), ship(t, besideComments, named)

			for _, g := range []string{gateChecks, gateVoice, gateTerms} {
				assert.Empty(t, valueFindings(gate(t, only, g)), "%s: a comments-only item has no value to check", g)
				assert.NotEmpty(t, valueFindings(gate(t, beside, g)), "must fail: %s reads the values beside comments: true", g)
			}
			assert.Len(t, findingsOnBlock(gate(t, only, gateChecks), "comment/greeting"), 1, "the comment is checked")
			assert.Len(t, findingsOnBlock(gate(t, beside, gateChecks), "comment/greeting"), 1)

			onlyChecks, besideChecks := gate(t, only, gateChecks), gate(t, beside, gateChecks)
			require.NotNil(t, onlyChecks.Coverage)
			require.NotNil(t, besideChecks.Coverage)
			assert.Equal(t, besideChecks.Coverage.Blocks-1, onlyChecks.Coverage.Blocks, "the one value is the only block the gate no longer counts")
		})
	}
}

// The source gate over named files measures the units those files resolve to,
// and a comments-only file's unit holds no value to measure.
func TestNamedSourceGateMeasuresNoValueOfACommentsOnlyItem(t *testing.T) {
	total := func(t *testing.T, spelled string) int {
		t.Helper()
		root, content := sourceShipFixture(t)
		path := appendYAMLCollection(t, root, spelled, "")
		proj, err := project.Load(filepath.Join(root, "kapi.yaml"))
		require.NoError(t, err)
		a := &App{}
		a.InitRegistries()
		units, err := a.unitsFromArgs(proj, root, []string{content, path}, "")
		require.NoError(t, err)
		readiness, err := a.computeSourceReadiness(t.Context(), proj, root, units)
		require.NoError(t, err)
		return readiness.Total
	}

	assert.Equal(t, 2, total(t, onlyComments), "the JSON file's two values")
	assert.Equal(t, 3, total(t, besideComments), "must fail: comments: true measures the YAML value")
}

// commentsOnlyStatusProject is a project with a JSON catalog translated into
// Norwegian and a YAML file declared with `comments:` spelled as given.
func commentsOnlyStatusProject(t *testing.T, spelled string) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()
	recipe := `version: v1
name: src
defaults:
  source_language: en
  target_languages: [nb]
collections:
  - path: en.json
    target: "{lang}.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"), []byte(`{"a":"Apple","b":"Banana","c":"Cherry"}`), 0o644))
	appendYAMLCollection(t, root, spelled, "source_gate: { written: 100 }\n")
	readProjectContext(t, root)
	return root
}

// `kapi status` counts no value of a comments-only item in source coverage.
func TestStatusCountsNoValueOfACommentsOnlyItem(t *testing.T) {
	t.Run("a comments-only item", func(t *testing.T) {
		t.Chdir(commentsOnlyStatusProject(t, onlyComments))
		out := runStatusJSON(t)
		require.NotNil(t, out.Source)
		assert.Equal(t, 3, out.Source.Total, "the JSON catalog's three values alone")
		nb, ok := localeCoverage(out, "nb")
		require.True(t, ok)
		assert.Equal(t, 3, nb.Total)
	})

	t.Run("must fail: comments: true counts the value", func(t *testing.T) {
		t.Chdir(commentsOnlyStatusProject(t, besideComments))
		out := runStatusJSON(t)
		require.NotNil(t, out.Source)
		assert.Equal(t, 4, out.Source.Total)
	})

	t.Run("source units hold no comments-only file", func(t *testing.T) {
		units := func(t *testing.T, spelled string) []string {
			t.Helper()
			root := commentsOnlyStatusProject(t, spelled)
			proj, err := project.Load(filepath.Join(root, "kapi.yaml"))
			require.NoError(t, err)
			found, err := appWithFormats().SourceUnitsFromProject(proj, root)
			require.NoError(t, err)
			var files []string
			for _, u := range found {
				files = append(files, filepath.Base(u.SourcePath))
			}
			return files
		}
		assert.Equal(t, []string{"en.json"}, units(t, onlyComments), "coverage settles no unit of a comments-only file")
		assert.ElementsMatch(t, []string{"en.json", "app.yaml"}, units(t, besideComments), "must fail: comments: true gives the file a source unit")
	})
}

// The plan prices the target units of a project. A comments-only item has none,
// even one built in memory with a target no recipe could load.
func TestUpPlanPricesNoValueOfACommentsOnlyItem(t *testing.T) {
	plan := func(t *testing.T, spelled string) UpPlanOutput {
		t.Helper()
		a, recipe := newPlanProject(t)
		root := filepath.Dir(recipe)
		require.NoError(t, os.MkdirAll(filepath.Join(root, "config"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "config", "app.yaml"), []byte("# Greets the reader.\ngreeting: Hello world\n"), 0o644))
		proj, err := project.Load(recipe)
		require.NoError(t, err)
		proj.Collections = append(proj.Collections, project.Collection{
			Name:    "config",
			Content: []project.ContentItem{{Path: "config/*.yaml", Target: "config/{lang}/app.yaml", Comments: spelledComments(t, spelled)}},
		})
		out, err := a.computeProjectPlan(context.Background(), proj, recipe)
		require.NoError(t, err)
		return out
	}
	collections := func(out UpPlanOutput) []string {
		var names []string
		for _, s := range out.Scopes {
			names = append(names, s.Collection)
		}
		return names
	}

	assert.Equal(t, []string{"app"}, collections(plan(t, onlyComments)))
	assert.ElementsMatch(t, []string{"app", "config"}, collections(plan(t, besideComments)), "must fail: comments: true prices the values")
}

// commentsOnlyYAMLProject is a project whose only content is a YAML file
// declared with `comments:` spelled as given, with a French target language
// and a flow that reaches no provider.
func commentsOnlyYAMLProject(t *testing.T, spelled string) (*App, string, *project.KapiProject) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "config"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config", "app.yaml"), []byte("# Greets the reader.\ngreeting: Hello world\n"), 0o644))
	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "CommentsOnly",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr"},
			Flow:            "recycle-only",
			SourceGate:      string(model.SourceGateNone),
		},
		Collections: []project.Collection{{
			Name:    "config",
			Content: []project.ContentItem{{Path: "config/*.yaml", Comments: spelledComments(t, spelled)}},
		}},
		Flows: map[string]*flow.StepsSpec{
			"recycle-only": {Steps: []flow.FlowStep{{Tool: "recycle", Config: map[string]any{"fillTarget": true}}}},
		},
	}
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	require.NoError(t, project.EnsureLayout(layout))

	a := &App{}
	a.InitRegistries()
	a.SourceLang = "en"
	a.Quiet = true
	return a, recipe, proj
}

// notNothingToDo holds a run over a comments-only item to reporting that it had
// nothing to read, and the same run over `comments: true` to reading the file.
func notNothingToDo(t *testing.T, want string, run func(t *testing.T, spelled string) error) {
	t.Helper()
	t.Run("a comments-only item gives the run nothing to read", func(t *testing.T) {
		err := run(t, onlyComments)
		require.Error(t, err)
		assert.Contains(t, err.Error(), want)
	})
	t.Run("must fail: comments: true gives the run the values", func(t *testing.T) {
		if err := run(t, besideComments); err != nil {
			assert.NotContains(t, err.Error(), want)
		}
	})
}

// `kapi up` converges nothing in a comments-only item.
func TestUpConvergesNoValueOfACommentsOnlyItem(t *testing.T) {
	notNothingToDo(t, "no content to catch up", func(t *testing.T, spelled string) error {
		a, recipe, proj := commentsOnlyYAMLProject(t, spelled)
		cmd := NewEnvCommand(context.Background(), "up")
		a.AddFlowRunFlags(cmd)
		AddUpFlags(cmd)
		AddProjectFlag(cmd)
		require.NoError(t, cmd.Flags().Set("project", recipe))
		return a.RunDefaultFlowConverge(cmd, proj, recipe, ConvergeOptions{UntilGate: true, MaxPasses: 3, noChecks: true})
	})
}

// `kapi run` runs a flow over no value of a comments-only item.
func TestRunReadsNoValueOfACommentsOnlyItem(t *testing.T) {
	notNothingToDo(t, "no input files found", func(t *testing.T, spelled string) error {
		a, recipe, _ := commentsOnlyYAMLProject(t, spelled)
		cmd := NewEnvCommand(context.Background(), "pseudo-translate")
		fs := cmd.Flags()
		for _, name := range []string{"source-lang", "output", "encoding", "trace", "format"} {
			fs.String(name, "", "")
		}
		fs.String("target-lang", "qps", "")
		fs.StringSlice("input", nil, "")
		fs.Int("concurrency", 0, "")
		fs.Bool("explain", false, "")
		require.NoError(t, fs.Set("output", filepath.Join(t.TempDir(), "out.yaml")))
		a.TargetLang = "qps"
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		return a.RunFromProject(cmd, "pseudo-translate", recipe, RunCmdOptions{})
	})
}

// `kapi merge --materialize` writes no target for a comments-only item.
func TestMergeMaterializesNoValueOfACommentsOnlyItem(t *testing.T) {
	notNothingToDo(t, "has no source files to materialize", func(t *testing.T, spelled string) error {
		a, recipe, proj := commentsOnlyYAMLProject(t, spelled)
		_, err := a.materializeProject(context.Background(), io.Discard, proj, recipe, []model.LocaleID{"fr"}, true, nil)
		return err
	})
}

// `kapi extract` packages no value of a comments-only item.
func TestExtractPackagesNoValueOfACommentsOnlyItem(t *testing.T) {
	extractCommand := func(t *testing.T, recipe string) *EnvCommand {
		t.Helper()
		cmd := NewEnvCommand(t.Context(), "extract")
		cmd.SetErr(&bytes.Buffer{})
		addKpzExtractFlags(cmd, recipe)
		require.NoError(t, cmd.Flags().Set("out-dir", t.TempDir()))
		return cmd
	}
	t.Run("kpz", func(t *testing.T) {
		notNothingToDo(t, "no source files matched", func(t *testing.T, spelled string) error {
			a, recipe, _ := commentsOnlyYAMLProject(t, spelled)
			return a.RunExtractKpz(extractCommand(t, recipe))
		})
	})
	t.Run("xliff", func(t *testing.T) {
		notNothingToDo(t, "no source files matched", func(t *testing.T, spelled string) error {
			a, recipe, _ := commentsOnlyYAMLProject(t, spelled)
			cmd := extractCommand(t, recipe)
			cmd.Flags().String("format", ExtractFormatXLIFF2, "")
			cmd.Flags().String("xliff-version", "", "")
			cmd.Flags().Bool("force", false, "")
			cmd.Flags().Bool("redact", false, "")
			cmd.Flags().String("redact-rules", "", "")
			return a.RunExtract(cmd)
		})
	})
}

// `kapi stats` with no file measures a project's content, which holds no value
// of a comments-only item.
func TestStatsMeasuresNoValueOfACommentsOnlyItem(t *testing.T) {
	stats := func(t *testing.T, spelled string) []string {
		t.Helper()
		a, recipe, _ := commentsOnlyYAMLProject(t, spelled)
		root := filepath.Dir(recipe)
		require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"), []byte(`{"a":"Apple"}`), 0o644))
		proj, err := project.Load(recipe)
		require.NoError(t, err)
		proj.Collections = append(proj.Collections, project.Collection{Name: "app", Path: "en.json"})
		require.NoError(t, project.Save(recipe, proj))
		t.Chdir(root)

		cmd := NewEnvCommand(t.Context(), "stats")
		AddProjectFlag(cmd)
		require.NoError(t, cmd.Flags().Set("project", recipe))
		cmd.Flags().Bool("json", true, "")
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&bytes.Buffer{})
		require.NoError(t, a.RunStats(cmd, nil))
		var parsed StatsOutput
		require.NoError(t, json.Unmarshal(out.Bytes(), &parsed), out.String())
		var files []string
		for _, f := range parsed.Files {
			files = append(files, filepath.Base(f.File))
		}
		return files
	}

	assert.Equal(t, []string{"en.json"}, stats(t, onlyComments))
	assert.ElementsMatch(t, []string{"en.json", "app.yaml"}, stats(t, besideComments), "must fail: comments: true measures the values")
}
