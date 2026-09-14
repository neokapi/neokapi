package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
)

// markupPointProject declares one markup file at site/web, read by format, with
// its comments at source/comments when apart is set and at the file's own point
// otherwise. The voices and terms are commentPointProject's.
func markupPointProject(t *testing.T, format, name, body string, apart bool) string {
	t.Helper()
	isolateCheckExecution(t)
	root := t.TempDir()
	write := func(file, content string) {
		t.Helper()
		path := filepath.Join(root, file)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	comments := "true"
	if apart {
		comments = "{channel: source/comments}"
	}
	write("kapi.yaml", `version: v1
name: markup-points
defaults:
  source_language: en
  terms_source: .kapi/terms.json
profiles:
  site:
    channels: [web]
  source:
    channels: [comments]
collections:
  - name: pages
    channel: site/web
    source_only: true
    content:
      - path: "`+filepath.ToSlash(filepath.Join(filepath.Dir(name), "*"+filepath.Ext(name)))+`"
        format: `+format+`
        comments: `+comments+`
`)
	write(".kapi/profiles/site/voice.yaml", siteVoice)
	write(".kapi/profiles/source/voice.yaml", sourceVoice)
	writeTermsBundle(t, filepath.Join(root, ".kapi", "terms.json"), "project-name", "Kapi", "OldKapi")
	writeTermsBundle(t, filepath.Join(root, ".kapi", "profiles", "site", "terms.json"), "site-name", "SiteName", "ScopedName")
	writeTermsBundle(t, filepath.Join(root, ".kapi", "profiles", "source", "terms.json"), "source-name", "SourceName", "LegacyName")
	write(name, body)
	return root
}

// markupFinding is a voice finding reduced to whether its block is a comment
// and the word it caught.
type markupFinding struct {
	comment bool
	caught  string
}

func markupFindings(t *testing.T, findings []check.Diagnostic, apart bool) []markupFinding {
	t.Helper()
	var out []markupFinding
	for _, d := range findings {
		if d.Check != "voice" {
			continue
		}
		isComment := strings.HasPrefix(d.Location.Block, "comment/")
		out = append(out, markupFinding{comment: isComment, caught: caughtWord(d.Message)})
		want := sitePoint
		if apart && isComment {
			want = commentsPoint
		}
		if assert.NotNil(t, d.Point, d.Location.Block) {
			assert.Equal(t, want, *d.Point, d.Location.Block)
		}
	}
	return out
}

// The comment providers for XML-based and HTML files place their comments at
// the point a content item gives them, and the values their readers extract
// stay at the item's.
func TestCheckGovernsMarkupCommentsAtTheirOwnPoint(t *testing.T) {
	for _, tc := range []struct {
		format, name, body string
	}{
		{
			format: "html",
			name:   "site/page.html",
			body:   "<!-- Callers utilize this page, and ScopedName is fine in a comment. -->\n<p>We utilize ScopedName and LegacyName. FIXME later</p>\n<!-- FIXME: LegacyName is retired here. -->\n<p>Goodbye</p>\n",
		},
		{
			format: "androidxml",
			name:   "res/values/strings.xml",
			body:   "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<resources>\n    <!-- Callers utilize this string, and ScopedName is fine in a comment. -->\n    <string name=\"greeting\">We utilize ScopedName and LegacyName. FIXME later</string>\n    <!-- FIXME: LegacyName is retired here. -->\n    <string name=\"farewell\">Goodbye</string>\n</resources>\n",
		},
	} {
		t.Run(tc.format+": comments placed apart are held to their own point", func(t *testing.T) {
			report := checkProject(t, markupPointProject(t, tc.format, tc.name, tc.body, true))
			assert.NotEqual(t, check.VerdictDidNotRun, report.Verdict, report.DidNotRun)
			assert.Positive(t, report.Target.Blocks)
			assert.ElementsMatch(t, []markupFinding{
				{comment: false, caught: "utilize"},
				{comment: false, caught: "ScopedName"},
				{comment: true, caught: "FIXME"},
				{comment: true, caught: "LegacyName"},
			}, markupFindings(t, report.Findings, true))
		})

		t.Run(tc.format+": must fail: comments with no point of their own sit at the file's point", func(t *testing.T) {
			report := checkProject(t, markupPointProject(t, tc.format, tc.name, tc.body, false))
			assert.Positive(t, report.Target.Blocks)
			assert.ElementsMatch(t, []markupFinding{
				{comment: false, caught: "utilize"},
				{comment: false, caught: "ScopedName"},
				{comment: true, caught: "utilize"},
				{comment: true, caught: "ScopedName"},
			}, markupFindings(t, report.Findings, false))
		})
	}
}
