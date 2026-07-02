package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression for the "Website 0 blocks" bug: readers assign file-local block
// IDs ("tu1", "tu2", …) that repeat across files, so keying the project block
// store on the raw ID let blocks from different files/collections collide and
// overwrite each other (last-writer-wins), zeroing out the first-extracted
// collection. Extracting the bundled KapiMart sample must now report every
// collection's real translatable block count.
func TestKapimartExtract_NoCollisionAcrossCollections(t *testing.T) {
	src, err := filepath.Abs("sample/kapimart")
	require.NoError(t, err)
	dst := t.TempDir()
	require.NoError(t, os.CopyFS(dst, os.DirFS(src)))

	app := NewApp()
	tab, err := app.OpenProject(filepath.Join(dst, "project.kapi"))
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	if _, err := app.RunExtract(tab.ID); err != nil {
		t.Fatalf("extract: %v", err)
	}

	st, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)

	got := map[string]int{}
	for _, c := range st.Collections {
		got[c.Name] = c.BlockCount
	}
	// The docs collection (extracted first) is the one that used to come back 0.
	want := map[string]int{
		"Website":      245,
		"Online Store": 349,
		"Contracts":    80,
		"Templates":    25,
	}
	for name, n := range want {
		assert.Equalf(t, n, got[name], "collection %q block count", name)
	}
	// Idempotent: a second extract rebuilds the same set (no accumulation).
	if _, err := app.RunExtract(tab.ID); err != nil {
		t.Fatalf("re-extract: %v", err)
	}
	st2, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)
	for _, c := range st2.Collections {
		assert.Equalf(t, want[c.Name], c.BlockCount, "re-extract collection %q", c.Name)
	}
}
