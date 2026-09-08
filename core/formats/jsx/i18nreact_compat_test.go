package jsx

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/kbf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testdata/i18n-react-1.2.3.klf is the literal output of
//
//	npm install -D @neokapi/i18n-react@1.2.3
//	neokapi-i18n extract --src "src/**/*.tsx" --out i18n --project kapimart
//
// over a page carrying a heading, a sentence with a variable and a paired
// inline code, and two translatable attributes. It is the file a reader who
// follows the install line in the docs has on disk, so it is the input
// `kapi pseudo-translate i18n/` has to accept.
const i18nReactFixture = "i18n-react-1.2.3.klf"

// The fixture is only worth anything while it is still the 1.2.3 shape. Assert
// the two markers that make it one, so a well-meaning reformat of testdata
// cannot turn this suite into a second test of the current format.
func TestI18nReactFixtureIsThePublishedShape(t *testing.T) {
	raw := readFixture(t, i18nReactFixture)

	assert.Equal(t, format.KBFExtI18nReact, filepath.Ext(i18nReactFixture),
		"the fixture must carry the suffix 1.2.3 writes")
	assert.Contains(t, string(raw), `"kind": "`+kbf.KindI18nReact+`"`,
		"the fixture must carry the root kind 1.2.3 stamps")

	file, err := kbf.Unmarshal(raw)
	require.NoError(t, err)
	assert.Equal(t, "@neokapi/i18n-react", file.Generator.ID)
	assert.Equal(t, "1.2.3", file.Generator.Version)
}

// A catalog 1.2.3 wrote is a current bundle in every byte but its root kind.
// Swapping that one string yields the file the current extractor writes for the
// same source, and both must read to the same blocks.
func TestI18nReactCatalogMatchesItsCurrentTwin(t *testing.T) {
	raw := readFixture(t, i18nReactFixture)
	twin := []byte(strings.Replace(string(raw),
		`"kind": "`+kbf.KindI18nReact+`"`, `"kind": "`+kbf.Kind+`"`, 1))
	require.NotEqual(t, string(raw), string(twin), "the swap must have applied")

	published, err := kbf.Unmarshal(raw)
	require.NoError(t, err)
	current, err := kbf.Unmarshal(twin)
	require.NoError(t, err)

	publishedBlocks := allBlocks(published)
	currentBlocks := allBlocks(current)
	require.NotEmpty(t, publishedBlocks)
	require.Len(t, publishedBlocks, len(currentBlocks))
	for i := range publishedBlocks {
		assert.Equal(t, currentBlocks[i], publishedBlocks[i])
	}
}

// Reading the published catalog and writing it back produces the current
// spelling: kapi takes the file as it is and hands back one every other kapi
// surface reads.
func TestI18nReactCatalogIsRestampedOnWrite(t *testing.T) {
	raw := readFixture(t, i18nReactFixture)
	out := readWriteKBF(t, i18nReactFixture, raw)

	written, err := kbf.Unmarshal(out)
	require.NoError(t, err)
	assert.Equal(t, kbf.Kind, written.Kind)
	assert.NotContains(t, string(out), kbf.KindI18nReact)

	require.Len(t, allBlocks(written), len(allBlocks(mustUnmarshal(t, raw))),
		"no block may be lost on the way through")
}

// The same bytes, read a second time, are a fixed point: the restamped catalog
// is what the next run of any flow over the tree sees.
func TestI18nReactCatalogReachesAFixedPoint(t *testing.T) {
	raw := readFixture(t, i18nReactFixture)
	first := readWriteKBF(t, i18nReactFixture, raw)
	second := readWriteKBF(t, i18nReactFixture, first)
	assert.Equal(t, string(first), string(second))
}

// Detection has to agree with the reader, by suffix and by content. A tree
// written by 1.2.3 reaches kapi as a directory of .klf files with no --format
// flag in sight, so both routes are the ones a reader actually takes.
func TestI18nReactCatalogIsDetected(t *testing.T) {
	sig := NewReader().Signature()
	assert.Contains(t, sig.Extensions, format.KBFExtI18nReact)
	assert.Contains(t, sig.Extensions, format.KBFExt,
		"the current suffix stays first-class")

	require.NotNil(t, sig.Sniff)
	assert.True(t, sig.Sniff(readFixture(t, i18nReactFixture)))
	assert.True(t, sig.Sniff(readFixture(t, "extractor-plain.kbf.json")))
	assert.False(t, sig.Sniff([]byte(`{"kind":"something-else","documents":[]}`)))
}

// A JSON blob from another producer is still refused, and the diagnostic names
// the kind this build writes rather than listing what it will tolerate.
func TestUnknownKindIsStillRefused(t *testing.T) {
	_, err := kbf.Unmarshal([]byte(`{"schemaVersion":"1.0","kind":"xliff","documents":[]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unexpected kind "xliff"`)
	assert.Contains(t, err.Error(), kbf.Kind)
}

func mustUnmarshal(t *testing.T, data []byte) *kbf.File {
	t.Helper()
	f, err := kbf.Unmarshal(data)
	require.NoError(t, err)
	return f
}

// Guard against a fixture that stops being byte-exact: os.ReadFile through
// readFixture is the only path used above, and this keeps the file from
// growing a BOM or CRLF on some editor's round trip.
func TestI18nReactFixtureIsUnadornedUTF8(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", i18nReactFixture))
	require.NoError(t, err)
	assert.False(t, bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}), "no BOM")
	assert.NotContains(t, string(raw), "\r\n", "no CRLF")
}
