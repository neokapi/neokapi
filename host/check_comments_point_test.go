package host

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
)

// The fixture's two points hold content to different rules. site/web forbids
// the term "utilize" and retires ScopedName for SiteName. source/comments
// prohibits an unowned FIXME and retires LegacyName for SourceName. The YAML
// value and the comments each use those words, so a block held to the other
// point reports what that point forbids.
const (
	siteVoice = `id: site
name: Site
vocabulary:
  forbidden_terms:
    - term: utilize
      replacement: use
`
	sourceVoice = `id: source
name: Source
constraints:
  - id: source/owned-fixme
    version: 1
    source: source-guide.md
    statement: Do not leave a FIXME without an owner.
    kind: prohibited_pattern
    regex: '\bFIXME\b'
`
	pointYAML = "# Callers utilize this value, and ScopedName is fine in a comment.\ngreeting: We utilize ScopedName and LegacyName. FIXME later\n# FIXME: LegacyName is retired here.\nfarewell: Goodbye\n"
	pointGo   = "package code\n\n// Parse helps callers utilize the input.\nfunc Parse() {}\n\n// FIXME name the owner of the retry.\nfunc Retry() {}\n"
)

// Where commentPointProject places the comments.
type placement int

const (
	// atFilePoint declares the comments with `comments: true` alone, so they sit
	// at their file's point, site/web.
	atFilePoint placement = iota
	// onItem places them with `comments: {channel: source/comments}` on each item.
	onItem
	// byDefaults places them with `defaults.comments.channel`.
	byDefaults
	// onlyOnItem declares each item for its comments alone and places them with
	// `comments: {only: true, channel: source/comments}`.
	onlyOnItem
)

// commentPointProject declares a YAML file and a Go file at site/web, with
// their comments placed as p says.
func commentPointProject(t *testing.T, p placement) string {
	t.Helper()
	isolateCheckExecution(t)
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	defaults, comments := "", "true"
	switch p {
	case onlyOnItem:
		comments = "\n          only: true\n          channel: source/comments"
	case onItem:
		comments = "\n          channel: source/comments"
	case byDefaults:
		defaults = "  comments:\n    channel: source/comments\n"
	}
	// The project binds terms of its own, so the ship gate runs its terminology
	// gate; each profile binds a terms store of its own, which wins at that
	// profile's points.
	write("kapi.yaml", `version: v1
name: comment-points
defaults:
  source_language: en
`+defaults+`profiles:
  site:
    channels: [web]
    termstore: site
  source:
    channels: [comments]
    termstore: source
collections:
  - name: config
    channel: site/web
    source_only: true
    content:
      - path: "config/*.yaml"
        comments: `+comments+`
  - name: code
    channel: site/web
    source_only: true
    content:
      - path: "code/*.go"
        comments: `+comments+`
`)
	write(".kapi/profiles/site/voice.yaml", siteVoice)
	write(".kapi/profiles/source/voice.yaml", sourceVoice)
	writeTermsBundle(t, filepath.Join(root, ".kapi", "terms.json"), "project-name", "Kapi", "OldKapi")
	writeTermsStore(t, namedTermStorePath(t, "site"), "site-name", "SiteName", "ScopedName")
	writeTermsStore(t, namedTermStorePath(t, "source"), "source-name", "SourceName", "LegacyName")
	write("config/app.yaml", pointYAML)
	write("code/parse.go", pointGo)
	readProjectContext(t, root)
	return root
}

// writeTermsBundle writes a terms bundle at path holding one concept, which
// retires forbidden for preferred.
func writeTermsBundle(t *testing.T, path, id, preferred, forbidden string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	data, err := ktb.Marshal(ktb.FromConcepts([]terms.Concept{{ID: id, Terms: []terms.Term{
		{Text: preferred, Locale: model.LocaleEnglish, Status: model.TermPreferred},
		{Text: forbidden, Locale: model.LocaleEnglish, Status: model.TermForbidden},
	}}}))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

// writeTermsStore writes a standalone terms store holding one concept, for a
// profile that binds its vocabulary with `termstore:`.
func writeTermsStore(t *testing.T, path, id, preferred, forbidden string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	store, err := terms.NewSQLiteStore(path)
	require.NoError(t, err)
	defer func() { require.NoError(t, store.Close()) }()
	require.NoError(t, store.AddConcept(t.Context(), terms.Concept{ID: id, Terms: []terms.Term{
		{Text: preferred, Locale: model.LocaleEnglish, Status: model.TermPreferred},
		{Text: forbidden, Locale: model.LocaleEnglish, Status: model.TermForbidden},
	}}))
}

// governedFinding is a finding reduced to its file, its block and the word it
// caught. A YAML value's block reads "value": the greeting is the fixture's only
// value with a finding.
type governedFinding struct {
	file, block, caught string
}

func caughtWord(text string) string {
	for _, w := range []string{"utilize", "ScopedName", "FIXME", "LegacyName"} {
		if strings.Contains(text, w) {
			return w
		}
	}
	return text
}

func blockKind(block string) string {
	if strings.HasPrefix(block, "comment/") || strings.HasPrefix(block, "func/") {
		return block
	}
	return "value"
}

func governedFindings(findings []check.Diagnostic) []governedFinding {
	var out []governedFinding
	for _, d := range findings {
		if d.Check == "voice" {
			out = append(out, governedFinding{file: filepath.Base(d.Location.File), block: blockKind(d.Location.Block), caught: caughtWord(d.Message)})
		}
	}
	return out
}

// commentsApart is what a check reports when the comments sit at
// source/comments: each block reports what its own point forbids.
var commentsApart = []governedFinding{
	{"app.yaml", "value", "utilize"},
	{"app.yaml", "value", "ScopedName"},
	{"app.yaml", "comment/farewell", "FIXME"},
	{"app.yaml", "comment/farewell", "LegacyName"},
	{"parse.go", "func/Retry", "FIXME"},
}

// commentsTogether is what a check reports when the comments sit at their
// file's point, site/web.
var commentsTogether = []governedFinding{
	{"app.yaml", "value", "utilize"},
	{"app.yaml", "value", "ScopedName"},
	{"app.yaml", "comment/greeting", "utilize"},
	{"app.yaml", "comment/greeting", "ScopedName"},
	{"parse.go", "func/Parse", "utilize"},
}

func inFile(file string, in []governedFinding) []governedFinding {
	return slices.DeleteFunc(slices.Clone(in), func(f governedFinding) bool { return f.file != file })
}

func caughtOnly(in []governedFinding, words ...string) []governedFinding {
	return slices.DeleteFunc(slices.Clone(in), func(f governedFinding) bool { return !slices.Contains(words, f.caught) })
}

// contextChannels lists the channels each file's recorded contexts were
// resolved at.
func contextChannels(contexts []check.CheckContext) map[string][]string {
	out := map[string][]string{}
	for _, c := range contexts {
		file := filepath.Base(c.File)
		out[file] = append(out[file], c.Voice.Channel)
	}
	for file := range out {
		slices.Sort(out[file])
	}
	return out
}

// sitePoint and commentsPoint are the points the fixture's blocks sit at.
var (
	sitePoint     = check.Point{Profile: "site", Channel: "web"}
	commentsPoint = check.Point{Profile: "source", Channel: "comments", Comments: true}
)

// goCommentsPoint is where a Go file's comments sit when no point of their own
// is declared: at their item's point, site/web. No reader parses a Go file, so
// its own content resolves past that item to the project's default point, and
// the comments sit apart from it.
var goCommentsPoint = check.Point{Profile: "site", Channel: "web", Comments: true}

// wantPoint is the point a block of the fixture is checked at.
func wantPoint(block string, apart bool) check.Point {
	switch {
	case apart && blockKind(block) != "value":
		return commentsPoint
	case strings.HasPrefix(block, "func/"):
		return goCommentsPoint
	}
	return sitePoint
}

// assertPoints holds every voice finding to the point its block sits at.
func assertPoints(t *testing.T, findings []check.Diagnostic, apart bool) {
	t.Helper()
	checked := 0
	for _, d := range findings {
		if d.Check != "voice" {
			continue
		}
		checked++
		if assert.NotNil(t, d.Point, "%s %s", d.Location.File, d.Location.Block) {
			assert.Equal(t, wantPoint(d.Location.Block, apart), *d.Point, "%s %s", d.Location.File, d.Location.Block)
		}
	}
	assert.Positive(t, checked, "the findings carry points to hold")
}

// assertContextPoints holds each recorded context's point to the channel its
// voice was resolved at. A Go file's comments sit apart from its content
// wherever they are placed (goCommentsPoint).
func assertContextPoints(t *testing.T, contexts []check.CheckContext) {
	t.Helper()
	for _, c := range contexts {
		if assert.NotNil(t, c.Point, c.File) {
			assert.Equal(t, c.Voice.Channel, c.Point.Channel, c.File)
			assert.Equal(t, c.Voice.Channel == "comments" || filepath.Ext(c.File) == ".go", c.Point.Comments, c.File)
		}
	}
}

// ruleRuns lists the points the voice.rules analyzer ran at for each file.
func ruleRuns(analyzers []check.AnalyzerExecution) map[string][]string {
	out := map[string][]string{}
	for _, run := range analyzers {
		if run.ID != "voice.rules" {
			continue
		}
		at := "file"
		if run.Point != nil {
			at = run.Point.Profile + "/" + run.Point.Channel
		}
		file := filepath.Base(run.File)
		out[file] = append(out[file], at)
	}
	for file := range out {
		slices.Sort(out[file])
	}
	return out
}

// A content item can place its comments at a governance point apart from the
// values its reader extracts. Each block is held to the voice and terms of the
// point it sits at.
func TestCheckGovernsCommentsAtTheirOwnPoint(t *testing.T) {
	for name, p := range map[string]placement{"on the item": onItem, "by the defaults": byDefaults} {
		t.Run("comments placed "+name+" are held to their own point", func(t *testing.T) {
			report := checkProject(t, commentPointProject(t, p))
			assert.NotEqual(t, check.VerdictDidNotRun, report.Verdict, report.DidNotRun)
			assert.Equal(t, 6, report.Target.Blocks, "two YAML values, two YAML comments and two Go comments")
			assert.ElementsMatch(t, commentsApart, governedFindings(report.Findings))
			require.NotNil(t, report.Execution)
			assert.Equal(t, map[string][]string{"app.yaml": {"comments", "web"}, "parse.go": {"comments"}}, contextChannels(report.Execution.Contexts))
			assertContextPoints(t, report.Execution.Contexts)
			assertPoints(t, report.Findings, true)
			assert.Equal(t, map[string][]string{"app.yaml": {"site/web", "source/comments"}, "parse.go": {"source/comments"}}, ruleRuns(report.Execution.Analyzers),
				"the vocabulary checker runs once for each point a file's blocks sit at")
		})
	}

	t.Run("named files are held to the same points", func(t *testing.T) {
		root := commentPointProject(t, onItem)
		cmd := executionCommand(t)
		cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")
		report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, []string{filepath.Join(root, "config", "app.yaml"), filepath.Join(root, "code", "parse.go")})
		require.NoError(t, err)
		assert.Equal(t, 6, report.Target.Blocks)
		assert.ElementsMatch(t, commentsApart, governedFindings(report.Findings))
		assertPoints(t, report.Findings, true)
	})

	t.Run("must fail: comments with no point of their own sit at their file's point", func(t *testing.T) {
		report := checkProject(t, commentPointProject(t, atFilePoint))
		assert.Equal(t, 6, report.Target.Blocks)
		assert.ElementsMatch(t, commentsTogether, governedFindings(report.Findings))
		require.NotNil(t, report.Execution)
		assert.Equal(t, map[string][]string{"app.yaml": {"web"}, "parse.go": {"web"}}, contextChannels(report.Execution.Contexts))
		assertContextPoints(t, report.Execution.Contexts)
		assertPoints(t, report.Findings, false)
		assert.Equal(t, map[string][]string{"app.yaml": {"file"}, "parse.go": {"site/web"}}, ruleRuns(report.Execution.Analyzers),
			"a Go file's comments sit apart from its content, so the checker's run names their point")
	})
}

// A diff-scoped check holds each touched block to its own point.
func TestDiffCheckGovernsCommentsAtTheirOwnPoint(t *testing.T) {
	const patch = "--- a/config/app.yaml\n+++ b/config/app.yaml\n@@ -1,3 +1,3 @@\n-# Callers use this value.\n-greeting: Hello\n-# Confirm the farewell.\n" +
		"+# Callers utilize this value, and ScopedName is fine in a comment.\n+greeting: We utilize ScopedName and LegacyName. FIXME later\n+# FIXME: LegacyName is retired here.\n"

	t.Run("comments placed apart are held to their own point", func(t *testing.T) {
		report := diffCheckProject(t, commentPointProject(t, onItem), patch)
		assert.Equal(t, 3, report.Target.Blocks, "the two comments and the value the change touches")
		assert.ElementsMatch(t, inFile("app.yaml", commentsApart), governedFindings(report.Findings))
		assertPoints(t, report.Findings, true)
	})

	t.Run("must fail: comments with no point of their own sit at their file's point", func(t *testing.T) {
		report := diffCheckProject(t, commentPointProject(t, atFilePoint), patch)
		assert.Equal(t, 3, report.Target.Blocks)
		assert.ElementsMatch(t, inFile("app.yaml", commentsTogether), governedFindings(report.Findings))
		assertPoints(t, report.Findings, false)
	})
}

// The ship gates hold each block to its own point.
func TestShipGatesGovernCommentsAtTheirOwnPoint(t *testing.T) {
	gateFindings := func(t *testing.T, out verifyOutput, name string, apart bool) []governedFinding {
		t.Helper()
		gate, ok := gateByName(out, name)
		require.True(t, ok, "the %s gate ran", name)
		require.NotNil(t, gate.Coverage, name)
		assert.Positive(t, gate.Coverage.Blocks, name)
		var got []governedFinding
		for _, f := range gate.Findings {
			if f.Block != "" {
				got = append(got, governedFinding{file: filepath.Base(f.File), block: blockKind(f.Block), caught: caughtWord(f.Message)})
				if assert.NotNil(t, f.Point, "%s %s %s", name, f.File, f.Block) {
					assert.Equal(t, wantPoint(f.Block, apart), *f.Point, "%s %s %s", name, f.File, f.Block)
				}
			}
		}
		return got
	}
	ship := func(t *testing.T, p placement) verifyOutput {
		t.Helper()
		out, err := (&App{}).computeVerify(sourceShipCommand(t, commentPointProject(t, p)), nil)
		require.NoError(t, err)
		return out
	}

	t.Run("comments placed apart are held to their own point", func(t *testing.T) {
		out := ship(t, onItem)
		assert.ElementsMatch(t, caughtOnly(commentsApart, "ScopedName", "LegacyName"), gateFindings(t, out, gateTerms, true))
		assert.ElementsMatch(t, commentsApart, gateFindings(t, out, gateVoice, true))
		assert.ElementsMatch(t, commentsApart, gateFindings(t, out, gateChecks, true))
	})

	t.Run("must fail: comments with no point of their own sit at their file's point", func(t *testing.T) {
		out := ship(t, atFilePoint)
		assert.ElementsMatch(t, caughtOnly(commentsTogether, "ScopedName", "LegacyName"), gateFindings(t, out, gateTerms, false))
		assert.ElementsMatch(t, commentsTogether, gateFindings(t, out, gateVoice, false))
		assert.ElementsMatch(t, commentsTogether, gateFindings(t, out, gateChecks, false))
	})
}

// MCP check_file holds each block of the file it names to its own point.
func TestMCPCheckFileGovernsCommentsAtTheirOwnPoint(t *testing.T) {
	t.Run("comments placed apart are held to their own point", func(t *testing.T) {
		report := checkFileOverMCP(t, commentPointProject(t, onItem), "config/app.yaml")
		assert.Equal(t, 4, report.Target.Blocks)
		assert.ElementsMatch(t, inFile("app.yaml", commentsApart), governedFindings(report.Findings))
		assertPoints(t, report.Findings, true)

		report = checkFileOverMCP(t, commentPointProject(t, onItem), "code/parse.go")
		assert.Equal(t, 2, report.Target.Blocks)
		assert.ElementsMatch(t, inFile("parse.go", commentsApart), governedFindings(report.Findings))
		assertPoints(t, report.Findings, true)
	})

	t.Run("must fail: comments with no point of their own sit at their file's point", func(t *testing.T) {
		report := checkFileOverMCP(t, commentPointProject(t, atFilePoint), "config/app.yaml")
		assert.Equal(t, 4, report.Target.Blocks)
		assert.ElementsMatch(t, inFile("app.yaml", commentsTogether), governedFindings(report.Findings))
		assertPoints(t, report.Findings, false)
	})
}
