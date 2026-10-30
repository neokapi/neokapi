package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host"
)

// `kapi look` end to end through the root: the Map with no path, the Reader
// with one, and bare `kapi` printing the Map.

// writeLookProject writes a project with one docs collection holding files,
// named after the test so the workspace every test in this process shares
// keeps each project's context apart. extra is appended to defaults. It
// returns the project's directory, with project discovery pointed at it.
func writeLookProject(t *testing.T, extra string, files map[string]string) string {
	t.Helper()
	isolateHelp(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	name := strings.ToLower(strings.NewReplacer("/", "-", "_", "-").Replace(t.Name()))
	recipe := "version: v1\nname: " + name + "\ndefaults:\n  source_language: en\n" + extra
	if len(files) > 0 {
		recipe += "collections:\n  - name: docs\n    content:\n      - path: \"docs/*.md\"\n"
	} else {
		recipe += "collections: []\n"
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi.yaml"), []byte(recipe), 0o644))
	for rel, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	t.Setenv("KAPI_NO_PROJECT", "")
	t.Setenv("KAPI_PROJECT", dir)
	t.Chdir(dir)
	return dir
}

// checkFindings is the number of findings `kapi check` reports over the
// project, read from its JSON report.
func checkFindings(t *testing.T) int {
	t.Helper()
	root := newTieredRoot(t)
	var buf strings.Builder
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"check", "--json", "--no-fail"})
	stdout, err := captureStdout(t, root.Execute)
	require.NoError(t, err, buf.String())
	var report struct {
		Summary struct {
			Findings int `json:"findings"`
		} `json:"summary"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout+buf.String()), &report), stdout+buf.String())
	return report.Summary.Findings
}

// noteAsAgent records a term suggestion through `kapi context note`, as an
// assistant would: a person's correction from one wording to another, with
// --suggest recording the rule it implies.
func noteAsAgent(t *testing.T, from, to, seenIn string) {
	t.Helper()
	t.Setenv("KAPI_ACTOR", "agent")
	out := runRoot(t, "context", "note", "--from", from, "--to", to, "--seen-in", seenIn, "--suggest")
	require.Contains(t, out, "[suggested]", out)
}

func TestLookMap(t *testing.T) {
	const clean = "# Guide\n\nOpen the editor to write.\n"
	tests := []struct {
		name  string
		extra string
		files map[string]string
		note  [3]string
		want  []string
		not   []string
	}{
		{
			name: "no content",
			want: []string{"source      no content yet: add files with `kapi add <pattern>`"},
			not:  []string{"blocks", "findings", "kapi look <path>"},
		},
		{
			name:  "content and no findings",
			files: map[string]string{"docs/guide.md": clean},
			want: []string{
				"1 file · 2 blocks · 0 rules · 0 findings",
				"Files\n  docs/guide.md  2 blocks\n",
				"Check with `kapi check`. See a file with `kapi look <path>`.",
			},
			not: []string{"kapi suggests", "What your files do", "languages"},
		},
		{
			name: "findings",
			files: map[string]string{
				"docs/guide.md": "# Guide\n\nOpen the  editor to write.\n",
				"docs/faq.md":   clean,
			},
			want: []string{
				"2 files · 4 blocks · 0 rules · 1 finding",
				"  docs/guide.md  2 blocks · 1 finding\n  docs/faq.md    2 blocks\n",
			},
		},
		{
			name:  "a suggestion waiting",
			files: map[string]string{"docs/guide.md": "# Guide\n\nPlease login to write.\n"},
			note:  [3]string{"login", "sign in", "docs/guide.md"},
			want: []string{
				"kapi suggests 1",
				"What your files do\n  Word  sign in, not login",
				"Decide with `kapi context review`. See a file with `kapi look <path>`.",
			},
		},
		{
			name:  "a target language with no target file named",
			extra: "  target_languages: [nb]\n",
			files: map[string]string{"docs/guide.md": clean},
			want:  []string{"2 languages · 0 blocks waiting for review · kapi status for coverage"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeLookProject(t, tt.extra, tt.files)
			if tt.note[0] != "" {
				noteAsAgent(t, tt.note[0], tt.note[1], tt.note[2])
			}
			out := runRoot(t, "look")
			for _, want := range tt.want {
				assert.Contains(t, out, want)
			}
			for _, not := range tt.not {
				assert.NotContains(t, out, not)
			}
			if len(tt.files) > 0 {
				var m host.LookProject
				require.NoError(t, json.Unmarshal([]byte(runRoot(t, "look", "--json")), &m))
				assert.Equal(t, checkFindings(t), m.Findings, "the Map counts the findings kapi check reports")
			}
		})
	}
}

// TestLookMap_TargetLanguage: a project with a target language adds the line
// with its languages and the blocks `kapi status --review` lists.
func TestLookMap_TargetLanguage(t *testing.T) {
	root := writeStatusProject(t)
	isolateHelp(t)
	t.Setenv("KAPI_PROJECT", root)
	t.Chdir(root)

	out := runRoot(t, "look")
	assert.Contains(t, out, "\n  1 file · 3 blocks · 0 rules · 0 findings\n  3 languages · 2 blocks waiting for review · kapi status for coverage\n")

	review := runRoot(t, "status", "--review", "--json")
	var queue struct {
		Pending []json.RawMessage `json:"pending"`
	}
	require.NoError(t, json.Unmarshal([]byte(review), &queue), review)
	assert.Len(t, queue.Pending, 2, "the count is the review queue's")
}

// TestLookMap_JSON: --json carries the counts, the files and the suggestions.
func TestLookMap_JSON(t *testing.T) {
	writeLookProject(t, "", map[string]string{"docs/guide.md": "# Guide\n\nPlease login  to write.\n"})
	noteAsAgent(t, "login", "sign in", "docs/guide.md")

	var m host.LookProject
	out := runRoot(t, "look", "--json")
	require.NoError(t, json.Unmarshal([]byte(out), &m), out)
	assert.Equal(t, 2, m.Blocks)
	assert.Equal(t, 2, m.Findings, "the doubled space and the suggested rule")
	assert.Equal(t, 1, m.Suggests)
	require.Len(t, m.Suggestions, 1)
	assert.Equal(t, "Word", m.Suggestions[0].Kind)
	assert.Equal(t, "sign in, not login", m.Suggestions[0].Text)
	require.Len(t, m.Files, 1)
	assert.Equal(t, host.LookFileSummary{Path: "docs/guide.md", Blocks: 2, Findings: 2}, m.Files[0])
	assert.Equal(t, "kapi context review", m.Next)
}

// TestLookReader_CaretsUnderTheFinding: a finding with a range is drawn with
// carets under exactly the runes it covers, and its label starts at the first
// caret; a finding with none is a line under the block.
func TestLookReader_CaretsUnderTheFinding(t *testing.T) {
	writeLookProject(t, "", map[string]string{"docs/guide.md": "# Guide\n\nPlease login to  write.\n"})
	noteAsAgent(t, "login", "sign in", "docs/guide.md")

	out := runRoot(t, "look", "docs/guide.md")
	lines := strings.Split(out, "\n")
	assert.Equal(t, "docs/guide.md · 2 blocks · 2 findings", lines[0])

	text := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "guide/p") {
			text = i
		}
	}
	require.GreaterOrEqual(t, text, 0, out)
	col := strings.Index(lines[text], "login")
	require.Positive(t, col)
	assert.Equal(t, strings.Repeat(" ", col)+"^^^^^", lines[text+1], "carets under login:\n%s", out)
	assert.Equal(t, strings.Repeat(" ", col)+"Word: sign in, not login (suggested)", lines[text+2])
	assert.Equal(t, strings.Repeat(" ", strings.Index(lines[text], "Please"))+"Hygiene: Content contains consecutive spaces", lines[text+3])

	assert.Contains(t, out, "Rules here\n  Word  sign in, not login  suggested, by ")
	assert.Contains(t, out, "`kapi check docs/guide.md` for the report. `kapi context docs/guide.md` for an assistant.")
}

// TestLookReader_NoFindings: a clean file shows its blocks and no carets.
func TestLookReader_NoFindings(t *testing.T) {
	writeLookProject(t, "", map[string]string{"docs/guide.md": "# Guide\n\nOpen the editor to write.\n"})

	out := runRoot(t, "look", "docs/guide.md")
	assert.True(t, strings.HasPrefix(out, "docs/guide.md · 2 blocks · 0 findings\n\nh        Guide\nguide/p  Open the editor to write.\n"), out)
	assert.NotContains(t, out, "^")
	assert.NotContains(t, out, "Rules here")
}

// TestLookReader_OutsideTheCollections: a file the recipe does not read is
// drawn anyway, opening with the line that says so and carrying no rules.
func TestLookReader_OutsideTheCollections(t *testing.T) {
	dir := writeLookProject(t, "", map[string]string{"docs/guide.md": "# Guide\n\nOpen the editor.\n"})
	require.NoError(t, os.WriteFile(filepath.Join(dir, "NOTES.md"), []byte("Notes  here.\n"), 0o644))
	noteAsAgent(t, "editor", "writer", "docs/guide.md")

	out := runRoot(t, "look", "NOTES.md")
	lines := strings.Split(out, "\n")
	assert.Equal(t, "not in the project's collections", lines[0])
	assert.Equal(t, "NOTES.md · 1 block · 1 finding", lines[1])
	assert.NotContains(t, out, "Rules here")
	assert.Contains(t, out, "`kapi check NOTES.md` for the report.\n")
	assert.NotContains(t, out, "kapi context NOTES.md")
}

// TestLookReader_JSON: --json holds the blocks, the findings with their spans,
// and the rules, as one object.
func TestLookReader_JSON(t *testing.T) {
	writeLookProject(t, "", map[string]string{"docs/guide.md": "# Guide\n\nPlease login to write.\n"})
	noteAsAgent(t, "login", "sign in", "docs/guide.md")

	var r host.LookReader
	out := runRoot(t, "look", "docs/guide.md", "--json")
	require.NoError(t, json.Unmarshal([]byte(out), &r), out)
	assert.Equal(t, "docs/guide.md", r.Path)
	assert.True(t, r.Declared)
	require.Len(t, r.Blocks, 2)
	assert.Equal(t, "guide/p", r.Blocks[1].ID)
	assert.Equal(t, "Please login to write.", r.Blocks[1].Text)
	require.Len(t, r.Blocks[1].Findings, 1)
	f := r.Blocks[1].Findings[0]
	assert.Equal(t, "terms.vocabulary", f.Rule)
	require.NotNil(t, f.Span)
	assert.Equal(t, host.LookSpan{Start: 7, End: 12}, *f.Span)
	require.Len(t, r.Rules, 1)
	assert.Equal(t, "Word", r.Rules[0].Kind)
	assert.Equal(t, "suggested", r.Rules[0].Status)
}

// TestRootHelp_ListsLookFirst: look is the first command of the Work group on
// the root help, and the rest of the group keeps its order.
func TestRootHelp_ListsLookFirst(t *testing.T) {
	isolateHelp(t)
	t.Setenv("KAPI_NO_PROJECT", "1")

	rows := commandRows(helpSection(runRoot(t, "--help"), "Work:"))
	require.NotEmpty(t, rows)
	assert.Equal(t, "look", rows[0])
	rest := append([]string(nil), rows[1:]...)
	assert.IsIncreasing(t, rest, "the rest of the group stays in cobra's order")
}

// TestBareKapi_PrintsTheMap: bare `kapi` in a project is the Map, its own last
// line, and the line naming where the commands are listed.
func TestBareKapi_PrintsTheMap(t *testing.T) {
	writeLookProject(t, "", map[string]string{"docs/guide.md": "# Guide\n\nOpen the editor to write.\n"})

	bare := runRoot(t)
	look := runRoot(t, "look")
	assert.Equal(t, look+"\nkapi --help for the commands\n", bare)
}
