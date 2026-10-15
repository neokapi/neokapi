package skills

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
)

// Every change set the edit topic shows decodes: an agent copies these, and
// an example the decoder refuses teaches a guess. The operations an agent
// reaches for each appear in one.
func TestEditExamplesDecode(t *testing.T) {
	body, err := fs.ReadFile(Tree(), "kapi/references/edit.md")
	require.NoError(t, err)
	seen := map[change.Kind]bool{}
	for _, fence := range jsonFences(string(body)) {
		if !strings.Contains(fence, `"op": `) {
			continue
		}
		set, err := change.Decode(strings.NewReader(fence))
		require.NoError(t, err, "edit.md shows a change set the decoder refuses:\n%s", fence)
		for _, op := range set.Ops {
			seen[op.Kind] = true
		}
	}
	for _, k := range []change.Kind{change.KindSetContent, change.KindReplaceText, change.KindSetAttribute, change.KindMark,
		change.KindInsertBlock, change.KindDeleteBlock} {
		assert.Truef(t, seen[k], "edit.md shows a %s", k)
	}
	text := strings.Join(strings.Fields(string(body)), " ")
	for _, want := range []string{
		"Several operations may name one block",
		"`kapi apply --schema set_attribute`",
		"## Write a translation",
		`"if_match": "absent"`,
		"keep it, and tell the user",
		"send `replace_text` with `find` set to its last words",
		"printf '%s' '<change set>' | kapi apply -",
	} {
		assert.Contains(t, text, want)
	}
	assert.NotContains(t, text, "one operation per block you changed")
}

// jsonFences lists the bodies of a Markdown page's ```json fences.
func jsonFences(text string) []string {
	var out []string
	var cur []string
	inside := false
	for line := range strings.SplitSeq(text, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case !inside && trimmed == "```json":
			inside, cur = true, nil
		case inside && trimmed == "```":
			inside = false
			out = append(out, strings.Join(cur, "\n"))
		case inside:
			cur = append(cur, line)
		}
	}
	return out
}
