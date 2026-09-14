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
// the comments of both placed at source/comments when apart is set. The source
// voice sets comment limits and the site voice sets none.
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
	write(".kapi/profiles/site/voice.yaml", "name: site\n")
	write(".kapi/profiles/source/voice.yaml", "name: source comments\nstyle:\n  comments:\n    doc_words: 120\n")
	write("code/parse.go", "package code\n\n// Parse reads the input.\nfunc Parse() {}\n")
	write("config/app.yaml", "# The greeting.\ngreeting: Hello\n")
	return root
}

// voiceGuide runs `kapi voice guide` with args in the working directory.
func voiceGuide(t *testing.T, args ...string) string {
	t.Helper()
	cmd := NewVoiceCmd(&App{})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"guide"}, args...))
	require.NoError(t, cmd.Execute())
	return out.String()
}

// A file whose only content is its comments is written under the voice at the
// point its comments sit at, so the guide for it lists the comment limits an
// agent writing a comment there is held to.
func TestVoiceGuideResolvesTheCommentsPoint(t *testing.T) {
	t.Chdir(writeCommentVoiceProject(t, true))

	goGuide := voiceGuide(t, filepath.Join("code", "parse.go"))
	assert.Contains(t, goGuide, "# Voice Guide: source comments")
	assert.Contains(t, goGuide, "- Code comments:")
	assert.Contains(t, goGuide, "  - A declaration's doc comment: at most 120 words")
	assert.Contains(t, goGuide, "  - A sentence over 50 words is a minor finding, and over 70 words a major one")

	yamlGuide := voiceGuide(t, filepath.Join("config", "app.yaml"))
	assert.Contains(t, yamlGuide, "# Voice Guide: site", "a file a reader parses is written under its own point")
	assert.NotContains(t, yamlGuide, "Code comments")

	commented := voiceGuide(t, "--comments", filepath.Join("config", "app.yaml"))
	assert.Contains(t, commented, "# Voice Guide: source comments", "--comments asks for the comments' point")
	assert.Contains(t, commented, "- Code comments:")

	assert.Contains(t, voiceGuide(t, "--comments"), "# Voice Guide: source comments",
		"with no file, --comments resolves the project's comments point")
}

func TestVoiceGuideCommentsSharingTheirFilesPoint(t *testing.T) {
	t.Chdir(writeCommentVoiceProject(t, false))

	guide := voiceGuide(t, filepath.Join("code", "parse.go"))
	assert.Contains(t, guide, "# Voice Guide: site", "must fail: with no comments point, a Go file's comments sit at its file's point")
	assert.NotContains(t, guide, "Code comments")
}
