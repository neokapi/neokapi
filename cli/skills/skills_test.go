package skills

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The skill kapi init writes is short on purpose: four habits, each in its CLI
// and MCP form, and a pointer to `kapi help` for the rest. Agent hosts load the
// description at startup and cap it at 1,024 characters.
func TestSkillIsShort(t *testing.T) {
	body, err := fs.ReadFile(Tree(), "kapi/SKILL.md")
	require.NoError(t, err)
	assert.Less(t, len(strings.Fields(string(body))), 300, "SKILL.md stays under 300 words")

	front, _, ok := strings.Cut(strings.TrimPrefix(string(body), "---\n"), "\n---\n")
	require.True(t, ok, "SKILL.md opens with front matter")
	var description string
	for line := range strings.SplitSeq(front, "\n") {
		if d, ok := strings.CutPrefix(line, "description: "); ok {
			description = d
		}
	}
	require.NotEmpty(t, description)
	assert.Less(t, len(description), 1024)

	for _, want := range []string{
		"context_search", "context_observe", "context_propose", "context_correct",
		"context_withdraw", "context_session_summary", "check_file", "kapi help",
	} {
		assert.Contains(t, string(body), want)
	}
}

// kapi init writes SKILL.md and nothing else.
func TestWiringIsOneFile(t *testing.T) {
	var files []string
	require.NoError(t, fs.WalkDir(Wiring(), ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, p)
		}
		return err
	}))
	assert.Equal(t, []string{"kapi/SKILL.md"}, files)
}

// Every reference file is a topic, and a link from one reference to another
// reads as the command that serves it.
func TestTopics(t *testing.T) {
	names := map[string]Topic{}
	for _, tp := range Topics() {
		names[tp.Name] = tp
		assert.NotEmpty(t, tp.Title, "%s has a title", tp.Name)
	}
	for _, want := range []string{"check", "context", "edit", "growing-context", "i18n", "i18n-react", "i18n-frameworks", "project", "translate", "voice"} {
		assert.Contains(t, names, want)
	}
	assert.Equal(t, "Check what you changed", names["check"].Title)

	text, ok := TopicText("translate")
	require.True(t, ok)
	assert.Contains(t, text, "`kapi help project`")
	assert.NotContains(t, text, "](project.md)")

	_, ok = TopicText("no-such-topic")
	assert.False(t, ok)
}

// A file some kapi release copied into a skill directory is recognised by its
// content: the current references, and the earlier versions the list names.
func TestRetired(t *testing.T) {
	current, err := fs.ReadFile(Tree(), "kapi/references/check.md")
	require.NoError(t, err)
	assert.True(t, Retired(current))
	assert.False(t, Retired([]byte("a person's own notes\n")))
	assert.Greater(t, strings.Count(retiredList, "\n"), 30, "the list covers the releases that copied references")
}
