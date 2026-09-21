package host

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/golang"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProseP2_go is the P2 rung for Go comments: comments take part in `kapi
// check` at their file's point, a finding carries the lines of its comment, a
// comment gofmt would rewrite fails the gate, a file with nothing to check did
// not run, the recipe declaration is never converged, and the comment layer and
// its formatter each catch a canary on every run.
//
// The subtests named "must fail" break one of those on purpose and assert that
// the run notices. Nothing in here skips.
func TestProseP2_go(t *testing.T) {
	t.Run("a comment is held to the governance of its file's point", func(t *testing.T) {
		root := commentProjectFixture(t)
		cmd := executionCommand(t)
		cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")
		report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, nil)
		require.NoError(t, err)
		assert.NotEqual(t, check.VerdictDidNotRun, report.Verdict, report.DidNotRun)

		voice := findingsOf(report, "voice")
		require.Len(t, voice, 1, "the code comment is held to the rule and the docs comment is excepted")
		assert.Equal(t, "parse.go", filepath.Base(voice[0].Location.File))
		assert.Equal(t, "func/Parse", voice[0].Location.Block)
		require.NotNil(t, voice[0].Location.Lines)
		assert.Equal(t, format.LineRange{First: 3, Last: 3}, *voice[0].Location.Lines)
	})

	t.Run("a comment gofmt would rewrite fails the gate", func(t *testing.T) {
		file := goFile(t, "package demo\n\nfunc Parse() {\n  // Indented with spaces.\n\t_ = 1\n}\n")
		report, err := (&App{SourceLang: "en"}).ComputeCheck(executionCommand(t), []string{file})
		require.NoError(t, err)
		assert.Equal(t, check.VerdictFailed, report.Verdict)
		require.Len(t, findingsOf(report, formatterCheck), 1)

		lenient := executionCommand(t)
		lenient.Flags().Bool("lenient", true, "")
		report, err = (&App{SourceLang: "en"}).ComputeCheck(lenient, []string{file})
		require.NoError(t, err)
		assert.Equal(t, check.VerdictPassed, report.Verdict, report.DidNotRun)
	})

	t.Run("the comment layer and its formatter catch their canaries", func(t *testing.T) {
		file := goFile(t, "package demo\n\n// Parse parses the input.\nfunc Parse() {}\n")
		report, err := (&App{SourceLang: "en"}).ComputeCheck(executionCommand(t), []string{file})
		require.NoError(t, err)
		assert.Equal(t, check.VerdictPassed, report.Verdict, report.DidNotRun)
		for _, id := range []string{"comments.go", "formatter.gofmt"} {
			run := analyzerRun(t, report, id)
			assert.Equal(t, check.AnalyzerPassed, run.Status, id)
			require.NotNil(t, run.Canary, id)
			assert.Equal(t, check.CanaryCaught, run.Canary.Status, id)
		}
	})

	t.Run("a Go file with no comment prose did not run", func(t *testing.T) {
		file := goFile(t, "package demo\n\n//go:noinline\nfunc Parse() {}\n")
		report, err := (&App{SourceLang: "en"}).ComputeCheck(executionCommand(t), []string{file})
		require.NoError(t, err)
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict, "zero comments checked is never a pass")
		assert.False(t, report.Pass)
	})

	t.Run("the declaration is never converged", func(t *testing.T) {
		a, cmd, recipe := newFreshCheckoutProject(t)
		goSource := declareGoComments(t, recipe, true)
		require.NoError(t, cmd.Flags().Set("fail-on-unknown", "true"))
		proj, err := project.Load(recipe)
		require.NoError(t, err)
		require.NoError(t, a.RunDefaultFlowConverge(cmd, proj, recipe, ConvergeOptions{UntilGate: true, MaxPasses: 3, noChecks: true}))
		matches, err := filepath.Glob(filepath.Join(filepath.Dir(goSource), "*"))
		require.NoError(t, err)
		assert.Equal(t, []string{goSource}, matches)
	})

	t.Run("a comment gofmt -s would change is a major finding that fails the gate", func(t *testing.T) {
		for name, src := range map[string]string{
			"misindented continuation": "package demo\n\nfunc Parse() {\n\t// Parse reads the input,\n\t  // one line at a time.\n\t_ = 1\n}\n",
			"trailing space":           "package demo\n\n// Parse reads the input. \n// It stops at the end.\nfunc Parse() {}\n",
		} {
			t.Run(name, func(t *testing.T) {
				report, err := (&App{SourceLang: "en"}).ComputeCheck(executionCommand(t), []string{goFile(t, src)})
				require.NoError(t, err)
				formatter := findingsOf(report, formatterCheck)
				require.Len(t, formatter, 1)
				assert.Equal(t, "formatter.gofmt", formatter[0].Rule)
				assert.Equal(t, check.SeverityMajor, formatter[0].Severity)
				assert.Equal(t, check.VerdictFailed, report.Verdict)
			})
		}
	})

	t.Run("a formatter that cannot compare the file did not run", func(t *testing.T) {
		swapCommentProviders(t, failingFormatterProvider{})
		file := goFile(t, "package demo\n\n// Parse parses the input.\nfunc Parse() {}\n")
		report, err := (&App{SourceLang: "en"}).ComputeCheck(executionCommand(t), []string{file})
		require.NoError(t, err)
		run := analyzerRun(t, report, "formatter.gofmt")
		assert.Equal(t, check.AnalyzerDidNotRun, run.Status)
		assert.Contains(t, run.Reason, "could not compare")
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
	})

	t.Run("must fail: an inert hygiene checker invalidates a comment run", func(t *testing.T) {
		saved := hygieneTool
		hygieneTool = func() BlockProcessor {
			return &tool.BaseTool{ToolName: "inert", Annotate: func(tool.BlockView) error { return nil }}
		}
		t.Cleanup(func() { hygieneTool = saved })
		file := goFile(t, "package demo\n\n// Parse parses the input.\nfunc Parse() {}\n")
		report, err := (&App{SourceLang: "en"}).ComputeCheck(executionCommand(t), []string{file})
		require.NoError(t, err)
		assert.Equal(t, check.AnalyzerInvalid, analyzerRun(t, report, "comments.go").Status)
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
	})

	t.Run("must fail: a provider that drops prose beside a directive invalidates the run", func(t *testing.T) {
		swapCommentProviders(t, mixedGroupDroppingProvider{})
		file := goFile(t, "package demo\n\n// Parse parses the input.\nfunc Parse() {}\n")
		report, err := (&App{SourceLang: "en"}).ComputeCheck(executionCommand(t), []string{file})
		require.NoError(t, err)
		assert.Equal(t, check.AnalyzerInvalid, analyzerRun(t, report, "comments.go").Status)
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
	})

	t.Run("must fail: a formatter that never disagrees invalidates the run", func(t *testing.T) {
		swapCommentProviders(t, agreeableFormatterProvider{})
		file := goFile(t, "package demo\n\n// Parse parses the input.\nfunc Parse() {}\n")
		report, err := (&App{SourceLang: "en"}).ComputeCheck(executionCommand(t), []string{file})
		require.NoError(t, err)
		assert.Equal(t, check.AnalyzerInvalid, analyzerRun(t, report, "formatter.gofmt").Status)
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
	})

	t.Run("must fail: without comments: true the Go file reaches the flow", func(t *testing.T) {
		a, cmd, recipe := newFreshCheckoutProject(t)
		declareGoComments(t, recipe, false)
		require.NoError(t, cmd.Flags().Set("fail-on-unknown", "true"))
		proj, err := project.Load(recipe)
		require.NoError(t, err)
		err = a.RunDefaultFlowConverge(cmd, proj, recipe, ConvergeOptions{UntilGate: true, MaxPasses: 3, noChecks: true})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse.go")
	})
}

func goFile(t *testing.T, src string) string {
	t.Helper()
	isolateCheckExecution(t)
	file := filepath.Join(t.TempDir(), "parse.go")
	require.NoError(t, os.WriteFile(file, []byte(src), 0o600))
	return file
}

func findingsOf(report check.Report, family string) []check.Diagnostic {
	var out []check.Diagnostic
	for _, d := range report.Findings {
		if d.Check == family {
			out = append(out, d)
		}
	}
	return out
}

func swapCommentProviders(t *testing.T, p comment.Provider) {
	t.Helper()
	saved := commentProviders
	commentProviders = comment.NewRegistry(p)
	t.Cleanup(func() { commentProviders = saved })
}

// mixedGroupDroppingProvider excludes any comment group that holds a directive,
// which loses the prose beside a `//go:` pragma.
type mixedGroupDroppingProvider struct{ golang.Provider }

func (p mixedGroupDroppingProvider) Locate(name string, src []byte) (*comment.File, error) {
	f, err := p.Provider.Locate(name, src)
	if err != nil || !bytes.Contains(src, []byte("//go:")) {
		return f, err
	}
	return &comment.File{Language: f.Language, Excluded: f.Excluded}, nil
}

// agreeableFormatterProvider reports that gofmt agrees with every comment.
type agreeableFormatterProvider struct{ golang.Provider }

func (agreeableFormatterProvider) Disagreements(string, []byte, *comment.File) ([]comment.Disagreement, error) {
	return nil, nil
}

// failingFormatterProvider stands for a formatter that cannot compare a file.
type failingFormatterProvider struct{ golang.Provider }

func (failingFormatterProvider) Disagreements(string, []byte, *comment.File) ([]comment.Disagreement, error) {
	return nil, errors.New("the formatter is unavailable")
}
