package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Each self-test case holds the guard to its status, and the must-fail cases
// fail on the fixture that breaks coverage.
func TestSelfTestCases(t *testing.T) {
	for _, c := range selfTestCases {
		t.Run(c.name, func(t *testing.T) {
			got, out, err := runCase(c)
			require.NoError(t, err)
			assert.Equal(t, c.want, got, out)
			if c.names != "" {
				assert.Contains(t, out, c.names)
			}
		})
	}
}

// The check itself, without git: a file under an unlisted directory is
// reported once, under its own family, and an excluded one is not.
func TestCheckNamesUnclaimedFiles(t *testing.T) {
	dir := t.TempDir()
	for rel, body := range map[string]string{
		"kapi.yaml":       fixtureRecipe("    - \"vendor/**\"\n"),
		"core/a.go":       "package core\n",
		"newpkg/c.go":     "package newpkg\n",
		"vendor/v.go":     "package vendor\n",
		"docs/guide.yaml": "# A guide.\ntitle: Guide\n",
	} {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	r, err := check(filepath.Join(dir, "kapi.yaml"), []string{"core/a.go", "newpkg/c.go", "vendor/v.go", "docs/guide.yaml", "kapi.yaml"})
	require.NoError(t, err)
	assert.Equal(t, []string{"newpkg/c.go"}, r.unclaimed["Go"])
	assert.Equal(t, []string{"docs/guide.yaml"}, r.unclaimed["YAML"], "the recipe is excluded, and the guide is not")
	assert.Equal(t, 3, r.checked["Go"])
	var out bytes.Buffer
	assert.Equal(t, 1, r.write(&out))
	assert.Contains(t, out.String(), "newpkg/c.go")
}
