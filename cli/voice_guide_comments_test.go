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
  voice: .kapi/voice.yaml
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

// The guide for a file answers for the file's own content, and --comments for
// its comments. A Go file's content is its comments alone, so no item governs
// anything else in it: the guide gives the project's voice, and --comments the
// limits a comment there is held to.
func TestVoiceGuideAnswersForAFilesContentAndForItsComments(t *testing.T) {
	t.Chdir(writeCommentVoiceProject(t, true))

	goGuide := voiceGuide(t, filepath.Join("code", "parse.go"))
	assert.Contains(t, goGuide, "# Voice Guide: project", "no item governs a Go file's content")
	assert.NotContains(t, goGuide, "Code comments")

	goComments := voiceGuide(t, "--comments", filepath.Join("code", "parse.go"))
	assert.Contains(t, goComments, "# Voice Guide: source comments")
	assert.Contains(t, goComments, "- Code comments:")
	assert.Contains(t, goComments, "  - A declaration's doc comment: at most 120 words")
	assert.Contains(t, goComments, "  - A sentence over 50 words is a minor finding, and over 70 words a major one")

	yamlGuide := voiceGuide(t, filepath.Join("config", "app.yaml"))
	assert.Contains(t, yamlGuide, "# Voice Guide: site", "a file a reader parses is written under its own point")
	assert.NotContains(t, yamlGuide, "Code comments")

	commented := voiceGuide(t, "--comments", filepath.Join("config", "app.yaml"))
	assert.Contains(t, commented, "# Voice Guide: source comments", "--comments asks for the comments' point")
	assert.Contains(t, commented, "- Code comments:")

	assert.Contains(t, voiceGuide(t, "--comments"), "# Voice Guide: source comments",
		"with no file, --comments resolves the project's comments point")
}

// An item declared for its comments alone governs only the comments: the guide
// for its YAML file resolves past it to the project's point, where no item
// claims the file, and --comments still gives the item's comment voice.
func TestVoiceGuideAnswersForTheContentPastACommentsOnlyItem(t *testing.T) {
	root := writeCommentVoiceProject(t, true)
	recipe := filepath.Join(root, "kapi.yaml")
	data, err := os.ReadFile(recipe)
	require.NoError(t, err)
	const beside = "      - path: \"config/*.yaml\"\n        comments: true\n"
	require.Contains(t, string(data), beside)
	data = bytes.Replace(data, []byte(beside), []byte("      - path: \"config/*.yaml\"\n        comments:\n          only: true\n"), 1)
	require.NoError(t, os.WriteFile(recipe, data, 0o644))
	t.Chdir(root)

	guide := voiceGuide(t, filepath.Join("config", "app.yaml"))
	assert.Contains(t, guide, "# Voice Guide: project")
	assert.NotContains(t, guide, "Code comments")

	comments := voiceGuide(t, "--comments", filepath.Join("config", "app.yaml"))
	assert.Contains(t, comments, "# Voice Guide: source comments")
	assert.Contains(t, comments, "- Code comments:")
}

// With no comments point declared, a Go file's comments sit at its item's
// point, so --comments gives the site voice, and the guide for the file still
// answers for its content at the project's point.
func TestVoiceGuideCommentsSharingTheirFilesPoint(t *testing.T) {
	t.Chdir(writeCommentVoiceProject(t, false))

	guide := voiceGuide(t, filepath.Join("code", "parse.go"))
	assert.Contains(t, guide, "# Voice Guide: project")
	assert.NotContains(t, guide, "Code comments")

	comments := voiceGuide(t, "--comments", filepath.Join("code", "parse.go"))
	assert.Contains(t, comments, "# Voice Guide: site", "with no comments point, a Go file's comments sit at their item's point")
	assert.NotContains(t, comments, "Code comments")
}
