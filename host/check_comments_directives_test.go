package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/format"
)

// directiveGo holds prose split by two declared markers, a comment that is only
// a declared marker, a marker the recipe does not declare, and two lines of
// prose that mention a declared marker without opening with it. Every comment
// line carries a doubled word, so a line read as prose is a finding.
const directiveGo = `package code

// Parse reads the input.
// okapi-skip: ParseTest#testEmpty the the reason is recorded elsewhere
// okapi-unmapped: ParseTest#testLong the the harness covers it
// It returns an an error for empty input.
func Parse() {}

// okapi-skip: FormatTest#testAll no no port exists
func Format() {}

// okapi-deferred: RenderTest#testAll the the port is planned
func Render() {}

// Sort orders the the rows; see the okapi-skip: markers on skipped tests.
func Sort() {}

// OKAPI-SKIP: in capitals is prose, so so this line is checked.
func Merge() {}
`

// directiveYAML holds a marker only the YAML item declares, between two prose
// lines, and a marker the project defaults declare.
const directiveYAML = `# Greets the reader.
# deploy-lock: the the release train holds this key
# Shown on on the landing page.
greeting: Hello world
# okapi-skip: the the defaults marker applies here too
farewell: Goodbye
`

// What directiveProject declares.
type declares int

const (
	// declaresNothing declares both files with `comments: true` alone.
	declaresNothing declares = iota
	// declaresDefaults adds `okapi-skip:` and `okapi-unmapped:` under
	// defaults.comments.
	declaresDefaults
	// declaresAll adds `deploy-lock:` on the YAML item as well.
	declaresAll
)

// directiveProject declares the comments of a Go file and a YAML file.
func directiveProject(t *testing.T, d declares) string {
	t.Helper()
	isolateCheckExecution(t)
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	defaults, yamlComments := "", "true"
	if d >= declaresDefaults {
		defaults = "  comments:\n    directives: [\"okapi-skip:\", \"okapi-unmapped:\"]\n"
	}
	if d == declaresAll {
		yamlComments = "\n          directives: [\"deploy-lock:\"]"
	}
	write("kapi.yaml", `version: v1
name: directives
defaults:
  source_language: en
`+defaults+`collections:
  - name: code
    source_only: true
    content:
      - path: "code/*.go"
        comments: true
  - name: config
    source_only: true
    content:
      - path: "config/*.yaml"
        comments: `+yamlComments+`
`)
	write("code/parse.go", directiveGo)
	write("config/app.yaml", directiveYAML)
	return root
}

// placed is where a finding sits: the file's base name, the block and its lines.
type placed struct {
	file  string
	block string
	lines format.LineRange
}

func doubledWords(findings []check.Diagnostic) []placed {
	var out []placed
	for _, d := range findings {
		if d.Rule != "hygiene.doubled-word" {
			continue
		}
		p := placed{file: filepath.Base(d.Location.File), block: d.Location.Block}
		if d.Location.Lines != nil {
			p.lines = *d.Location.Lines
		}
		out = append(out, p)
	}
	return out
}

func lineRange(first, last int) format.LineRange { return format.LineRange{First: first, Last: last} }

// goProseFindings are the findings on directiveGo's comments that are prose
// whatever the recipe declares: the undeclared marker, and the two lines that
// mention a declared marker without opening with it.
var goProseFindings = []placed{
	{file: "parse.go", block: "func/Render", lines: lineRange(12, 12)},
	{file: "parse.go", block: "func/Sort", lines: lineRange(15, 15)},
	{file: "parse.go", block: "func/Merge", lines: lineRange(18, 18)},
}

// with returns goProseFindings followed by more.
func with(more ...placed) []placed { return append(slices.Clone(goProseFindings), more...) }

// withoutLines drops the lines from each finding, for a surface that reports
// none.
func withoutLines(in []placed) []placed {
	out := make([]placed, len(in))
	for i, p := range in {
		out[i] = placed{file: p.file, block: p.block}
	}
	return out
}

// A comment line that opens with a marker the recipe declares is read by the
// project's own tools. A check sets it aside, so it is never a sentence, and the
// prose on each side of it is a comment of its own.
func TestCheckSetsAsideDeclaredDirectives(t *testing.T) {
	t.Run("declared markers are never prose, in Go or YAML", func(t *testing.T) {
		report := checkProject(t, directiveProject(t, declaresAll))
		assert.NotEqual(t, check.VerdictDidNotRun, report.Verdict, report.DidNotRun)
		assert.Equal(t, 9, report.Target.Blocks, "five Go comments, two YAML comments and two YAML values")
		assert.ElementsMatch(t, with(
			placed{file: "parse.go", block: "func/Parse#2", lines: lineRange(6, 6)},
			placed{file: "app.yaml", block: "comment/greeting#2", lines: lineRange(3, 3)},
		), doubledWords(report.Findings))
		for _, id := range []string{"comments.go", "comments.yaml"} {
			run := analyzerRun(t, report, id)
			require.NotNil(t, run.Canary, id)
			assert.Equal(t, check.CanaryCaught, run.Canary.Status, id)
		}
	})

	t.Run("the defaults govern every item that declares comments", func(t *testing.T) {
		report := checkProject(t, directiveProject(t, declaresDefaults))
		assert.Equal(t, 8, report.Target.Blocks, "five Go comments, one YAML comment and two YAML values")
		assert.ElementsMatch(t, with(
			placed{file: "parse.go", block: "func/Parse#2", lines: lineRange(6, 6)},
			placed{file: "app.yaml", block: "comment/greeting", lines: lineRange(1, 3)},
		), doubledWords(report.Findings))
	})

	t.Run("must fail: without the declarations every marker line is prose", func(t *testing.T) {
		report := checkProject(t, directiveProject(t, declaresNothing))
		assert.Equal(t, 9, report.Target.Blocks)
		assert.ElementsMatch(t, with(
			placed{file: "parse.go", block: "func/Parse", lines: lineRange(3, 6)},
			placed{file: "parse.go", block: "func/Format", lines: lineRange(9, 9)},
			placed{file: "app.yaml", block: "comment/greeting", lines: lineRange(1, 3)},
			placed{file: "app.yaml", block: "comment/farewell", lines: lineRange(5, 5)},
		), doubledWords(report.Findings))
	})
}

// A diff-scoped check reads the same layer: a change to a marker line touches no
// comment, and a change beside it touches only the comment it sits in.
func TestDiffCheckSetsAsideDeclaredDirectives(t *testing.T) {
	const (
		goMarker   = "--- a/code/parse.go\n+++ b/code/parse.go\n@@ -4 +4 @@\n-// okapi-skip: ParseTest#testEmpty the reason is recorded elsewhere\n+// okapi-skip: ParseTest#testEmpty the the reason is recorded elsewhere\n"
		goProse    = "--- a/code/parse.go\n+++ b/code/parse.go\n@@ -6 +6 @@\n-// It returns an error for empty input.\n+// It returns an an error for empty input.\n"
		yamlMarker = "--- a/config/app.yaml\n+++ b/config/app.yaml\n@@ -2 +2 @@\n-# deploy-lock: the release train holds this key\n+# deploy-lock: the the release train holds this key\n"
	)
	goFile, yamlFile := filepath.FromSlash("code/parse.go"), filepath.FromSlash("config/app.yaml")

	t.Run("a change to a declared marker line touches no block", func(t *testing.T) {
		for file, patch := range map[string]string{goFile: goMarker, yamlFile: yamlMarker} {
			report := diffCheckProject(t, directiveProject(t, declaresAll), patch)
			assert.Equal(t, check.ScopeUntouched, scopeEntry(t, report, file).Status, file)
			assert.Empty(t, report.Findings, file)
		}
	})

	t.Run("a change beside a marker checks only the comment it sits in", func(t *testing.T) {
		report := diffCheckProject(t, directiveProject(t, declaresAll), goProse)
		entry := scopeEntry(t, report, goFile)
		assert.Equal(t, check.ScopeChecked, entry.Status)
		assert.Equal(t, []check.ScopeBlock{{Block: "func/Parse#2", Lines: lineRange(6, 6)}}, entry.Blocks)
		assert.Equal(t, 1, report.Target.Blocks)
		assert.Equal(t, []placed{{file: "parse.go", block: "func/Parse#2", lines: lineRange(6, 6)}}, doubledWords(report.Findings))
	})

	t.Run("must fail: without the declarations a change to a marker line checks its comment", func(t *testing.T) {
		report := diffCheckProject(t, directiveProject(t, declaresNothing), goMarker)
		entry := scopeEntry(t, report, goFile)
		assert.Equal(t, check.ScopeChecked, entry.Status)
		assert.Equal(t, []check.ScopeBlock{{Block: "func/Parse", Lines: lineRange(3, 6)}}, entry.Blocks)
		assert.NotEmpty(t, doubledWords(report.Findings))
	})
}

// checkFileOverMCP calls the MCP check_file tool on one file of the project at
// root and returns its report.
func checkFileOverMCP(t *testing.T, root, file string) check.Report {
	t.Helper()
	app := &App{SourceLang: "en", mcpRecipePath: filepath.Join(root, "kapi.yaml")}
	server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: "test"}, nil)
	registerCheckMCPTools(server, app)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "directives-test", Version: "test"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "check_file", Arguments: map[string]any{"file": filepath.Join(root, file)}})
	require.NoError(t, err)
	body, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	require.False(t, result.IsError, "%s", body)
	var report check.Report
	require.NoError(t, json.Unmarshal(body, &report))
	return report
}

// MCP check_file reads a named file through the same layer, under the
// directives of the project it sits in.
func TestMCPCheckFileSetsAsideDeclaredDirectives(t *testing.T) {
	t.Run("declared markers are never prose", func(t *testing.T) {
		report := checkFileOverMCP(t, directiveProject(t, declaresAll), "code/parse.go")
		assert.Equal(t, 5, report.Target.Blocks)
		assert.ElementsMatch(t, with(placed{file: "parse.go", block: "func/Parse#2", lines: lineRange(6, 6)}), doubledWords(report.Findings))

		report = checkFileOverMCP(t, directiveProject(t, declaresAll), "config/app.yaml")
		assert.Equal(t, 4, report.Target.Blocks, "two comments and two values")
		assert.Equal(t, []placed{{file: "app.yaml", block: "comment/greeting#2", lines: lineRange(3, 3)}}, doubledWords(report.Findings))
	})

	t.Run("must fail: without the declarations the marker lines are prose", func(t *testing.T) {
		report := checkFileOverMCP(t, directiveProject(t, declaresNothing), "code/parse.go")
		assert.Equal(t, 5, report.Target.Blocks)
		assert.ElementsMatch(t, with(
			placed{file: "parse.go", block: "func/Parse", lines: lineRange(3, 6)},
			placed{file: "parse.go", block: "func/Format", lines: lineRange(9, 9)},
		), doubledWords(report.Findings))
	})
}

// The ship gate reads the same layer as a bare check.
func TestShipCheckSetsAsideDeclaredDirectives(t *testing.T) {
	blocksWithFindings := func(t *testing.T, d declares) []placed {
		t.Helper()
		out, err := (&App{}).computeVerify(sourceShipCommand(t, directiveProject(t, d)), nil)
		require.NoError(t, err)
		qa, ok := gateByName(out, gateChecks)
		require.True(t, ok)
		require.NotNil(t, qa.Coverage)
		assert.Positive(t, qa.Coverage.Blocks)
		seen := map[placed]bool{}
		var blocks []placed
		for _, f := range qa.Findings {
			p := placed{file: filepath.Base(f.File), block: f.Block}
			if f.Block != "" && !seen[p] {
				seen[p] = true
				blocks = append(blocks, p)
			}
		}
		return blocks
	}

	t.Run("declared markers are never prose", func(t *testing.T) {
		assert.ElementsMatch(t, withoutLines(with(
			placed{file: "parse.go", block: "func/Parse#2"},
			placed{file: "app.yaml", block: "comment/greeting#2"},
		)), blocksWithFindings(t, declaresAll))
	})

	t.Run("must fail: without the declarations the marker lines are prose", func(t *testing.T) {
		assert.ElementsMatch(t, withoutLines(with(
			placed{file: "parse.go", block: "func/Parse"},
			placed{file: "parse.go", block: "func/Format"},
			placed{file: "app.yaml", block: "comment/greeting"},
			placed{file: "app.yaml", block: "comment/farewell"},
		)), blocksWithFindings(t, declaresNothing))
	})
}
