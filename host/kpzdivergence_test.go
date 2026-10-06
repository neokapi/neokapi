package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
)

const kpzMessages = `{"greeting": "Hello there", "farewell": "Goodbye now", "thanks": "Thank you"}` + "\n"

// kpzProject is a project whose directory holds work.kpz, extracted with its
// source, and the project's recipe.
func kpzProject(t *testing.T) (*App, string, string) {
	t.Helper()
	t.Setenv("KAPI_KPZ_CACHE", t.TempDir())
	root := t.TempDir()
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte("version: v1\nname: kpz\ndefaults:\n  source_language: en\n  target_languages: [fr]\n"), 0o644))
	src := filepath.Join(t.TempDir(), "messages.json")
	require.NoError(t, os.WriteFile(src, []byte(kpzMessages), 0o644))
	a := &App{SourceLang: "en", Quiet: true}
	a.InitRegistries()
	work := filepath.Join(root, "work.kpz")
	require.NoError(t, a.ExtractToKpz(context.Background(), []string{src}, work, "fr", "", true))
	return a, recipe, work
}

// replaceKpz writes over work a KPZ extracted from source, as a teammate's
// pack or a fresh extract replaces the file.
func replaceKpz(t *testing.T, a *App, work, source string) {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "messages.json")
	require.NoError(t, os.WriteFile(src, []byte(source), 0o644))
	other := filepath.Join(dir, "other.kpz")
	require.NoError(t, a.ExtractToKpz(context.Background(), []string{src}, other, "fr", "", true))
	data, err := os.ReadFile(other)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(work, data, 0o644))
}

// kpzSet sets the block of doc whose text is from to to, through the
// project's change service, as a person.
func kpzSet(t *testing.T, a *App, recipe, doc, from, to string) *change.Result {
	t.Helper()
	ctx := context.Background()
	svc, err := a.ChangeService(ctx, ChangeServiceOptions{Project: recipe, Origin: "desktop"})
	require.NoError(t, err)
	page, err := svc.Read(ctx, change.ReadRequest{Doc: doc})
	require.NoError(t, err)
	for _, b := range page.Blocks {
		if b.Text != from {
			continue
		}
		res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{{Kind: change.KindSetContent, At: b.Ref, IfMatch: b.Rev,
			Body: &change.SetContent{Text: &to}}}}, changePerson)
		require.NoError(t, err)
		return res
	}
	t.Fatalf("no block of %s reads %q", doc, from)
	return nil
}

// kpzTexts reads every block of doc through the project's change service,
// with the writes that did not land.
func kpzTexts(t *testing.T, a *App, recipe, doc string) ([]string, *change.Page) {
	t.Helper()
	svc, err := a.ChangeService(context.Background(), ChangeServiceOptions{Project: recipe, Origin: "status"})
	require.NoError(t, err)
	page, err := svc.Read(context.Background(), change.ReadRequest{Doc: doc})
	require.NoError(t, err)
	var out []string
	for _, b := range page.Blocks {
		out = append(out, b.Text)
	}
	return out, page
}

// TestKpzDivergence_AReplacedKPZIsAConflictToRebaseOrDiscard: when the .kpz
// on disk is replaced while a document it carries holds edits nobody packed,
// the new version is a write that did not land. A read reports it, the
// project lists it as a conflict, and a person rebases it onto the edits,
// deciding any block both changed, or discards it.
func TestKpzDivergence_AReplacedKPZIsAConflictToRebaseOrDiscard(t *testing.T) {
	const doc = "work.kpz!messages.json"
	tests := []struct {
		name string
		// outside is the source the replacing KPZ carries.
		outside string
		// discard discards the write in place of a rebase.
		discard bool
		// contested is how many blocks the rebase leaves for a person, and
		// decide the wording the person writes into each.
		contested int
		decide    string
		want      []string
	}{
		{
			name:    "a rebase carries a change to another block over",
			outside: `{"greeting": "Hello there", "farewell": "Goodbye from the team", "thanks": "Thank you"}` + "\n",
			want:    []string{"Hello, edited here", "Goodbye from the team", "Thank you"},
		},
		{
			name:      "a rebase leaves a block both changed for a person",
			outside:   `{"greeting": "Hello from the team", "farewell": "Goodbye now", "thanks": "Thanks a lot"}` + "\n",
			contested: 1,
			decide:    "Hello from both",
			want:      []string{"Hello from both", "Goodbye now", "Thanks a lot"},
		},
		{
			name:    "a discard keeps the edits",
			outside: `{"greeting": "Hello from the team", "farewell": "Goodbye from the team", "thanks": "Thank you"}` + "\n",
			discard: true,
			want:    []string{"Hello, edited here", "Goodbye now", "Thank you"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			a, recipe, work := kpzProject(t)
			res := kpzSet(t, a, recipe, doc, "Hello there", "Hello, edited here")
			require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

			none, err := a.KeptConflicts(ctx, recipe)
			require.NoError(t, err)
			assert.Empty(t, none, "an edit alone is no conflict")

			replaceKpz(t, a, work, tt.outside)
			conflicts, err := a.KeptConflicts(ctx, recipe)
			require.NoError(t, err)
			require.Len(t, conflicts, 1)
			c := conflicts[0]
			assert.Equal(t, ConflictDocument, c.Kind)
			assert.Equal(t, doc, c.Doc)
			assert.False(t, c.Rebased)
			assert.Empty(t, c.Blocks, "nothing is contested before a rebase")
			require.NotEmpty(t, c.Edit)

			texts, page := kpzTexts(t, a, recipe, doc)
			require.Len(t, page.Divergent, 1, "a read reports the version that did not land")
			assert.Equal(t, c.Edit, page.Divergent[0].Op)
			assert.Equal(t, []string{"Hello, edited here", "Goodbye now", "Thank you"}, texts, "the edits stay the head")

			again, err := a.KeptConflicts(ctx, recipe)
			require.NoError(t, err)
			assert.Len(t, again, 1, "reading the replaced file again records nothing more")

			if tt.discard {
				require.NoError(t, a.DiscardKeptDocument(ctx, recipe, doc, c.Edit, "desktop"))
			} else {
				rb, err := a.RebaseKeptDocument(ctx, recipe, doc, c.Edit, "desktop")
				require.NoError(t, err)
				assert.Empty(t, rb.Refused)
				assert.Equal(t, tt.contested, rb.Contested)
				assert.Positive(t, rb.Carried, "the change to a block the edits left alone is carried over")
			}

			if tt.contested > 0 {
				conflicts, err = a.KeptConflicts(ctx, recipe)
				require.NoError(t, err)
				require.Len(t, conflicts, 1)
				c = conflicts[0]
				assert.True(t, c.Rebased)
				require.Len(t, c.Blocks, tt.contested)
				b := c.Blocks[0]
				assert.Equal(t, "Hello, edited here", b.Held.Text)
				assert.Equal(t, "Hello from the team", b.Other.Text)
				assert.Empty(t, b.Edition, "the document's own edition")

				svc, err := a.ChangeService(ctx, ChangeServiceOptions{Project: recipe, Origin: "desktop"})
				require.NoError(t, err)
				text := tt.decide
				res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{{Kind: change.KindSetContent,
					At: change.Ref{Doc: c.Doc, Block: b.Block}, IfMatch: b.Held.Rev, Body: &change.SetContent{Text: &text}}}}, changePerson)
				require.NoError(t, err)
				require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
			}

			conflicts, err = a.KeptConflicts(ctx, recipe)
			require.NoError(t, err)
			assert.Empty(t, conflicts, "the conflict is settled")
			texts, page = kpzTexts(t, a, recipe, doc)
			assert.Empty(t, page.Divergent)
			assert.Equal(t, tt.want, texts)

			// The settled document reaches the pack.
			require.NoError(t, a.PackKpz(ctx, work))
			out := filepath.Join(t.TempDir(), "out")
			require.NoError(t, a.MergeFromKpz(ctx, work, out))
		})
	}
}

// TestKpzDivergence_ARebaseOfAWriteThatIsNotDivergentIsRefused: a write the
// log does not list beside the document's head has nothing to settle, and a
// reference outside every KPZ of the project names no document to rebase.
func TestKpzDivergence_ARebaseOfAWriteThatIsNotDivergentIsRefused(t *testing.T) {
	ctx := context.Background()
	a, recipe, _ := kpzProject(t)
	res := kpzSet(t, a, recipe, "work.kpz!messages.json", "Hello there", "Hello, edited here")
	require.Equal(t, change.SetApplied, res.Status)
	require.NotNil(t, res.Record)

	_, err := a.RebaseKeptDocument(ctx, recipe, "work.kpz!messages.json", *res.Record, "desktop")
	require.ErrorContains(t, err, "not divergent")
	assert.ErrorIs(t, a.DiscardKeptDocument(ctx, recipe, "missing.kpz!messages.json", *res.Record, "desktop"), ErrNotKpzDocument)
}
