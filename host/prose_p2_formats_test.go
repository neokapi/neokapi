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
	"github.com/neokapi/neokapi/core/tool"
)

// proseFormat is a format that supplies its files' comments: a file of the
// format holding one comment, the block that comment becomes, and the lines it
// spans.
type proseFormat struct {
	name  string
	file  string
	body  func(comment string) string
	block string
	lines format.LineRange
}

var proseFormats = map[string]proseFormat{
	"yaml": {
		name: "yaml", file: "app.yaml",
		body:  func(c string) string { return "# " + c + "\ngreeting: Hello world\n" },
		block: "comment/greeting", lines: format.LineRange{First: 1, Last: 1},
	},
	"xml": {
		name: "xml", file: "app.xml",
		body: func(c string) string {
			return "<root>\n  <!-- " + c + " -->\n  <greeting>Hello world</greeting>\n</root>\n"
		},
		block: "comment/root/greeting", lines: format.LineRange{First: 2, Last: 2},
	},
	"html": {
		name: "html", file: "index.html",
		body: func(c string) string {
			return "<!DOCTYPE html>\n<html>\n<body>\n  <!-- " + c + " -->\n  <p id=\"greeting\">Hello world</p>\n</body>\n</html>\n"
		},
		block: "comment/p[greeting]", lines: format.LineRange{First: 4, Last: 4},
	},
	"markdown": {
		name: "markdown", file: "guide.md",
		body:  func(c string) string { return "# Guide\n\n<!-- " + c + " -->\n\nHello world.\n" },
		block: "comment/guide", lines: format.LineRange{First: 3, Last: 3},
	},
	"mdx": {
		name: "mdx", file: "guide.mdx",
		body:  func(c string) string { return "# Guide\n\n{/* " + c + " */}\n\nHello world.\n" },
		block: "comment/guide", lines: format.LineRange{First: 3, Last: 3},
	},
	"androidxml": {
		name: "androidxml", file: "strings.xml",
		body: func(c string) string {
			return "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<resources>\n  <!-- " + c + " -->\n  <string name=\"greeting\">Hello world</string>\n</resources>\n"
		},
		block: "comment/resources/string[greeting]", lines: format.LineRange{First: 3, Last: 3},
	},
	"resx": {
		name: "resx", file: "Resources.resx",
		body: func(c string) string {
			return "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<root>\n  <!-- " + c + " -->\n  <data name=\"Greeting\" xml:space=\"preserve\">\n    <value>Hello world</value>\n  </data>\n</root>\n"
		},
		block: "comment/root/data[Greeting]", lines: format.LineRange{First: 3, Last: 3},
	},
	"tmx": {
		name: "tmx", file: "memory.tmx",
		body: func(c string) string {
			return "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<tmx version=\"1.4\">\n  <header srclang=\"en\" datatype=\"plaintext\" segtype=\"sentence\" adminlang=\"en\" o-tmf=\"none\" creationtool=\"kapi\" creationtoolversion=\"1\"/>\n  <body>\n    <!-- " + c + " -->\n    <tu tuid=\"greeting\">\n      <tuv xml:lang=\"en\"><seg>Hello world</seg></tuv>\n    </tu>\n  </body>\n</tmx>\n"
		},
		block: "comment/tmx/body/tu", lines: format.LineRange{First: 5, Last: 5},
	},
	"doclang": {
		name: "doclang", file: "report.dclg.xml",
		body: func(c string) string {
			return "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<doclang version=\"0.6\">\n  <!-- " + c + " -->\n  <text>Hello world</text>\n</doclang>\n"
		},
		block: "comment/doclang/text", lines: format.LineRange{First: 3, Last: 3},
	},
}

// serviceVoiceProfile prohibits "utilize" everywhere except the docs channel.
const serviceVoiceProfile = `name: Service
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
`

// proseFormatProject declares the same file of a format at two points of one
// voice profile, the code channel and the docs channel, with its comment
// holding text. comments sets `comments: true` on both items.
func proseFormatProject(t *testing.T, pf proseFormat, comments bool, text string) string {
	t.Helper()
	isolateCheckExecution(t)
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	write("kapi.yaml", fmt.Sprintf(`version: v1
name: prose-%[1]s
defaults:
  source_language: en
  voice:
    profile_file: .kapi/voice.yaml
profiles:
  service:
    channels: [code, docs]
    voice: .kapi/voice.yaml
collections:
  - name: code
    channel: service/code
    source_only: true
    content:
      - path: "src/%[2]s"
        format: %[1]s
        comments: %[3]t
  - name: guide
    channel: service/docs
    source_only: true
    content:
      - path: "guide/%[2]s"
        format: %[1]s
        comments: %[3]t
`, pf.name, pf.file, comments))
	write(".kapi/voice.yaml", serviceVoiceProfile)
	write(filepath.Join("src", pf.file), pf.body(text))
	write(filepath.Join("guide", pf.file), pf.body(text))
	return root
}

func TestProseP2_yaml(t *testing.T)       { proseP2(t, proseFormats["yaml"]) }
func TestProseP2_xml(t *testing.T)        { proseP2(t, proseFormats["xml"]) }
func TestProseP2_html(t *testing.T)       { proseP2(t, proseFormats["html"]) }
func TestProseP2_markdown(t *testing.T)   { proseP2(t, proseFormats["markdown"]) }
func TestProseP2_mdx(t *testing.T)        { proseP2(t, proseFormats["mdx"]) }
func TestProseP2_androidxml(t *testing.T) { proseP2(t, proseFormats["androidxml"]) }
func TestProseP2_resx(t *testing.T)       { proseP2(t, proseFormats["resx"]) }
func TestProseP2_tmx(t *testing.T)        { proseP2(t, proseFormats["tmx"]) }
func TestProseP2_doclang(t *testing.T)    { proseP2(t, proseFormats["doclang"]) }

// proseP2 is the P2 rung for a format that supplies its files' comments: a
// declared comment takes part in `kapi check` at its file's point, a finding
// carries the comment's lines, the comment layer catches its canary on every
// run, and a format with no comment formatter reports that analyzer as
// unsupported. The subtests named "must fail" break one of those on purpose.
func proseP2(t *testing.T, pf proseFormat) {
	t.Helper()
	const governed = "Helps you utilize the greeting."

	t.Run("a comment is held to the governance of its file's point", func(t *testing.T) {
		report := checkProject(t, proseFormatProject(t, pf, true, governed))
		assert.NotEqual(t, check.VerdictDidNotRun, report.Verdict, report.DidNotRun)
		voice := findingsOf(report, "voice")
		require.Len(t, voice, 1, "the code comment is held to the rule and the docs comment is excepted: %+v", report.Findings)
		assert.Equal(t, "src", filepath.Base(filepath.Dir(voice[0].Location.File)))
		assert.Equal(t, pf.block, voice[0].Location.Block)
		require.NotNil(t, voice[0].Location.Lines)
		assert.Equal(t, pf.lines, *voice[0].Location.Lines)
	})

	t.Run("the comment layer catches its canary and claims no formatter", func(t *testing.T) {
		report := checkProject(t, proseFormatProject(t, pf, true, "Greets the reader."))
		run := analyzerRun(t, report, "comments."+pf.name)
		assert.Equal(t, check.AnalyzerPassed, run.Status)
		require.NotNil(t, run.Canary)
		assert.Equal(t, check.CanaryCaught, run.Canary.Status)
		formatter := analyzerRun(t, report, formatterCheck)
		assert.Equal(t, check.AnalyzerUnsupported, formatter.Status, "a check with no formatter to run is never a pass")
		assert.Contains(t, formatter.Reason, pf.name)
	})

	t.Run("must fail: without comments: true the comment is not checked", func(t *testing.T) {
		report := checkProject(t, proseFormatProject(t, pf, false, governed))
		assert.Empty(t, findingsOf(report, "voice"))
		assert.Positive(t, report.Target.Blocks, "the reader's blocks were checked")
	})

	t.Run("must fail: an inert hygiene checker invalidates the comment run", func(t *testing.T) {
		saved := hygieneTool
		hygieneTool = func() BlockProcessor {
			return &tool.BaseTool{ToolName: "inert", Annotate: func(tool.BlockView) error { return nil }}
		}
		t.Cleanup(func() { hygieneTool = saved })
		report := checkProject(t, proseFormatProject(t, pf, true, "Greets the reader."))
		assert.Equal(t, check.AnalyzerInvalid, analyzerRun(t, report, "comments."+pf.name).Status)
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
	})

	t.Run("must fail: comments the provider cannot place leave the check not run", func(t *testing.T) {
		real, ok := commentProviders.ForFormat(pf.name)
		require.True(t, ok)
		saved := commentProviders
		r := comment.NewRegistry(golang.Provider{})
		r.RegisterFormat(pf.name, unlocatedProvider{real})
		commentProviders = r
		t.Cleanup(func() { commentProviders = saved })

		report := checkProject(t, proseFormatProject(t, pf, true, "Greets the reader."))
		assert.Equal(t, check.AnalyzerDidNotRun, analyzerRun(t, report, "comments."+pf.name).Status)
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
	})
}

// unlocatedProvider stands for a provider that reads a file and cannot place
// its comments exactly.
type unlocatedProvider struct{ comment.Provider }

func (p unlocatedProvider) Locate(string, []byte) (*comment.File, error) {
	return nil, fmt.Errorf("%s: %w", p.Language(), comment.ErrUnlocated)
}

// A diff-scoped check records the same analyzers for a file whose format
// supplies its comments as a whole-file check of the project, in the same order
// and with the same outcomes, and scopes a change to a comment line to that
// comment. Reader validation is left out: a diff-scoped check refuses
// --validate.
//
// A reader that cannot place its own blocks leaves every change to its file
// unscoped, whatever the comments' spans, so the file did not run.
func TestDiffCheckFormatCommentAnalyzersMatchAWholeFileCheck(t *testing.T) {
	unplaced := map[string]string{
		"tmx":     "has no position in the file",
		"doclang": "keeps no record of where its content sits",
	}
	type analyzer struct {
		ID       string
		Status   check.AnalyzerStatus
		Required bool
		Canary   check.CanaryStatus
	}
	for _, name := range []string{"yaml", "xml", "html", "markdown", "mdx", "androidxml", "resx", "tmx", "doclang"} {
		pf := proseFormats[name]
		t.Run(name, func(t *testing.T) {
			src := "src/" + pf.file
			analyzersOf := func(report check.Report) []analyzer {
				require.NotNil(t, report.Execution)
				var out []analyzer
				for _, run := range report.Execution.Analyzers {
					if !strings.HasSuffix(filepath.ToSlash(run.File), src) || run.ID == "reader.validation" {
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

			root := proseFormatProject(t, pf, true, "Greets the the reader.")
			line := strings.Split(pf.body("Greets the the reader."), "\n")[pf.lines.First-1]
			patch := fmt.Sprintf("--- a/%[1]s\n+++ b/%[1]s\n@@ -%[2]d +%[2]d @@\n-%[3]s\n+%[4]s\n",
				src, pf.lines.First, strings.Replace(line, "the the", "the", 1), line)
			diff := diffCheckProject(t, root, patch)
			whole := checkProject(t, root)

			entry := scopeEntry(t, diff, filepath.FromSlash(src))
			if reason, ok := unplaced[name]; ok {
				assert.Equal(t, check.ScopeDidNotRun, entry.Status)
				assert.Contains(t, entry.Reason, reason)
				assert.Equal(t, check.VerdictDidNotRun, diff.Verdict, "a changed file whose blocks cannot be placed is never a pass")
				assert.Equal(t, check.AnalyzerPassed, analyzerRun(t, whole, "comments."+name).Status, "the whole-file check reads the comments")
				return
			}
			assert.Equal(t, check.ScopeChecked, entry.Status, entry.Reason)
			var blocks []string
			for _, b := range entry.Blocks {
				blocks = append(blocks, b.Block)
			}
			assert.Equal(t, []string{pf.block}, blocks, "the change to the comment line checks that comment alone")

			scoped := analyzersOf(diff)
			ids := make([]string, len(scoped))
			for i, a := range scoped {
				ids[i] = a.ID
			}
			assert.Subset(t, ids, []string{"comments." + name, "hygiene"})
			assert.Equal(t, analyzersOf(whole), scoped)
		})
	}
}
