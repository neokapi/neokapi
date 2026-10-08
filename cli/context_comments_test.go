package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeCommentVoiceProject declares a Go file and a YAML file at site/web, with
// the comments of both placed at source/comments when apart is set. The project
// binds its own voice at the default point, the source voice sets comment
// limits, and the site voice sets none.
func writeCommentVoiceProject(t *testing.T, apart bool) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	defaults := ""
	if apart {
		defaults = "  comments:\n    channel: source/comments\n"
	}
	write("kapi.yaml", `version: v1
name: comment-voice
defaults:
  source_language: en
`+defaults+`profiles:
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
        comments: true
`)
	write(".kapi/voice.yaml", "name: project\n")
	write(".kapi/profiles/site/voice.yaml", "name: site\n")
	write(".kapi/profiles/source/voice.yaml", "name: source comments\nstyle:\n  comments:\n    doc_words: 120\n")
	write("code/parse.go", "package code\n\n// Parse reads the input.\nfunc Parse() {}\n")
	write("config/app.yaml", "# The greeting.\ngreeting: Hello\n")
	readProjectContext(t, root)
	return root
}

// contextAt runs `kapi context` with args in the working directory.
func contextAt(t *testing.T, args ...string) string {
	t.Helper()
	cmd := NewContextCmd(&App{})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute())
	return out.String()
}

// The answer for a file is for the file's own content, and --comments for its
// comments. A Go file's content is its comments alone, so no item governs
// anything else in it: the answer gives the project's voice, and --comments
// the limits a comment there is held to.
func TestContextAnswersForAFilesContentAndForItsComments(t *testing.T) {
	t.Chdir(writeCommentVoiceProject(t, true))

	goGuide := contextAt(t, filepath.Join("code", "parse.go"))
	assert.Contains(t, goGuide, "Voice: project.", "no item governs a Go file's content")
	assert.NotContains(t, goGuide, "Code comments")

	goComments := contextAt(t, "--comments", filepath.Join("code", "parse.go"))
	assert.Contains(t, goComments, "Voice: source comments.")
	assert.Contains(t, goComments, "Code comments:")
	assert.Contains(t, goComments, "a doc comment up to 120")
	assert.Contains(t, goComments, "sentences up to 50 words")

	yamlGuide := contextAt(t, filepath.Join("config", "app.yaml"))
	assert.Contains(t, yamlGuide, "Voice: site.", "a file a reader parses is written under its own point")
	assert.NotContains(t, yamlGuide, "Code comments")

	commented := contextAt(t, "--comments", filepath.Join("config", "app.yaml"))
	assert.Contains(t, commented, "Voice: source comments.", "--comments asks for the comments' point")
	assert.Contains(t, commented, "Code comments:")
}

// An item declared for its comments alone governs only the comments: the answer
// for its YAML file resolves past it to the project's point, where no item
// claims the file, and --comments still gives the item's comment voice.
func TestContextAnswersForTheContentPastACommentsOnlyItem(t *testing.T) {
	root := writeCommentVoiceProject(t, true)
	recipe := filepath.Join(root, "kapi.yaml")
	data, err := os.ReadFile(recipe)
	require.NoError(t, err)
	const beside = "      - path: \"config/*.yaml\"\n        comments: true\n"
	require.Contains(t, string(data), beside)
	data = bytes.Replace(data, []byte(beside), []byte("      - path: \"config/*.yaml\"\n        comments:\n          only: true\n"), 1)
	require.NoError(t, os.WriteFile(recipe, data, 0o644))
	t.Chdir(root)

	guide := contextAt(t, filepath.Join("config", "app.yaml"))
	assert.Contains(t, guide, "Voice: project.")
	assert.NotContains(t, guide, "Code comments")

	comments := contextAt(t, "--comments", filepath.Join("config", "app.yaml"))
	assert.Contains(t, comments, "Voice: source comments.")
	assert.Contains(t, comments, "Code comments:")
}

// With no comments point declared, a Go file's comments sit at its item's
// point, so --comments gives the site voice, and the answer for the file still
// is for its content at the project's point.
func TestContextCommentsSharingTheirFilesPoint(t *testing.T) {
	t.Chdir(writeCommentVoiceProject(t, false))

	guide := contextAt(t, filepath.Join("code", "parse.go"))
	assert.Contains(t, guide, "Voice: project.")
	assert.NotContains(t, guide, "Code comments")

	comments := contextAt(t, "--comments", filepath.Join("code", "parse.go"))
	assert.Contains(t, comments, "Voice: site.", "with no comments point, a Go file's comments sit at their item's point")
	assert.NotContains(t, comments, "Code comments")
}
