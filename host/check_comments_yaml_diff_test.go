package host

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/golang"
	"github.com/neokapi/neokapi/core/format"
	yamlformat "github.com/neokapi/neokapi/core/formats/yaml"
)

// diffCheckProject runs a diff-scoped check of patch over the project at root.
func diffCheckProject(t *testing.T, root, patch string) check.Report {
	t.Helper()
	writeCheckInput(t, root, "change.diff", patch)
	t.Chdir(root)
	cmd := diffCommand(t)
	cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")
	require.NoError(t, cmd.Flags().Set("diff-file", "change.diff"))
	report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, nil)
	require.NoError(t, err)
	return report
}

// unlocatedYAML stands for a YAML comment provider that reads a file and cannot
// place its comments exactly.
type unlocatedYAML struct{ yamlformat.CommentProvider }

func (unlocatedYAML) Locate(string, []byte) (*comment.File, error) {
	return nil, fmt.Errorf("yaml: %w", comment.ErrUnlocated)
}

// config/app.yaml in onePathProject: line 1 is a comment holding a doubled
// word, line 2 the value its reader extracts.
const (
	yamlCommentLine = "--- a/config/app.yaml\n+++ b/config/app.yaml\n@@ -1 +1 @@\n-# Greets the reader.\n+# Greets the the reader.\n"
	yamlValueLine   = "--- a/config/app.yaml\n+++ b/config/app.yaml\n@@ -2 +2 @@\n-greeting: Hello\n+greeting: Hello world\n"
	yamlBothLines   = "--- a/config/app.yaml\n+++ b/config/app.yaml\n@@ -1,2 +1,2 @@\n-# Greets the reader.\n-greeting: Hello\n+# Greets the the reader.\n+greeting: Hello world\n"
)

// A diff scopes a YAML file whose comments the recipe declares over the union of
// its reader's blocks and its comment blocks: a changed line takes whichever
// block it sits in.
func TestDiffCheckYAMLDeclaredComments(t *testing.T) {
	app := filepath.FromSlash("config/app.yaml")

	t.Run("a change to a comment line checks that comment", func(t *testing.T) {
		report := diffCheckProject(t, onePathProject(t, true, ""), yamlCommentLine)

		entry := scopeEntry(t, report, app)
		assert.Equal(t, check.ScopeChecked, entry.Status)
		assert.Equal(t, []check.ScopeBlock{{Block: "comment/greeting", Lines: format.LineRange{First: 1, Last: 1}}}, entry.Blocks)
		var doubled []check.Diagnostic
		for _, d := range report.Findings {
			if d.Rule == "hygiene.doubled-word" {
				doubled = append(doubled, d)
			}
		}
		require.Len(t, doubled, 1)
		assert.Equal(t, "comment/greeting", doubled[0].Location.Block)
		require.NotNil(t, doubled[0].Location.Lines)
		assert.Equal(t, format.LineRange{First: 1, Last: 1}, *doubled[0].Location.Lines)
		run := analyzerRun(t, report, "comments.yaml")
		require.NotNil(t, run.Canary)
		assert.Equal(t, check.CanaryCaught, run.Canary.Status)
	})

	t.Run("a change to a value line checks only the reader's block", func(t *testing.T) {
		report := diffCheckProject(t, onePathProject(t, true, ""), yamlValueLine)

		entry := scopeEntry(t, report, app)
		assert.Equal(t, check.ScopeChecked, entry.Status)
		require.Len(t, entry.Blocks, 1)
		assert.False(t, strings.HasPrefix(entry.Blocks[0].Block, "comment/"), entry.Blocks[0].Block)
		assert.Equal(t, format.LineRange{First: 2, Last: 2}, entry.Blocks[0].Lines)
		for _, d := range report.Findings {
			assert.NotEqual(t, "comment/greeting", d.Location.Block, d.Rule)
		}
	})

	t.Run("a change over both lines checks the comment and the value", func(t *testing.T) {
		report := diffCheckProject(t, onePathProject(t, true, ""), yamlBothLines)

		entry := scopeEntry(t, report, app)
		require.Len(t, entry.Blocks, 2)
		keys := []string{entry.Blocks[0].Block, entry.Blocks[1].Block}
		assert.Contains(t, keys, "comment/greeting")
	})

	t.Run("must fail: without comments: true a change to a comment line is untouched", func(t *testing.T) {
		report := diffCheckProject(t, onePathProject(t, false, ""), yamlCommentLine)
		assert.Equal(t, check.ScopeUntouched, scopeEntry(t, report, app).Status)
		assert.Empty(t, report.Findings)
	})

	t.Run("comments: true on a format that supplies no comments did not run", func(t *testing.T) {
		root := onePathProject(t, false, `  - name: data
    source_only: true
    content:
      - path: "data/*.json"
        comments: true
`)
		require.NoError(t, os.MkdirAll(filepath.Join(root, "data"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(root, "data", "copy.json"), []byte("{\"title\":\"Ready.\"}\n"), 0o600))
		report := diffCheckProject(t, root, "--- a/data/copy.json\n+++ b/data/copy.json\n@@ -1 +1 @@\n-{\"title\":\"Set.\"}\n+{\"title\":\"Ready.\"}\n")

		run := analyzerRun(t, report, "comments.json")
		assert.Equal(t, check.AnalyzerDidNotRun, run.Status)
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
		assert.Equal(t, check.CauseContentNotChecked, report.DidNotRunCause)
	})

	t.Run("comments a provider cannot place leave the file not run, even with no value in it", func(t *testing.T) {
		saved := commentProviders
		r := comment.NewRegistry(golang.Provider{})
		r.RegisterFormat("yaml", unlocatedYAML{})
		commentProviders = r
		t.Cleanup(func() { commentProviders = saved })

		root := onePathProject(t, true, "")
		require.NoError(t, os.WriteFile(filepath.Join(root, "config", "app.yaml"), []byte("# Only a comment.\n"), 0o600))
		report := diffCheckProject(t, root, "--- a/config/app.yaml\n+++ b/config/app.yaml\n@@ -1 +1 @@\n-# A comment.\n+# Only a comment.\n")

		entry := scopeEntry(t, report, app)
		assert.Equal(t, check.ScopeDidNotRun, entry.Status)
		assert.Contains(t, entry.Reason, "could not be located")
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict)

		whole := checkProject(t, root)
		assert.Equal(t, check.AnalyzerDidNotRun, analyzerRun(t, whole, "comments.yaml").Status)
		assert.Equal(t, check.VerdictDidNotRun, whole.Verdict)
	})
}

// A diff-scoped check records the same analyzers for a YAML file with declared
// comments as a whole-file check of the project, in the same order and with the
// same outcomes. Reader validation is left out: a diff-scoped check refuses
// --validate.
func TestDiffCheckYAMLCommentAnalyzersMatchAWholeFileCheck(t *testing.T) {
	type analyzer struct {
		ID       string
		Status   check.AnalyzerStatus
		Required bool
		Canary   check.CanaryStatus
	}
	analyzersOf := func(report check.Report) []analyzer {
		require.NotNil(t, report.Execution)
		var out []analyzer
		for _, run := range report.Execution.Analyzers {
			if filepath.Base(run.File) != "app.yaml" || run.ID == "reader.validation" {
				continue
			}
			a := analyzer{ID: run.ID, Status: run.Status, Required: run.Required}
			if run.Canary != nil {
				a.Canary = run.Canary.Status
			}
			out = append(out, a)
		}
		return out
	}

	root := onePathProject(t, true, "")
	diff := diffCheckProject(t, root, yamlCommentLine)
	whole := checkProject(t, root)

	scoped := analyzersOf(diff)
	ids := make([]string, len(scoped))
	for i, a := range scoped {
		ids[i] = a.ID
	}
	assert.Subset(t, ids, []string{"comments.yaml", "hygiene"})
	assert.Equal(t, analyzersOf(whole), scoped)
}
