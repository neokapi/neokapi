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

// claimOrderProject declares config/*.yaml for its values at site/web, and the
// YAML files under config/ for their comments alone at source/comments. The
// comments-only collection is listed first when commentsFirst is set.
func claimOrderProject(t *testing.T, commentsFirst bool) string {
	t.Helper()
	isolateCheckExecution(t)
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	const values = `  - name: config
    channel: site/web
    source_only: true
    content:
      - path: "config/*.yaml"
`
	const comments = `  - name: source-comments
    channel: source/comments
    source_only: true
    content:
      - path: "config/**/*.yaml"
        comments:
          only: true
`
	collections := values + comments
	if commentsFirst {
		collections = comments + values
	}
	write("kapi.yaml", `version: v1
name: claim-order
defaults:
  source_language: en
profiles:
  site:
    channels: [web]
    termstore: site
  source:
    channels: [comments]
    termstore: source
collections:
`+collections)
	write(".kapi/profiles/site/voice.yaml", siteVoice)
	write(".kapi/profiles/source/voice.yaml", sourceVoice)
	writeTermsBundle(t, filepath.Join(root, ".kapi", "terms.json"), "project-name", "Kapi", "OldKapi")
	writeTermsStore(t, namedTermStorePath(t, "site"), "site-name", "SiteName", "ScopedName")
	writeTermsStore(t, namedTermStorePath(t, "source"), "source-name", "SourceName", "LegacyName")
	write("config/app.yaml", pointYAML)
	readProjectContext(t, root)
	return root
}

// The directives in force in a file's comments are the ones of the item that
// claims the comments, so a comments-only item's directive sets a line aside in
// a file that an earlier item claims for its values.
func TestCommentDirectivesComeFromTheItemThatClaimsTheComments(t *testing.T) {
	directed := func(t *testing.T, directive string) []governedFinding {
		t.Helper()
		root := claimOrderProject(t, false)
		recipe := filepath.Join(root, "kapi.yaml")
		data, err := os.ReadFile(recipe)
		require.NoError(t, err)
		const anchor = "        comments:\n          only: true\n"
		require.Contains(t, string(data), anchor)
		data = []byte(strings.Replace(string(data), anchor, anchor+"          directives: [\""+directive+"\"]\n", 1))
		require.NoError(t, os.WriteFile(recipe, data, 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(root, "config", "app.yaml"), []byte("# okapi-skip: FIXME is set aside.\ngreeting: Hello\n"), 0o600))
		report := checkProject(t, root)
		assert.NotEqual(t, check.VerdictDidNotRun, report.Verdict, report.DidNotRun)
		return governedFindings(report.Findings)
	}

	assert.Empty(t, directed(t, "okapi-skip:"), "the comments-only item's directive sets the line aside")
	assert.Equal(t, []governedFinding{{"app.yaml", "comment/greeting", "FIXME"}}, directed(t, "okapi:"),
		"must fail: under another directive the line is a comment the source voice holds to")
}
