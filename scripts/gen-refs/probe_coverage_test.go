package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The qps catalog is written by pseudo-translating the source document, so it
// holds an entry for every string the source carries. A reference key that
// falls back to English there is a key the generator writes under one name and
// the localizer asks for under another.
//
// Four formats and one option label lived in that gap. The generator keyed an
// option by fmt.Sprint of its value, which spells the empty string as itself,
// while the localizer keyed it by i18n.OptionKey, which falls back to the
// index. And epub, messageformat, odf and tsv reached the reference through
// their reader's config while the generator read the schema registry, which
// had never been given theirs.
func TestProbeLocaleResolvesEveryReferenceKey(t *testing.T) {
	root := repoRoot(t)
	english, err := readEnglishDatasets(filepath.Join(root, "packages/reference-data/data"))
	require.NoError(t, err)

	cats, err := loadLocaleCatalogs(
		filepath.Join(root, "core/i18n/catalogs"),
		filepath.Join(root, "host/i18n/catalogs"),
		ProbeLocale,
	)
	require.NoError(t, err)

	v, err := localizeAll(english, cats, filepath.Join(root, "scripts/gen-refs/nativedocs"), ProbeLocale)
	require.NoError(t, err)

	assert.Empty(t, keysOf(v.coverage),
		"every reference key must resolve in the probe catalog; run `make l10n-build` if the catalog is stale")
	assert.Positive(t, v.coverage.translated)
}

// A target locale is allowed to lag: that drift is the toil kapi absorbs, and a
// source change must never fail a build for it.
func TestTargetLocaleShortfallIsReported(t *testing.T) {
	var cov localeCoverage
	cov.translated = 3
	cov.missing = map[string]bool{"tools.segment.displayName": true}

	require.NoError(t, reportCoverage("nb", cov))
	require.Error(t, reportCoverage(ProbeLocale, cov))
	require.NoError(t, reportCoverage(ProbeLocale, localeCoverage{translated: 3}))
}

func keysOf(cov localeCoverage) []string {
	out := make([]string, 0, len(cov.missing))
	for k := range cov.missing {
		out = append(out, k)
	}
	return out
}

// repoRoot walks up from the test's working directory to the checkout root,
// named by the go.work file the workspace is coordinated by.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "no go.work above %s", dir)
		dir = parent
	}
}
