package host

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/registry"
)

// A failed conversion never deletes a file the user already had at the
// output path. The json writer creates its output at SetOutput, so the file
// may be left empty, but it is still there. The kbf writer only records the
// path and writes at Close, so a conversion that fails before Close leaves
// the file's bytes exactly as they were.
func TestConvFailureNeverDeletesAnExistingOutput(t *testing.T) {
	for _, tc := range []struct {
		format, ext string
		intact      bool
	}{
		{"json", ".json", false},
		{"kbf", ".kbf.json", true},
	} {
		t.Run(tc.format, func(t *testing.T) {
			app := newToolboxApp(t)
			app.FormatFlag = tc.format
			dir := t.TempDir()
			in := writeToolboxFile(t, dir, "bad"+tc.ext, "{ not valid")
			out := writeToolboxFile(t, dir, "keep"+tc.ext, "USER DATA")

			require.Error(t, app.convertDocument(context.Background(), in, registry.FormatID(tc.format), "", out))
			got, err := os.ReadFile(out)
			require.NoError(t, err, "the existing output must still exist after the failed conversion")
			if tc.intact {
				assert.Equal(t, "USER DATA", string(got))
			}
		})
	}
}
