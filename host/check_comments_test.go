package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func analyzerStatus(report check.Report, id string) check.AnalyzerStatus {
	if report.Execution == nil {
		return ""
	}
	for _, run := range report.Execution.Analyzers {
		if run.ID == id {
			return run.Status
		}
	}
	return ""
}

func TestCheckReadsTheCommentsOfANamedGoFile(t *testing.T) {
	isolateCheckExecution(t)
	file := filepath.Join(t.TempDir(), "parse.go")
	require.NoError(t, os.WriteFile(file, []byte("package demo\n\n// Parse reads the the input.\nfunc Parse() {}\n"), 0o600))

	report, err := (&App{SourceLang: "en"}).ComputeCheck(executionCommand(t), []string{file})
	require.NoError(t, err)
	assert.Equal(t, 1, report.Target.Blocks, "the one comment is the one block")
	require.Len(t, report.Findings, 1)
	assert.Equal(t, "hygiene.doubled-word", report.Findings[0].Rule)
	assert.Equal(t, "func/Parse", report.Findings[0].Location.Block)
	require.NotNil(t, report.Findings[0].Location.Lines, "a comment finding names the lines of its comment")
	assert.Equal(t, format.LineRange{First: 3, Last: 3}, *report.Findings[0].Location.Lines)
	assert.Equal(t, check.AnalyzerPassed, analyzerStatus(report, "formatter.gofmt"), "gofmt ran and agreed")
}

// commentProjectFixture is a project whose Go sources are declared for their
// comments at two points of one profile, beside ordinary JSON content. The
// profile prohibits "utilize" everywhere except the docs channel, so the two
// Go files are held to different rules only if each comment resolves the
// governance of its own file's point.
func commentProjectFixture(t *testing.T) string {
	t.Helper()
	isolateCheckExecution(t)
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	write("kapi.yaml", `version: v1
name: code-comments
defaults:
  source_language: en
profiles:
  service:
    channels: [code, docs]
collections:
  - name: code
    channel: service/code
    source_only: true
    content:
      - path: "src/*.go"
        comments: true
  - name: guide
    channel: service/docs
    source_only: true
    content:
      - path: "guide/*.go"
        comments: true
  - name: copy
    content:
      - path: copy.json
`)
	write(".kapi/voice.yaml", `name: Service
constraints:
  - id: service/plain-words
    version: 1
    source: service-guide.md
    statement: Say use rather than utilize.
    kind: prohibited_pattern
    regex: '(?i)\butilize\b'
    exceptions:
      - scope:
          channel: docs
        reason: The style guide quotes the word it retires.
        approved_by: fixture
        approval_ref: service-guide.md#utilize
`)
	// The prohibited word sits in a comment and in a string literal. Only the
	// comment is content.
	write("src/parse.go", "package demo\n\n// Parse helps you utilize the input.\nfunc Parse() string { return \"utilize\" }\n")
	write("guide/quote.go", "package guide\n\n// Quote shows the retired word utilize in context.\nfunc Quote() {}\n")
	write("copy.json", `{"title":"Ready."}`)
	readProjectContext(t, root)
	return root
}

func TestCheckGovernsGoCommentsAtTheirPoint(t *testing.T) {
	root := commentProjectFixture(t)
	cmd := executionCommand(t)
	cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")

	report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, nil)
	require.NoError(t, err)
	assert.Equal(t, 3, report.Target.Blocks, "two comments and one JSON value")

	var voice []check.Diagnostic
	for _, d := range report.Findings {
		if d.Check == "voice" {
			voice = append(voice, d)
		}
	}
	require.Len(t, voice, 1, "the code comment is held to the rule, the docs comment is excepted, and the string literal is not content")
	assert.Equal(t, "parse.go", filepath.Base(voice[0].Location.File))
	assert.Equal(t, "func/Parse", voice[0].Location.Block)
	require.NotNil(t, voice[0].Location.Lines)
	assert.Equal(t, format.LineRange{First: 3, Last: 3}, *voice[0].Location.Lines)

	channels := map[string]string{}
	for _, c := range report.Execution.Contexts {
		channels[filepath.Base(c.File)] = c.Voice.Channel
		assert.True(t, c.Voice.Applied, "%s resolved no voice", c.File)
	}
	assert.Equal(t, map[string]string{"parse.go": "code", "quote.go": "docs", "copy.json": ""}, channels,
		"each comment resolves the voice of its own file's point")
}

func TestCheckGofmtDisagreementFailsTheGate(t *testing.T) {
	isolateCheckExecution(t)
	file := filepath.Join(t.TempDir(), "parse.go")
	require.NoError(t, os.WriteFile(file, []byte("package demo\n\nfunc Parse() {\n  // Indented with spaces.\n\t_ = 1\n}\n"), 0o600))

	report, err := (&App{SourceLang: "en"}).ComputeCheck(executionCommand(t), []string{file})
	require.NoError(t, err)
	require.Len(t, report.Findings, 1)
	f := report.Findings[0]
	assert.Equal(t, "formatter.gofmt", f.Rule)
	assert.True(t, f.Fails)
	assert.Equal(t, "func/Parse/comment", f.Location.Block)
	require.NotNil(t, f.Location.Lines)
	assert.Equal(t, format.LineRange{First: 4, Last: 4}, *f.Location.Lines)
	assert.Contains(t, f.Suggestion, "// Indented with spaces.")
	assert.False(t, report.Pass, "a comment gofmt would rewrite does not pass")
	assert.True(t, f.Fails, "a formatter's disagreement fails")
	assert.Equal(t, "formatter.gofmt", f.Rule)
}

func TestSourceUnitsLeaveCommentsOnlyFilesAlone(t *testing.T) {
	root := commentProjectFixture(t)
	proj, err := project.Load(filepath.Join(root, "kapi.yaml"))
	require.NoError(t, err)

	units, err := appWithFormats().SourceUnitsFromProject(proj, root)
	require.NoError(t, err)
	var paths []string
	for _, u := range units {
		paths = append(paths, filepath.Base(u.SourcePath))
	}
	assert.Equal(t, []string{"copy.json"}, paths, "source coverage has no unit to settle in a comment")
}
