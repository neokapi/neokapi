//go:build unix

package image

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/vision"
)

// TestReadDoesNotFollowAPathSwap opens an image, then replaces it at its name
// with a symbolic link to another image outside the project. The reader works
// from the descriptor the caller opened, so it still sees the original bytes:
// the dimensions and the metadata of the first image, nothing of the second.
func TestReadDoesNotFollowAPathSwap(t *testing.T) {
	vision.ResetForTest()
	defer vision.ResetForTest()

	project, outside := t.TempDir(), t.TempDir()
	in := filepath.Join(project, "hero.png")
	require.NoError(t, os.WriteFile(in, insertAfterIHDR(makePNG(t, 10, 10), tEXt("Title", "Original")), 0o644))
	secret := filepath.Join(outside, "secret.png")
	require.NoError(t, os.WriteFile(secret, insertAfterIHDR(makePNG(t, 33, 7), tEXt("Title", "Secret")), 0o644))

	f, err := os.Open(in)
	require.NoError(t, err)
	defer f.Close()
	info, err := f.Stat()
	require.NoError(t, err)

	// The swap, after the caller opened the file.
	require.NoError(t, os.Remove(in))
	require.NoError(t, os.Symlink(secret, in))

	r := NewReader()
	doc := &model.RawDocument{URI: in, Reader: f, ReaderAt: f, Size: info.Size()}
	require.NoError(t, r.Open(context.Background(), doc))
	var media *model.Media
	var texts []string
	for res := range r.Read(context.Background()) {
		require.NoError(t, res.Error)
		switch v := res.Part.Resource.(type) {
		case *model.Media:
			media = v
		case *model.Block:
			texts = append(texts, v.SourceText())
		}
	}
	require.NotNil(t, media)
	require.Equal(t, "10", media.Properties["width"])
	require.Equal(t, "10", media.Properties["height"])
	require.Contains(t, texts, "Original")
	require.NotContains(t, texts, "Secret")
}
