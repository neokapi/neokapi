//go:build parity

package formats

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParityKnowledgeFromSpecYAML is the migration guard for #852: it proves
// the per-format parity knowledge (bridge filter class, tikal corner, parity
// skips) now lives in spec.yaml and resolves onto every formatSpecs row,
// WITHOUT needing a live okapi-bridge daemon. It runs as a plain unit test so
// the migration is verifiable on a developer machine (the full head-to-head
// run still needs `make parity-sandbox`).
func TestParityKnowledgeFromSpecYAML(t *testing.T) {
	// Load-all probe: every core/formats/*/spec.yaml must load + validate.
	idx, err := loadSpecIndex()
	require.NoError(t, err, "every core/formats/*/spec.yaml must load and validate")
	require.GreaterOrEqual(t, len(idx), 40, "spec index should cover the format corpus")

	// Every formatSpecs row resolves cleanly and yields a non-empty bridge
	// filter class (the dispatch key sent to BridgeService.Process).
	for _, fs := range formatSpecs {
		resolved, err := resolveParity(fs)
		require.NoErrorf(t, err, "resolveParity(%s)", fs.ID)
		assert.NotEmptyf(t, bridgeClass(resolved), "%s: bridge class", fs.ID)
	}

	// Value-preservation: the knowledge migrated out of the Go table now
	// arrives via spec.yaml exactly as before.
	t.Run("tikal_corner", func(t *testing.T) {
		cases := map[string]struct{ ext, cfg string }{
			"okf_properties": {".properties", "okf_properties"},
			"okf_po":         {".po", "okf_po"},
			"okf_plaintext":  {".txt", "okf_plaintext"},
		}
		for id, want := range cases {
			got, err := resolveParity(FormatSpec{ID: id})
			require.NoError(t, err)
			assert.Equalf(t, want.ext, got.TikalExt, "%s tikal ext", id)
			assert.Equalf(t, want.cfg, got.TikalConfig, "%s tikal config", id)
			assert.NotNilf(t, got.NewWriter, "%s writer from parityWriters registry", id)
		}
	})

	t.Run("po_roundtrip_and_tikal_skips", func(t *testing.T) {
		got, err := resolveParity(FormatSpec{ID: "okf_po"})
		require.NoError(t, err)
		assert.NotEmpty(t, got.SkipRoundTrip, "po round-trip skip")
		assert.NotEmpty(t, got.SkipTikal, "po tikal skip")
	})

	t.Run("filter_level_skips", func(t *testing.T) {
		// Formats whose spec.yaml carries a parity.skip — the divergence-453
		// readers and the binary-corpus filters with a native Go port. okf_pdf
		// is NOT here: PDF lost its in-core reader (now the kapi-pdfium plugin),
		// so it has no spec.yaml and keeps an inline SkipBinary on its
		// formatSpecs row, like the other bridge-only rows (okf_odf, okf_archive).
		for _, id := range []string{
			"okf_phpcontent", "okf_doxygen", "okf_tex", "okf_transtable",
			"okf_commaseparatedvalues", "okf_fixedwidthcolumns", "okf_ttx",
			"okf_txml", "okf_vignette", "okf_ttml",
			"okf_idml", "okf_icml", "okf_openxml", "okf_openoffice",
			"okf_mif", "okf_rtf",
		} {
			got, err := resolveParity(FormatSpec{ID: id})
			require.NoError(t, err)
			assert.NotEmptyf(t, got.Skip, "%s: parity.skip should resolve from spec.yaml", id)
		}
	})

	t.Run("residual_rows_keep_inline_fields", func(t *testing.T) {
		// The okf_xml/okf_dita config-preset rows have no spec.yaml (xml's
		// spec.yaml is okf_xmlstream); their inline bridge class survives.
		got, err := resolveParity(FormatSpec{ID: "okf_dita", BridgeFilterClass: "okf_xmlstream", ConfigID: "okf_xmlstream-dita"})
		require.NoError(t, err)
		assert.Equal(t, "okf_xmlstream", got.BridgeFilterClass)
		assert.Equal(t, "okf_xmlstream-dita", got.ConfigID)
	})
}
