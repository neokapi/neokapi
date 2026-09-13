package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// declareYAMLComments adds a collection over config/app.yaml to a recipe, with
// or without `comments: true`, and with a target language when targets is set.
func declareYAMLComments(t *testing.T, recipe string, comments, targets bool, body string) {
	t.Helper()
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	item := project.ContentItem{Path: "config/*.yaml", Comments: project.ContentComments{Declared: comments}}
	collection := project.Collection{Name: "config", SourceOnly: !targets}
	if targets {
		item.Target = "config/{lang}/app.yaml"
		item.TargetLanguages = []model.LocaleID{"nb"}
	}
	collection.Content = []project.ContentItem{item}
	proj.Collections = append(proj.Collections, collection)
	require.NoError(t, project.Save(recipe, proj))
	path := filepath.Join(filepath.Dir(recipe), "config", "app.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func findingsOnBlock(g verifyGateResult, block string) []verifyFinding {
	var out []verifyFinding
	for _, f := range g.Findings {
		if f.Block == block {
			out = append(out, f)
		}
	}
	return out
}

// The ship gate reads the comments of every item declared with `comments:
// true`, whatever supplies them: a YAML comment and a Go comment holding the
// same fault produce the same finding, apart from where it is.
func TestShipCheckReadsYAMLCommentsLikeGoComments(t *testing.T) {
	const yamlBody = "# Greets the the reader.\ngreeting: Hello world\n"
	ship := func(t *testing.T, comments, targets bool, body string) verifyOutput {
		t.Helper()
		root, _ := sourceShipFixture(t)
		recipe := filepath.Join(root, "kapi.yaml")
		goFile := declareGoComments(t, recipe, true)
		require.NoError(t, os.WriteFile(goFile, []byte("package code\n\n// Parse reads the the input.\nfunc Parse() {}\n"), 0o644))
		declareYAMLComments(t, recipe, comments, targets, body)
		out, err := (&App{}).computeVerify(sourceShipCommand(t, root), nil)
		require.NoError(t, err)
		return out
	}

	t.Run("a YAML comment and a Go comment produce the same shape of finding", func(t *testing.T) {
		out := ship(t, true, false, yamlBody)
		qa, ok := gateByName(out, gateChecks)
		require.True(t, ok)

		yamlFindings, goFindings := findingsOnBlock(qa, "comment/greeting"), findingsOnBlock(qa, "func/Parse")
		require.Len(t, yamlFindings, 1, "%+v", qa.Findings)
		require.Len(t, goFindings, 1, "%+v", qa.Findings)
		assert.Equal(t, "app.yaml", filepath.Base(yamlFindings[0].File))
		assert.Equal(t, "parse.go", filepath.Base(goFindings[0].File))
		shape := func(f verifyFinding) verifyFinding {
			f.File, f.Block = "", ""
			return f
		}
		assert.Equal(t, shape(goFindings[0]), shape(yamlFindings[0]))
		assert.Equal(t, 3, qa.Coverage.Files, "the JSON content, the Go file and the YAML file")

		run := false
		for _, a := range qa.Execution.Analyzers {
			if a.ID == "comments.yaml" {
				run = true
				require.NotNil(t, a.Canary)
				assert.Equal(t, check.CanaryCaught, a.Canary.Status)
			}
		}
		assert.True(t, run, "the YAML comment extraction is recorded with its canary")
	})

	t.Run("must fail: without comments: true the YAML comment is not read", func(t *testing.T) {
		out := ship(t, false, false, yamlBody)
		qa, ok := gateByName(out, gateChecks)
		require.True(t, ok)
		assert.Empty(t, findingsOnBlock(qa, "comment/greeting"))
		assert.Len(t, findingsOnBlock(qa, "func/Parse"), 1, "the check itself ran")
	})

	t.Run("the comments of a file with targets are read once, as source content", func(t *testing.T) {
		out := ship(t, true, true, yamlBody)
		qa, ok := gateByName(out, gateChecks)
		require.True(t, ok)
		assert.Len(t, findingsOnBlock(qa, "comment/greeting"), 1, "%+v", qa.Findings)
	})

	t.Run("the voice gate governs a declared YAML comment", func(t *testing.T) {
		const guaranteed = "# The deploy is guaranteed safe.\ngreeting: Hello world\n"
		out := ship(t, true, false, guaranteed)
		voice, ok := gateByName(out, gateVoice)
		require.True(t, ok)
		assert.NotEmpty(t, findingsOnBlock(voice, "comment/greeting"), "%+v", voice.Findings)

		without := ship(t, false, false, guaranteed)
		voice, ok = gateByName(without, gateVoice)
		require.True(t, ok)
		assert.Empty(t, findingsOnBlock(voice, "comment/greeting"))
	})
}

// A declared comment layer with no comment prose adds no block, so a checks
// gate over nothing else has nothing to check and did not run.
func TestShipCheckCommentsWithNoProseDidNotRun(t *testing.T) {
	for name, content := range map[string]struct{ item, file, body string }{
		"a Go file with no comment":   {item: "path: \"code/*.go\"\n        comments: true", file: "code/parse.go", body: "package code\n\nfunc Parse() {}\n"},
		"a YAML file with no comment": {item: "path: \"config/*.yaml\"\n        comments: true", file: "config/app.yaml", body: "{}\n"},
	} {
		t.Run(name, func(t *testing.T) {
			isolateCheckExecution(t)
			root := t.TempDir()
			recipe := "version: v1\nname: quiet\ndefaults:\n  source_language: en\ncollections:\n  - name: only\n    source_only: true\n    content:\n      - " + content.item + "\n"
			require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
			require.NoError(t, os.MkdirAll(filepath.Join(root, filepath.Dir(content.file)), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(root, content.file), []byte(content.body), 0o644))

			out, err := (&App{}).computeVerify(sourceShipCommand(t, root), nil)
			require.NoError(t, err)
			qa, ok := gateByName(out, gateChecks)
			require.True(t, ok)
			require.NotNil(t, qa.Coverage)
			assert.Equal(t, 0, qa.Coverage.Blocks)
			assert.Equal(t, check.VerdictDidNotRun, qa.Verdict)
			assert.False(t, out.Pass)
		})
	}
}
