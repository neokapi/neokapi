//go:build unix

package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/rootpath"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// linkedProject makes a project directory holding locales/fr.json, a link
// to a file outside it, and returns the root and the link's path.
func linkedProject(t *testing.T) (root, link string) {
	t.Helper()
	root = t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.json")
	require.NoError(t, os.WriteFile(outside, []byte(`{"a":"secret"}`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales"), 0o755))
	link = filepath.Join(root, "locales", "fr.json")
	require.NoError(t, os.Symlink(outside, link))
	return root, link
}

func requireEscape(t *testing.T, err error) {
	t.Helper()
	var esc *rootpath.EscapeError
	require.ErrorAs(t, err, &esc, "want a refusal for a link out of the project")
}

// TestStatIn_RefusesLinkOut asserts statIn and readDirIn refuse a path
// reached through a link out of the project, and read a path beneath no
// project as named.
func TestStatIn_RefusesLinkOut(t *testing.T) {
	root, link := linkedProject(t)
	_, err := statIn(root, link)
	requireEscape(t, err)

	outsideDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outsideDir, "en.json"), []byte(`{}`), 0o644))
	require.NoError(t, os.Symlink(outsideDir, filepath.Join(root, "strings")))
	_, err = readDirIn(root, filepath.Join(root, "strings"))
	requireEscape(t, err)

	_, err = statIn("", link)
	require.NoError(t, err, "without a project root the path is a person's, stated as named")
	entries, err := readDirIn("", outsideDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

// TestPathLanguage_IgnoresDirectoryLinkedOut asserts the language a path
// names is not read from a directory that is a link out of the project.
func TestPathLanguage_IgnoresDirectoryLinkedOut(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "strings"), 0o755))
	write := func(dir string) {
		for _, n := range []string{"en.json", "nb.json"} {
			require.NoError(t, os.WriteFile(filepath.Join(dir, n), []byte(`{}`), 0o644))
		}
	}
	write(filepath.Join(root, "strings"))
	assert.Equal(t, "nb", string(pathLanguage(root, "strings/nb.json", filepath.Join(root, "strings", "nb.json"))))

	outside := t.TempDir()
	write(outside)
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "linked")))
	assert.Empty(t, pathLanguage(root, "linked/nb.json", filepath.Join(root, "linked", "nb.json")),
		"the siblings of a file in a directory linked out of the project are not read")
}

// TestPreserveAssetVariantIn_LinkOutIsNoVariant asserts a target variant
// that is a link out of the project reads as no variant.
func TestPreserveAssetVariantIn_LinkOutIsNoVariant(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "logo.png")
	require.NoError(t, os.WriteFile(src, []byte("src"), 0o644))
	outside := filepath.Join(t.TempDir(), "logo_fr.png")
	require.NoError(t, os.WriteFile(outside, []byte("fr"), 0o644))
	variant := filepath.Join(root, "logo_fr.png")
	require.NoError(t, os.Symlink(outside, variant))
	assert.False(t, preserveAssetVariantIn(root, "image", src, variant))

	require.NoError(t, os.Remove(variant))
	require.NoError(t, os.WriteFile(variant, []byte("fr"), 0o644))
	assert.True(t, preserveAssetVariantIn(root, "image", src, variant), "a variant inside the project is kept")
}

// TestFilePreviewIn_LinkOutPreviewsEmpty asserts a trace previews nothing of
// a file reached through a link out of the project.
func TestFilePreviewIn_LinkOutPreviewsEmpty(t *testing.T) {
	root, link := linkedProject(t)
	assert.Empty(t, filePreviewIn(root, link))
	assert.Contains(t, filePreviewIn("", link), "secret", "outside a project the file a person named is read as named")
}

// TestReadCommentLayerIn_RefusesLinkOut asserts a comment layer is not read
// from a source file that is a link out of the project.
func TestReadCommentLayerIn_RefusesLinkOut(t *testing.T) {
	root, link := linkedProject(t)
	called := false
	_, err := (&App{}).readCommentLayerIn(context.Background(), root, link, nil, func([]byte) (*commentLayer, error) {
		called = true
		return &commentLayer{}, nil
	})
	requireEscape(t, err)
	assert.False(t, called, "nothing outside the project reaches the comment locator")
}

// TestProjectCheck_RefusesTargetLinkedOut plants a target file that is a link
// out of the project and asserts the project check refuses it rather than
// checking it as a translation.
func TestProjectCheck_RefusesTargetLinkedOut(t *testing.T) {
	root, link := linkedProject(t)
	src := filepath.Join(root, "locales", "en.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"a":"hello"}`), 0o644))
	p := &projectPass{
		targets: map[string][]ProjectCheckTargetRef{src: {{Locale: "fr", Path: link}}},
		result:  &ProjectCheck{},
		root:    root,
	}
	_, err := p.checkFile(context.Background(), nil, nil, src, nil, nil, nil, checkRunOptions{})
	requireEscape(t, err)
	for _, f := range p.result.Files {
		assert.Empty(t, f.Targets, "the linked target is not reported as translated")
	}
}
