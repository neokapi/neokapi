package backend

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host"
)

// commentsRecipe declares a Go file for its comments and a YAML file for its
// comments alone at a point of their own. The defaults declare a directive, and
// the source profile holds comments to its limits.
const commentsRecipe = `version: v1
name: desktop-comments
defaults:
  source_language: en
  comments:
    directives: ["okapi-skip:"]
profiles:
  site:
    channels: [web]
  source:
    channels: [comments]
collections:
  - name: code
    channel: site/web
    source_only: true
    content:
      - path: "code/*.go"
        comments: true
  - name: config
    channel: site/web
    source_only: true
    content:
      - path: "config/*.yaml"
        comments:
          only: true
          channel: source/comments
`

// The site profile forbids "utilize". The source profile prohibits an unowned
// FIXME and holds comments to short sentences.
const (
	commentsSiteVoice = `id: site
name: Site
vocabulary:
  forbidden_terms:
    - term: utilize
      replacement: use
      severity: major
`
	commentsSourceVoice = `id: source
name: Source comments
constraints:
  - id: source/owned-fixme
    version: 1
    source: source-guide.md
    statement: Do not leave a FIXME without an owner.
    kind: prohibited_pattern
    regex: '\bFIXME\b'
style:
  sentence_length: short
  comments: {}
`
)

// The Go comments sit at site/web: "utilize" is a finding there and FIXME is
// not, and the directive line's doubled word is set aside. The YAML comments sit
// at source/comments: "utilize" is not a finding there, and FIXME, the doubled
// word and the 55-word sentence are.
const (
	commentsGo = `package code

// Parse helps callers utilize the input.
// okapi-skip: ParseTest#testEmpty the the reason is recorded elsewhere
func Parse() {}

// FIXME name the owner of the retry.
func Retry() {}
`
	commentsYAML = `# Callers utilize this value.
greeting: Hello
# FIXME: the the farewell is retired here.
farewell: Goodbye
# Status reads each value from the input and keeps what it finds reads each value from the input and keeps what it finds reads each value from the input and keeps what it finds reads each value from the input and keeps what it finds reads each value from the input and keeps what it.
status: Ready
`
)

func writeCommentsProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range map[string]string{
		"kapi.yaml":                        commentsRecipe,
		".kapi/profiles/site/voice.yaml":   commentsSiteVoice,
		".kapi/profiles/source/voice.yaml": commentsSourceVoice,
		"code/parse.go":                    commentsGo,
		"config/app.yaml":                  commentsYAML,
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	return root
}

// placedFinding is a comment finding as a reader locates it: the file, the
// comment's block and lines, and what fired.
type placedFinding struct {
	file, block, rule, message string
	first, last                int
}

// kapiCheckCommentFindings runs `kapi check` over the project and returns the
// findings on its comments.
func kapiCheckCommentFindings(t *testing.T, projPath string) []placedFinding {
	t.Helper()
	cmd := host.NewEnvCommand(t.Context(), "check")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.Flags().String("project", projPath, "")
	cmd.Flags().Int("max-critical", 0, "")
	cmd.Flags().Int("max-major", -1, "")
	cmd.Flags().Int("max-minor", -1, "")
	report, err := (&host.App{SourceLang: "en"}).ComputeCheck(cmd, nil)
	require.NoError(t, err)

	var out []placedFinding
	for _, d := range report.Findings {
		if d.Location.CommentSHA256 == "" {
			continue
		}
		p := placedFinding{file: filepath.Base(d.Location.File), block: d.Location.Block, rule: d.Rule, message: d.Message}
		if d.Location.Lines != nil {
			p.first, p.last = d.Location.Lines.First, d.Location.Lines.Last
		}
		out = append(out, p)
	}
	return out
}

// panelFindings returns the findings the Checks panel lists for the named
// files, read from the JSON the panel receives.
func panelFindings(t *testing.T, res *CheckRunResult, files ...string) []placedFinding {
	t.Helper()
	var out []placedFinding
	for _, file := range res.Files {
		base := filepath.Base(file.Path)
		if !slices.Contains(files, base) {
			continue
		}
		for _, f := range file.Findings {
			raw, err := json.Marshal(f)
			require.NoError(t, err)
			var wire struct {
				BlockID string `json:"block_id"`
				Rule    string `json:"rule"`
				Message string `json:"message"`
				Lines   *struct {
					First int `json:"first"`
					Last  int `json:"last"`
				} `json:"lines"`
			}
			require.NoError(t, json.Unmarshal(raw, &wire))
			p := placedFinding{file: base, block: wire.BlockID, rule: wire.Rule, message: wire.Message}
			if wire.Lines != nil {
				p.first, p.last = wire.Lines.First, wire.Lines.Last
			}
			out = append(out, p)
		}
	}
	return out
}

// The Checks panel reads a project's comments the way `kapi check` does, so the
// two report the same comment findings on the same blocks and lines: at each
// comment's own point, with the declared directives set aside, and held to the
// comment limits in force.
func TestRunChecksReportsCommentFindingsLikeKapiCheck(t *testing.T) {
	isolateCheckPlugins(t)
	t.Setenv("KAPI_NO_PROJECT", "1")
	root := writeCommentsProject(t)
	projPath := filepath.Join(root, "kapi.yaml")

	want := kapiCheckCommentFindings(t, projPath)
	require.NotEmpty(t, want, "kapi check reports findings on the fixture's comments")
	var rules []string
	for _, f := range want {
		rules = append(rules, f.rule)
	}
	assert.Subset(t, rules, []string{"voice.vocabulary", "voice.style", "hygiene.doubled-word"},
		"the fixture exercises both points and the checkset")

	app := NewApp()
	tab, err := app.OpenProject(projPath)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	res, err := app.RunChecks(tab.ID, ProjectFilter{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.NotEqual(t, "did_not_run", res.Verdict, "the comments were checked: %v", res.DidNotRun)
	assert.ElementsMatch(t, want, panelFindings(t, res, "parse.go", "app.yaml"))
}
