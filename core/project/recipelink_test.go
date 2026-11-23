package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// discoveryEnv clears what would make ResolveRecipePath skip the walk.
func discoveryEnv(t *testing.T) {
	t.Helper()
	t.Setenv(NoProjectEnvVar, "")
	t.Setenv(ProjectEnvVar, "")
}

// A checkout can commit a kapi.yaml that is a symbolic link to a file outside
// it. Discovery refuses it, naming the link; the same link named with -p or
// KAPI_PROJECT is the person's choice, and is followed.
func TestDiscoveryRefusesARecipeLinkOutOfItsDirectory(t *testing.T) {
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.yaml")
	require.NoError(t, os.WriteFile(secret, []byte("version: v1\nname: outside\n"), 0o644))
	checkout := t.TempDir()
	link := filepath.Join(checkout, RecipeFileName)
	require.NoError(t, os.Symlink(secret, link))
	sub := filepath.Join(checkout, "docs")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	t.Run("ResolveLayout refuses it", func(t *testing.T) {
		_, err := ResolveLayout(sub)
		require.Error(t, err)
		assert.Contains(t, err.Error(), RecipeFileName+" is a symbolic link that leads outside")
		assert.NotErrorIs(t, err, ErrNoProject)
	})
	t.Run("the upward walk from the working directory refuses it", func(t *testing.T) {
		discoveryEnv(t)
		t.Chdir(sub)
		got, err := ResolveRecipePath("")
		require.Error(t, err)
		assert.Empty(t, got)
		assert.Contains(t, err.Error(), "symbolic link that leads outside")
	})
	t.Run("-p follows it", func(t *testing.T) {
		got, err := ResolveRecipePath(link)
		require.NoError(t, err)
		assert.Equal(t, secret, got)
	})
	t.Run("KAPI_PROJECT follows it", func(t *testing.T) {
		t.Setenv(NoProjectEnvVar, "")
		t.Setenv(ProjectEnvVar, checkout)
		got, err := ResolveRecipePath("")
		require.NoError(t, err)
		assert.Equal(t, secret, got)
	})
}

// A recipe link that stays inside the directory holding it is followed by
// discovery, and the project roots at that directory.
func TestDiscoveryFollowsARecipeLinkInsideItsDirectory(t *testing.T) {
	checkout := t.TempDir()
	target := filepath.Join(checkout, "recipes", "main.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	require.NoError(t, os.WriteFile(target, []byte("version: v1\nname: inside\n"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join("recipes", "main.yaml"), filepath.Join(checkout, RecipeFileName)))

	l, err := ResolveLayout(checkout)
	require.NoError(t, err)
	assert.Equal(t, checkout, l.Root)
	p, err := Load(l.RecipePath)
	require.NoError(t, err)
	assert.Equal(t, "inside", p.Name)
}

// A discovered recipe that is a link to nothing is refused with what to do,
// rather than walked past to a project further up.
func TestDiscoveryRefusesADanglingRecipeLink(t *testing.T) {
	checkout := t.TempDir()
	require.NoError(t, os.Symlink("gone.yaml", filepath.Join(checkout, RecipeFileName)))
	_, err := ResolveLayout(checkout)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symbolic link to a file that does not exist")
}

// A recipe parse error quotes at most the offending line of the file. The
// lines after it stay out of the message, whichever decoder reports it.
func TestRecipeParseErrorQuotesOneLine(t *testing.T) {
	for _, tc := range []struct {
		name, body, says string
	}{
		{
			name: "a block scalar where comments takes a boolean",
			body: "version: v1\nname: p\ncollections:\n  - name: c\n    content:\n      - path: a.yaml\n        comments: |\n          first line\n          SECOND-LINE\n          THIRD-LINE\n",
			says: "first line",
		},
		{
			name: "a block list where requires takes a map",
			body: "version: v1\nname: p\nrequires:\n  - first-plugin\n  - SECOND-LINE\n  - THIRD-LINE\n",
			says: "bare-list form",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recipe := filepath.Join(t.TempDir(), RecipeFileName)
			require.NoError(t, os.WriteFile(recipe, []byte(tc.body), 0o644))
			_, err := Load(recipe)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.says)
			assert.NotContains(t, err.Error(), "SECOND-LINE")
			assert.NotContains(t, err.Error(), "THIRD-LINE")
		})
	}
}

// A long line is capped, and a message is cut at its first line break.
func TestRecipeYAMLErrorCapsEachMessage(t *testing.T) {
	long := make([]byte, 2*maxParseMessage)
	for i := range long {
		long[i] = 'x'
	}
	err := RecipeYAMLError(os.ErrInvalid)
	require.ErrorIs(t, err, os.ErrInvalid)

	cut := firstLine(string(long)+"\nnext", maxParseMessage, " ...")
	assert.Len(t, cut, maxParseMessage+len(" ..."))
	assert.Equal(t, "one ...", firstLine("one\ntwo", maxParseMessage, " ..."))
	assert.Equal(t, "é", firstLine("éé", 3, ""), "a cut never splits a character")
	assert.NoError(t, RecipeYAMLError(nil))
}
