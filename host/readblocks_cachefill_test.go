package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// templatesHTML has the shapes the HTML reader reads differently with a
// skeleton wired: a comment and whitespace between sections, a table written
// without a tbody, and a character reference.
const templatesHTML = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><title>Templates</title></head>
<body>

<!-- ORDER -->
<div id="order">
  <h1>Thank you for your order!</h1>
  <table>
    <tr><th>Item</th><th>Qty</th></tr>
  </table>
  <p>Hi <strong>{name}</strong> &amp; welcome.</p>
</div>

<!-- SHIPPING -->
<div id="shipping">
  <p>Your order is on its way.</p>
</div>
</body>
</html>
`

// guideMDX has JSX elements whose children are block-level markdown.
const guideMDX = `import { Callout } from "@site/components";

# Guide

<Callout type="note">

Keep the **recipe** beside the files it reads.

- Open the project folder.
- Check every file.

</Callout>
`

// TestReadSourceBlocks_ReadsAsTheCheck: the read that fills the block cache
// (project.ReadSourceBlocks, which `kapi up` and the desktop extract with)
// gives the blocks a check reads, with the same ids and texts.
func TestReadSourceBlocks_ReadsAsTheCheck(t *testing.T) {
	app := &App{}
	app.InitRegistries()
	t.Cleanup(app.Shutdown)
	dir := t.TempDir()
	for name, body := range map[string]string{"templates.html": templatesHTML, "guide.mdx": guideMDX} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name)
			require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
			format := filepath.Ext(name)[1:]

			read, _, err := project.ReadSourceBlocks(t.Context(), app.FormatReg, format, path, "en", "", nil)
			require.NoError(t, err)
			cached := map[string]string{}
			for _, b := range read {
				if b.Translatable {
					cached[b.ID] = model.RunsEditText(b.SourceRuns())
				}
			}

			checked, err := app.ReadBlocksForCheck(t.Context(), path, format, nil, "en")
			require.NoError(t, err)
			require.NotEmpty(t, checked)
			want := map[string]string{}
			for _, b := range checked {
				want[b.ID] = model.RunsEditText(b.SourceRuns())
			}
			assert.Equal(t, want, cached)
		})
	}
}
