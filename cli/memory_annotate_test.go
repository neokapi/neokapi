package cli

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
)

func TestMemoryAnnotate_MarksTheEntriesHoldingTheText(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	tm, err := memory.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	add := func(id, en, fr string) {
		t.Helper()
		require.NoError(t, tm.Add(t.Context(), memory.Entry{ID: id, HintSrcLang: "en", Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: en}}},
			"fr": {{Text: &model.TextRun{Text: fr}}},
		}}))
	}
	add("one", "Contact Acme today", "Contactez Acme aujourd'hui")
	add("two", "Nothing to see", "Rien à voir")
	require.NoError(t, tm.Close())

	a := newExtractApp(t)
	cmd := NewMemoryCmd(a)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"annotate", "Acme", "--type", "organization", "--file", dbPath})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "Marked 2 entities in 1 entry.")

	tm, err = memory.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tm.Close() })
	got, found, err := tm.GetEntry(t.Context(), "one")
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, got.Entities, 1)
	assert.Equal(t, model.EntityOrganization, got.Entities[0].Type)
	assert.Len(t, got.Entities[0].Values, 2, "the entity is paired across both languages")
	other, _, err := tm.GetEntry(t.Context(), "two")
	require.NoError(t, err)
	assert.Empty(t, other.Entities)
}

func TestMemoryAnnotate_RefusesAnUnknownType(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	a := newExtractApp(t)
	cmd := NewMemoryCmd(a)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"annotate", "Acme", "--type", "galaxy", "--file", dbPath})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not an entity type")
}
