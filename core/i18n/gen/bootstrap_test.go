package gen

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The two generators write what core/i18n embeds: `go run ./core/i18n/gen/cmd`
// writes the builtins document, and `go run ./core/i18n/gen/catalogs` compiles
// catalogs/<locale>.mo, which core/i18n embeds with a pattern that fails the
// build when it matches nothing. So a generator that reaches core/i18n cannot
// be built on a checkout whose catalogs directory is empty, which is every
// checkout before `make i18n-catalogs` has run.
//
// Nothing about that is visible where the import is written. It surfaced as
// `pattern catalogs/*.mo: no matching files found` from a Makefile target that
// had run for a year, on every job that builds Go: the wasm build, the tests,
// the linters, the tidy check.
func TestGeneratorsDoNotImportTheirOutput(t *testing.T) {
	for _, pkg := range []string{"./cmd", "./catalogs"} {
		t.Run(pkg, func(t *testing.T) {
			out, err := exec.Command("go", "list", "-deps", pkg).Output()
			require.NoError(t, err, "go list -deps %s", pkg)
			for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
				assert.NotEqual(t, "github.com/neokapi/neokapi/core/i18n", strings.TrimSpace(line),
					"%s reaches core/i18n, which embeds the catalogs this generator writes; "+
						"put the shared code somewhere neither embeds (core/schema holds OptionKey "+
						"for exactly this reason)", pkg)
			}
		})
	}
}
