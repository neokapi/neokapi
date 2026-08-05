package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/terms"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// occurrencesApp wires the in-memory backends the browser build uses, which is
// also what lets this test drive the real command without a project on disk.
func occurrencesApp(t *testing.T) *App {
	t.Helper()

	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
		ID:     "c-widget",
		Domain: "product",
		Terms: []terms.Term{
			{Text: "widget", Locale: model.LocaleEnglish, Status: model.TermDeprecated},
			{Text: "dings", Locale: "nb", Status: model.TermApproved},
		},
	}))
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
		ID:    "c-unused",
		Terms: []terms.Term{{Text: "flywheel", Locale: model.LocaleEnglish}},
	}))

	blocks := blockstore.NewMemoryStore()
	t.Cleanup(func() { _ = blocks.Close() })
	sess, err := blocks.Begin(t.Context())
	require.NoError(t, err)
	for _, b := range []struct {
		hash, id, file, source string
		targets                map[string][]model.Run
	}{
		{"h1", "guide.a", "docs/guide.md", "Drag a widget onto the board.", nil},
		{"h2", "guide.b", "docs/guide.md", "A widget is not a widgetry.", nil},
		{"h3", "guide.c", "docs/guide.md", "Nothing of interest.",
			map[string][]model.Run{"nb": {model.TextR("En dings her.")}}},
	} {
		blk := &blockstore.Block{Hash: b.hash, ID: b.id, Translatable: true,
			Source: []model.Run{model.TextR(b.source)}, Targets: b.targets}
		blk.Properties.File = b.file
		require.NoError(t, sess.PutBlock("docs", blk))
	}
	require.NoError(t, sess.Commit())

	return &App{TermsBackend: tb, BlocksBackend: blocks}
}

// runOccurrences executes the real subcommand and returns what it printed.
func runOccurrences(t *testing.T, a *App, args ...string) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "kapi"}
	root.AddGroup(&cobra.Group{ID: "assets", Title: "Assets"})
	root.AddCommand(NewTermsCmd(a))
	output.AddPersistentFlags(root.PersistentFlags())

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(append([]string{"terms", "occurrences"}, args...))
	err := root.ExecuteContext(t.Context())
	return buf.String(), err
}

func TestTermsOccurrencesText(t *testing.T) {
	a := occurrencesApp(t)

	out, err := runOccurrences(t, a, "widget")
	require.NoError(t, err)
	assert.Contains(t, out, "docs/guide.md")
	assert.Contains(t, out, "guide.a")
	assert.Contains(t, out, "source", "the source text is labelled, not left blank")
	assert.Contains(t, out, "2 use(s) in 2 block(s)",
		"two uses — and 'widgetry' is not one of them")
	assert.NotContains(t, out, "guide.c")
}

func TestTermsOccurrencesJSON(t *testing.T) {
	a := occurrencesApp(t)

	out, err := runOccurrences(t, a, "widget", "--json")
	require.NoError(t, err)

	var got output.TermsOccurrencesOutput
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Equal(t, "widget", got.Subject)
	assert.Equal(t, []string{"c-widget"}, got.ConceptIDs)
	assert.Equal(t, 2, got.Total)
	assert.Equal(t, 2, got.Blocks)
	assert.False(t, got.Truncated)
	require.Len(t, got.Occurrences, 2)
	assert.Equal(t, "guide.a", got.Occurrences[0].BlockID)
	assert.Equal(t, "docs/guide.md", got.Occurrences[0].Document)
	assert.Equal(t, "widget", got.Occurrences[0].Matched)
	assert.Empty(t, got.Occurrences[0].Locale, "the source text carries no locale of its own")
}

// A concept takes every term it carries, so the English source and the
// Norwegian target are both reported, each in its own language.
func TestTermsOccurrencesByConcept(t *testing.T) {
	a := occurrencesApp(t)

	out, err := runOccurrences(t, a, "c-widget", "--json")
	require.NoError(t, err)

	var got output.TermsOccurrencesOutput
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Equal(t, 3, got.Total)

	locales := map[string]int{}
	for _, o := range got.Occurrences {
		locales[o.Locale]++
	}
	assert.Equal(t, map[string]int{"": 2, "nb": 1}, locales)
}

func TestTermsOccurrencesLocaleFilter(t *testing.T) {
	a := occurrencesApp(t)

	out, err := runOccurrences(t, a, "c-widget", "--locale", "nb", "--json")
	require.NoError(t, err)
	var got output.TermsOccurrencesOutput
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	require.Len(t, got.Occurrences, 1)
	assert.Equal(t, "nb", got.Occurrences[0].Locale)

	// "source" is how the source text is named: its own key is the empty
	// string, which is not something anyone can type.
	out, err = runOccurrences(t, a, "c-widget", "--locale", "source", "--json")
	require.NoError(t, err)
	got = output.TermsOccurrencesOutput{}
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Equal(t, 2, got.Total)
	for _, o := range got.Occurrences {
		assert.Empty(t, o.Locale)
	}
}

func TestTermsOccurrencesLimitTruncates(t *testing.T) {
	a := occurrencesApp(t)

	out, err := runOccurrences(t, a, "widget", "--limit", "1", "--json")
	require.NoError(t, err)
	var got output.TermsOccurrencesOutput
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Len(t, got.Occurrences, 1)
	assert.Equal(t, 2, got.Total, "the total counts what was found, not what was shown")
	assert.True(t, got.Truncated)
}

// A term nobody uses is a real answer and must not read like a failure.
func TestTermsOccurrencesUnusedTerm(t *testing.T) {
	a := occurrencesApp(t)

	out, err := runOccurrences(t, a, "flywheel")
	require.NoError(t, err)
	assert.Contains(t, out, `No block uses "flywheel".`)
	assert.Contains(t, out, "c-unused")
}

// A word that is not in the terms store at all is a different thing, and the
// two must not look alike: one is "nothing uses it", the other "no such term".
func TestTermsOccurrencesUnknownSubject(t *testing.T) {
	a := occurrencesApp(t)

	_, err := runOccurrences(t, a, "no such thing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such term or concept")
}

func TestTermsOccurrencesRejectsEmptyLocale(t *testing.T) {
	a := occurrencesApp(t)

	_, err := runOccurrences(t, a, "widget", "--locale", " ")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "use 'source'")
}

// The subcommand carries the same store-resolution flags as its siblings.
// Resolving stores differently from `terms search` would be a parity break
// inside one command group.
func TestTermsOccurrencesCarriesResourceFlags(t *testing.T) {
	cmd, _, err := NewTermsCmd(&App{}).Find([]string{"occurrences"})
	require.NoError(t, err)
	for _, flag := range []string{"name", "local", "file"} {
		assert.NotNil(t, cmd.Flags().Lookup(flag), "--%s", flag)
	}
}
