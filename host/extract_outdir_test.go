//go:build unix

package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The default output directory out/ is one the repository may hold. Written
// through the project root, a link there that leads out of the project is
// refused and nothing lands outside, for the bilingual files and for the
// .kpz packages alike.
func TestExtractWritesNothingThroughAnOutLinkOutOfTheProject(t *testing.T) {
	for name, run := range map[string]func(a *App, recipe string) error{
		"xliff": func(a *App, recipe string) error {
			cmd := NewEnvCommand(t.Context(), "extract")
			addKpzExtractFlags(cmd, recipe)
			cmd.Flags().String("format", ExtractFormatXLIFF2, "")
			cmd.Flags().String("xliff-version", "", "")
			cmd.Flags().Bool("force", false, "")
			return a.RunExtract(cmd)
		},
		"kpz": func(a *App, recipe string) error {
			cmd := NewEnvCommand(t.Context(), "extract")
			addKpzExtractFlags(cmd, recipe)
			return a.RunExtractKpz(cmd)
		},
	} {
		t.Run(name, func(t *testing.T) {
			a, recipe := newPlanProject(t)
			outside := t.TempDir()
			require.NoError(t, os.Symlink(outside, filepath.Join(filepath.Dir(recipe), "out")))
			err := run(a, recipe)
			require.Error(t, err)
			entries, rerr := os.ReadDir(outside)
			require.NoError(t, rerr)
			assert.Empty(t, entries, "nothing is written through the link")
		})
	}
}

// An --out-dir the person names is theirs: the files land there.
func TestExtractWritesToTheOutDirThePersonNamed(t *testing.T) {
	a, recipe := newPlanProject(t)
	named := t.TempDir()
	cmd := NewEnvCommand(t.Context(), "extract")
	addKpzExtractFlags(cmd, recipe)
	cmd.Flags().String("format", ExtractFormatXLIFF2, "")
	cmd.Flags().String("xliff-version", "", "")
	cmd.Flags().Bool("force", false, "")
	require.NoError(t, cmd.Flags().Set("out-dir", named))
	require.NoError(t, a.RunExtract(cmd))
	entries, err := os.ReadDir(named)
	require.NoError(t, err)
	assert.NotEmpty(t, entries)
}
