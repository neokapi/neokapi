package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// onePathProject declares a YAML file and a Go file whose comments hold the
// same fault. The Go file is read for its comments alone; the YAML file is read
// by its reader, and its comments are content when yamlComments is set.
func onePathProject(t *testing.T, yamlComments bool, extra string) string {
	t.Helper()
	isolateCheckExecution(t)
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	flag := "false"
	if yamlComments {
		flag = "true"
	}
	write("kapi.yaml", `version: v1
name: one-path
defaults:
  source_language: en
collections:
  - name: config
    source_only: true
    content:
      - path: "config/*.yaml"
        comments: `+flag+`
  - name: code
    source_only: true
    content:
      - path: "code/*.go"
        comments: true
`+extra)
	write("config/app.yaml", "# Greets the the reader.\ngreeting: Hello world\n")
	write("code/parse.go", "package code\n\n// Parse reads the the input.\nfunc Parse() {}\n")
	return root
}

func checkProject(t *testing.T, root string) check.Report {
	t.Helper()
	cmd := executionCommand(t)
	cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")
	report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, nil)
	require.NoError(t, err)
	return report
}

// The comment layer is one layer with two providers: the YAML reader's side
// supplies a YAML file's comments, a language provider a Go file's. Everything
// after extraction is shared, so the same fault in each produces the same
// finding, apart from where it is.
func TestCommentsOfYAMLAndGoShareOnePath(t *testing.T) {
	t.Run("a YAML comment and a Go comment produce the same shape of finding", func(t *testing.T) {
		report := checkProject(t, onePathProject(t, true, ""))
		assert.NotEqual(t, check.VerdictDidNotRun, report.Verdict, report.DidNotRun)

		byExt := map[string]check.Diagnostic{}
		for _, d := range report.Findings {
			if d.Rule == "hygiene.doubled-word" {
				byExt[filepath.Ext(d.Location.File)] = d
			}
		}
		require.Contains(t, byExt, ".yaml", "the YAML comment was checked")
		require.Contains(t, byExt, ".go", "the Go comment was checked")
		yamlFinding, goFinding := byExt[".yaml"], byExt[".go"]

		assert.Equal(t, "comment/greeting", yamlFinding.Location.Block)
		assert.Equal(t, "func/Parse", goFinding.Location.Block)
		require.NotNil(t, yamlFinding.Location.Lines)
		require.NotNil(t, goFinding.Location.Lines)
		assert.Equal(t, format.LineRange{First: 1, Last: 1}, *yamlFinding.Location.Lines)
		assert.Equal(t, format.LineRange{First: 3, Last: 3}, *goFinding.Location.Lines)
		assert.Equal(t, yamlFinding.Location.Anchor != nil, goFinding.Location.Anchor != nil)

		// Where a finding is differs by construction; everything else is the
		// shared path's.
		shape := func(d check.Diagnostic) check.Diagnostic {
			d.Location = check.Location{Snippet: d.Location.Snippet}
			return d
		}
		assert.Equal(t, shape(goFinding), shape(yamlFinding))

		for _, id := range []string{"comments.yaml", "comments.go"} {
			run := analyzerRun(t, report, id)
			require.NotNil(t, run.Canary, id)
			assert.Equal(t, check.CanaryCaught, run.Canary.Status, id)
		}
	})

	t.Run("must fail: without comments: true the YAML comment is not checked", func(t *testing.T) {
		report := checkProject(t, onePathProject(t, false, ""))
		goFindings := 0
		for _, d := range report.Findings {
			assert.NotEqual(t, ".yaml", filepath.Ext(d.Location.File), "%s on %s", d.Rule, d.Location.Block)
			if filepath.Ext(d.Location.File) == ".go" {
				goFindings++
			}
		}
		assert.Equal(t, 1, goFindings, "the check itself ran")
	})

	t.Run("comments: true adds only the comment blocks to what check reads", func(t *testing.T) {
		without := checkProject(t, onePathProject(t, false, ""))
		with := checkProject(t, onePathProject(t, true, ""))
		assert.Equal(t, without.Target.Blocks+1, with.Target.Blocks, "the reader's block stays and the one YAML comment joins it")
	})

	t.Run("comments: true on a format that supplies no comments did not run", func(t *testing.T) {
		root := onePathProject(t, false, `  - name: data
    source_only: true
    content:
      - path: "data/*.json"
        comments: true
`)
		require.NoError(t, os.MkdirAll(filepath.Join(root, "data"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(root, "data", "copy.json"), []byte(`{"title":"Ready."}`), 0o600))
		report := checkProject(t, root)
		run := analyzerRun(t, report, "comments.json")
		assert.Equal(t, check.AnalyzerDidNotRun, run.Status)
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict, "a declared check that cannot run is never a pass")
	})
}
