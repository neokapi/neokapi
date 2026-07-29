package jsx

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/kbf"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fixtures under testdata/ are REAL catalogs taken from this repo's own
// dogfood pipeline — the thing that breaks first if the KBF reader/writer
// regresses:
//
//   - extractor-*.kbf.json   written by @neokapi/i18n-react `extract`
//     (`apps/kapi-desktop/frontend/i18n/**/*.kbf.json`), the source side of the
//     dogfood recipe's kapi-desktop collection.
//   - kapi-*.kbf.json        written by kapi itself (`i18n-nb/**/*.kbf.json`), the
//     target side the recipe recycles into and `neokapi-i18n compile` reads.
//
// Two different producers matter: the extractor emits fields kapi normalizes
// (`"placeholders": []` → `null`, its own generator stamp), so byte-identity is
// only the contract for catalogs kapi wrote. For the extractor's output the
// contract is that nothing is *lost* and that kapi's own output is then stable.
var roundTripFixtures = []string{
	"extractor-plain.kbf.json",
	"extractor-inline-codes.kbf.json",
	"kapi-translated.kbf.json",
	"kapi-translated-placeholders.kbf.json",
}

// readWriteKBF drives the real reader and writer over one .kbf.json payload and
// returns the emitted bytes — the exact path `kapi exec` / `kapi run` takes for
// a `.kbf.json` input and output.
func readWriteKBF(t *testing.T, name string, in []byte) []byte {
	t.Helper()
	r := NewReader()
	require.NoError(t, r.Open(context.Background(), &model.RawDocument{
		URI:    name,
		Reader: io.NopCloser(bytes.NewReader(in)),
	}))
	blocks := collectBlocks(t, r)
	require.NotEmpty(t, blocks, "%s: reader produced no blocks", name)

	w := NewWriter()
	var sink bytes.Buffer
	require.NoError(t, w.SetOutputWriter(&sink))
	ch := make(chan *model.Part, len(blocks))
	for _, b := range blocks {
		ch <- &model.Part{Type: model.PartBlock, Resource: b}
	}
	close(ch)
	require.NoError(t, w.Write(context.Background(), ch))
	require.NoError(t, w.Close())
	return sink.Bytes()
}

// A catalog kapi wrote must survive read → write byte-for-byte. This is the
// invariant the dogfood recipe leans on: re-running a flow over an already
// translated `i18n-<lang>/` tree must not rewrite the files, or every run
// churns git and invalidates the block cache.
func TestRoundTripKapiWrittenCatalogIsByteIdentical(t *testing.T) {
	for _, name := range []string{"kapi-translated.kbf.json", "kapi-translated-placeholders.kbf.json"} {
		t.Run(name, func(t *testing.T) {
			in, err := os.ReadFile(filepath.Join("testdata", name))
			require.NoError(t, err)
			out := readWriteKBF(t, name, in)
			assert.Equal(t, string(in), string(out),
				"a kapi-written .kbf.json must round-trip byte-for-byte")
		})
	}
}

// For every fixture — whoever wrote it — the second pass must be a fixed point:
// read → write → read → write produces identical bytes. Normalization is
// allowed once; drift on every pass is not.
func TestRoundTripIsIdempotent(t *testing.T) {
	for _, name := range roundTripFixtures {
		t.Run(name, func(t *testing.T) {
			in, err := os.ReadFile(filepath.Join("testdata", name))
			require.NoError(t, err)
			first := readWriteKBF(t, name, in)
			second := readWriteKBF(t, name, first)
			assert.Equal(t, string(first), string(second),
				"read → write must reach a fixed point after one pass")
		})
	}
}

// Nothing is dropped: every block keeps its id, hash, source text, targets,
// and translator-facing properties across the round trip.
func TestRoundTripPreservesEveryBlock(t *testing.T) {
	for _, name := range roundTripFixtures {
		t.Run(name, func(t *testing.T) {
			in, err := os.ReadFile(filepath.Join("testdata", name))
			require.NoError(t, err)
			want, err := kbf.Unmarshal(in)
			require.NoError(t, err)

			got, err := kbf.Unmarshal(readWriteKBF(t, name, in))
			require.NoError(t, err)

			wantBlocks := allBlocks(want)
			gotBlocks := allBlocks(got)
			require.Len(t, gotBlocks, len(wantBlocks), "block count must not change")

			for i := range wantBlocks {
				wb, gb := wantBlocks[i], gotBlocks[i]
				assert.Equal(t, wb.ID, gb.ID)
				assert.Equal(t, wb.Hash, gb.Hash, "%s: block hash is the catalog key — it must not move", wb.ID)
				assert.Equal(t, wb.Type, gb.Type, "%s: block type", wb.ID)
				assert.Equal(t, wb.Translatable, gb.Translatable, "%s: translatable", wb.ID)
				assert.Equal(t, wb.Source, gb.Source, "%s: source runs (inline codes + placeholders included)", wb.ID)
				assert.Equal(t, wb.Targets, gb.Targets, "%s: per-locale targets", wb.ID)
				assert.Equal(t, wb.Properties, gb.Properties, "%s: translator properties", wb.ID)
			}
		})
	}
}

// The extractor's own output carries the fields the dogfood pipeline depends on
// downstream (`neokapi-i18n compile` keys runtime dictionaries by block hash),
// so assert the fixtures really are the shapes the comment above claims.
func TestFixturesCoverBothProducers(t *testing.T) {
	extractor, err := kbf.Unmarshal(readFixture(t, "extractor-plain.kbf.json"))
	require.NoError(t, err)
	assert.Equal(t, "@neokapi/i18n-react", extractor.Generator.ID,
		"extractor-*.kbf.json must be i18n-react output")

	translated, err := kbf.Unmarshal(readFixture(t, "kapi-translated.kbf.json"))
	require.NoError(t, err)
	assert.Equal(t, "neokapi", translated.Generator.ID, "kapi-*.kbf.json must be kapi output")
	require.NotEmpty(t, allBlocks(translated))
	assert.NotEmpty(t, allBlocks(translated)[0].Targets, "kapi-*.kbf.json must carry targets")

	inline, err := kbf.Unmarshal(readFixture(t, "extractor-inline-codes.kbf.json"))
	require.NoError(t, err)
	var sawPaired bool
	for _, b := range allBlocks(inline) {
		for _, run := range b.Source {
			if run.PcOpen != nil || run.PcClose != nil {
				sawPaired = true
			}
		}
	}
	assert.True(t, sawPaired, "extractor-inline-codes.kbf.json must carry paired inline codes")

	placeholders, err := kbf.Unmarshal(readFixture(t, "kapi-translated-placeholders.kbf.json"))
	require.NoError(t, err)
	var sawPh bool
	for _, b := range allBlocks(placeholders) {
		for _, run := range b.Source {
			if run.Ph != nil {
				sawPh = true
			}
		}
	}
	assert.True(t, sawPh, "kapi-translated-placeholders.kbf.json must carry placeholder runs")
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return data
}

func allBlocks(f *kbf.File) []*kbf.Block {
	var out []*kbf.Block
	for i := range f.Documents {
		for j := range f.Documents[i].Blocks {
			out = append(out, &f.Documents[i].Blocks[j])
		}
	}
	return out
}
