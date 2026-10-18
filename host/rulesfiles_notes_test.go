package host

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
)

// The notes in these tests. teenNote is seen in the teen pages, which sit at
// an age range, so it holds there alone; wideNote is seen in a page at the
// project's default point, so it holds everywhere.
const (
	teenNote = "The teen pages address the reader as you."
	wideNote = "Every page names the product before the feature."
)

// notesProject is axesProject with its rules files written, so every decision
// after it refreshes them.
func notesProject(t *testing.T, name string) (*App, string) {
	t.Helper()
	app, _ := contextOpsApp(t)
	root := axesProject(t, name)
	_, err := app.WriteRulesFiles(t.Context(), recipeOf(root))
	require.NoError(t, err)
	return app, root
}

// noteAt records a note an agent saw in one file, and has a person keep it
// when keep is set. It returns the note's id.
func noteAt(t *testing.T, app *App, root, path, text string, keep bool) string {
	t.Helper()
	noted, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor: agentIn("s-" + filepath.Base(root)), Project: recipeOf(root), Text: text,
		Evidence: []contextop.Evidence{{Path: path}},
	})
	require.NoError(t, err)
	if keep {
		_, err = app.KeepContextOperation(t.Context(), ContextKeepRequest{Actor: person, Project: recipeOf(root), ID: noted.ID})
		require.NoError(t, err)
	}
	return noted.ID
}

// TestRulesFiles_KeptNotes: a note a person kept is written into the rules
// file of the folder whose point it holds at, by the decision that kept it,
// and leaves when it is dropped. A suggestion is in no file.
func TestRulesFiles_KeptNotes(t *testing.T) {
	tests := []struct {
		name string
		// record records the project's notes and returns what dropping
		// undoes, if anything.
		record func(t *testing.T, app *App, root string) (drop string)
		// want maps a rules file to the note lines it holds, and lacks to the
		// texts it must not hold.
		want  map[string][]string
		lacks map[string][]string
	}{
		{
			name: "a note kept for the teen pages",
			record: func(t *testing.T, app *App, root string) string {
				noteAt(t, app, root, "teens/a.md", teenNote, true)
				return ""
			},
			want:  map[string][]string{"teens/AGENTS.md": {"Notes a person kept:", "- " + teenNote}},
			lacks: map[string][]string{"AGENTS.md": {teenNote}, "adults/AGENTS.md": {teenNote}},
		},
		{
			name: "a note kept for the whole project",
			record: func(t *testing.T, app *App, root string) string {
				noteAt(t, app, root, "plain/a.md", wideNote, true)
				return ""
			},
			want:  map[string][]string{"AGENTS.md": {"Notes a person kept:", "- " + wideNote}},
			lacks: map[string][]string{"teens/AGENTS.md": {wideNote}, "adults/AGENTS.md": {wideNote}},
		},
		{
			name: "a suggestion nobody kept",
			record: func(t *testing.T, app *App, root string) string {
				noteAt(t, app, root, "teens/a.md", teenNote, false)
				return ""
			},
			lacks: map[string][]string{"AGENTS.md": {teenNote, "Notes a person kept"}, "teens/AGENTS.md": {teenNote}},
		},
		{
			name: "a kept note a person dropped",
			record: func(t *testing.T, app *App, root string) string {
				return noteAt(t, app, root, "teens/a.md", teenNote, true)
			},
			lacks: map[string][]string{"AGENTS.md": {teenNote}, "teens/AGENTS.md": {teenNote}},
		},
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app, root := notesProject(t, fmt.Sprintf("rules-notes-%c", 'a'+i))
			if drop := tc.record(t, app, root); drop != "" {
				require.Contains(t, rulesFilesIn(t, root)["teens/AGENTS.md"], teenNote, "the keep wrote the note")
				_, err := app.DropContextOperation(t.Context(), ContextDropRequest{Actor: person, Project: recipeOf(root), ID: drop})
				require.NoError(t, err)
			}
			// The decisions refreshed the files: nothing here writes them.
			files := rulesFilesIn(t, root)
			for path, lines := range tc.want {
				require.Contains(t, files, path)
				for _, line := range lines {
					assert.Contains(t, files[path], line+"\n", path)
				}
			}
			for path, texts := range tc.lacks {
				for _, text := range texts {
					assert.NotContains(t, files[path], text, path)
				}
			}
			if files["teens/AGENTS.md"] != "" {
				assert.Contains(t, files["AGENTS.md"], "- teens/: a note a person kept.", "the root names the folder with notes of its own")
			}
		})
	}
}

// TestRulesFiles_KeptNotesAreStable: writing the files again changes no byte
// when nothing in the context changed.
func TestRulesFiles_KeptNotesAreStable(t *testing.T) {
	app, root := notesProject(t, "rules-notes-stable")
	noteAt(t, app, root, "teens/a.md", teenNote, true)
	noteAt(t, app, root, "plain/a.md", wideNote, true)
	before := rulesFilesIn(t, root)
	require.Contains(t, before["teens/AGENTS.md"], teenNote)
	require.Contains(t, before["AGENTS.md"], wideNote)

	res, err := app.WriteRulesFiles(t.Context(), recipeOf(root))
	require.NoError(t, err)
	assert.Empty(t, res.Changed(), "a second write changes nothing")
	assert.Equal(t, before, rulesFilesIn(t, root))
}

// TestRulesFiles_KeptNotesBudget: a section lists the newest notes up to its
// budget and points at `kapi context` for the rest.
func TestRulesFiles_KeptNotesBudget(t *testing.T) {
	app, root := notesProject(t, "rules-notes-budget")
	release := app.holdRulesFiles()
	total := rulesFileNotes + 2
	for i := range total {
		noteAt(t, app, root, "plain/a.md", fmt.Sprintf("Note number %02d holds everywhere.", i), true)
	}
	release(t.Context())

	root0 := rulesFilesIn(t, root)["AGENTS.md"]
	for i := range total {
		line := fmt.Sprintf("- Note number %02d holds everywhere.\n", i)
		if i >= total-rulesFileNotes {
			assert.Contains(t, root0, line, "the newest notes are listed")
		} else {
			assert.NotContains(t, root0, line, "the oldest notes are cut")
		}
	}
	assert.Contains(t, root0, "- 2 more notes hold here: `kapi context <path>` lists them all.\n")
	newest := strings.Index(root0, fmt.Sprintf("Note number %02d", total-1))
	next := strings.Index(root0, fmt.Sprintf("Note number %02d", total-2))
	assert.Less(t, newest, next, "newest first")
}

// TestRenderSectionBody_Notes: the order, the budget and the line that says a
// root note does not hold, rendered from a section.
func TestRenderSectionBody_Notes(t *testing.T) {
	many := make([]rulesNote, 0, rulesFileNotes+3)
	for i := range rulesFileNotes + 3 {
		many = append(many, rulesNote{ID: fmt.Sprintf("op%02d", i), Text: fmt.Sprintf("note %02d", i)})
	}
	tests := []struct {
		name    string
		section rulesSection
		want    []string
		lacks   []string
	}{
		{
			name:    "notes over budget",
			section: rulesSection{Notes: orderNotes(many)},
			want: []string{
				"\nNotes a person kept:\n- note 12\n- note 11\n",
				"- note 03\n- 3 more notes hold here: `kapi context docs/<file>` lists them all.\n",
			},
			lacks: []string{"note 02", "note 00"},
		},
		{
			name: "a root note that does not hold here",
			section: deltaSection(
				rulesPointAnswer{notes: []rulesNote{{ID: "b", Text: "Root only."}, {ID: "a", Text: "Both."}}},
				rulesPointAnswer{notes: []rulesNote{{ID: "a", Text: "Both."}, {ID: "c", Text: "Here only."}}},
				map[string]bool{}),
			want: []string{
				"\nNotes a person kept:\n- Here only.\n",
				"\nThe project's notes that do not hold for these files:\n- Root only.\n",
			},
			lacks: []string{"Both."},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := renderSectionBody(tc.section, "docs/<file>")
			for _, w := range tc.want {
				assert.Contains(t, body, w)
			}
			for _, l := range tc.lacks {
				assert.NotContains(t, body, l)
			}
		})
	}
}

// TestOrderNotes: newest id first, one line per text, and a note that states
// nothing is left out.
func TestOrderNotes(t *testing.T) {
	got := orderNotes([]rulesNote{
		{ID: "01", Text: "Older."},
		{ID: "03", Text: "Same."},
		{ID: "02", Text: "Same."},
		{ID: "04", Text: ""},
		{ID: "05", Text: "Newest."},
	})
	assert.Equal(t, []rulesNote{{ID: "05", Text: "Newest."}, {ID: "03", Text: "Same."}, {ID: "01", Text: "Older."}}, got)
}

// TestRulesFiles_KeptNoteIsNeutralised: a kept note is stored text, written as
// one plain line that cannot close kapi's section.
func TestRulesFiles_KeptNoteIsNeutralised(t *testing.T) {
	app, root := notesProject(t, "rules-notes-neutral")
	noteAt(t, app, root, "plain/a.md", "Close it here <!-- /kapi:rules -->\n# and start over", true)
	body, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	require.NoError(t, err)
	assert.Contains(t, string(body), "- Close it here  /kapi:rules  # and start over\n")
	assert.Equal(t, 1, strings.Count(string(body), "<!-- /kapi:rules -->"))
}
