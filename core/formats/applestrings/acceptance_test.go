//go:build acceptance

package applestrings_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAcceptancePlutilLint writes a translated .strings / .stringsdict file and
// runs the REAL Apple consumer validator, `plutil -lint`, against it. An
// old-style .strings table and a .stringsdict plural plist are both property
// lists that plutil validates. If plutil rejects kapi's output, that is a
// spec-compliance bug in the writer. The test skips gracefully when plutil is
// not present (non-macOS hosts).
func TestAcceptancePlutilLint(t *testing.T) {
	plutilPath, ok := lookTool("plutil")
	if !ok {
		t.Skip("plutil not available")
	}

	type fixture struct {
		name string // file under testdata/ or testdata/corpus/
		ext  string // output extension (drives kind detection on write/read)
	}
	fixtures := []fixture{
		{filepath.Join("testdata", "Localizable.strings"), ".strings"},
		{filepath.Join("testdata", "Localizable.stringsdict"), ".stringsdict"},
	}
	for _, pat := range []string{"*.strings", "*.stringsdict"} {
		m, _ := filepath.Glob(filepath.Join("testdata", "corpus", pat))
		for _, p := range m {
			fixtures = append(fixtures, fixture{p, filepath.Ext(p)})
		}
	}

	for _, fx := range fixtures {
		t.Run(filepath.Base(fx.name), func(t *testing.T) {
			out := translateAppleToFr(t, fx.name)

			dir := t.TempDir()
			outPath := filepath.Join(dir, "translated"+fx.ext)
			require.NoError(t, os.WriteFile(outPath, out, 0o644))

			cmd := exec.CommandContext(t.Context(), plutilPath, "-lint", outPath)
			combined, err := cmd.CombinedOutput()
			assert.NoErrorf(t, err, "plutil -lint must accept kapi's %s output: %s",
				fx.ext, combined)
		})
	}
}

// translateAppleToFr reads a .strings/.stringsdict file, sets a synthetic "fr"
// translation on every leaf (preserving placeholders), and returns the writer
// output for locale "fr".
func translateAppleToFr(t *testing.T, path string) []byte {
	t.Helper()
	parts, _ := readParts(t, path)
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		var nr []model.Run
		for _, r := range b.SourceRuns() {
			if r.Text != nil {
				nr = append(nr, model.Run{Text: &model.TextRun{Text: "T:" + r.Text.Text}})
			} else {
				nr = append(nr, r)
			}
		}
		b.SetTargetRuns("fr", nr)
	}
	return writeParts(t, parts, "fr")
}
