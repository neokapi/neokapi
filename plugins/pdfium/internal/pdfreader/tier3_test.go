package pdfreader

import (
	"image/png"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// TestReadParts_Tier3 verifies the tier-3 mode: each page is rendered to a real
// PNG raster (emitted as a Media part the host's vision pass consumes) alongside
// the page's raw positioned blocks — and the plugin applies NO structure of its
// own (no table groups), leaving that to the host.
func TestReadParts_Tier3(t *testing.T) {
	data, err := os.ReadFile("testdata/multi.pdf")
	require.NoError(t, err)

	parts, err := ReadParts(data, model.LocaleEnglish, "multi.pdf", Options{Tier3: true})
	require.NoError(t, err)

	var rasters []*model.Media
	var blocks, groups int
	for _, p := range parts {
		switch p.Type {
		case model.PartMedia:
			m := p.Resource.(*model.Media)
			if m.Properties[VisionRasterProperty] == "page" {
				rasters = append(rasters, m)
			}
		case model.PartBlock:
			blocks++
		case model.PartGroupStart:
			groups++
		}
	}

	require.NotEmpty(t, rasters, "tier-3 should emit a page raster Media")
	assert.Positive(t, blocks, "tier-3 should emit raw positioned blocks")
	assert.Zero(t, groups, "tier-3 plugin output must not pre-structure (no groups); host does tier-3")

	// The raster is a real PNG on disk with the advertised dimensions. Clean up
	// the temp files the renderer created (the host normally does this).
	for _, m := range rasters {
		assert.Equal(t, "image/png", m.MimeType)
		f, err := os.Open(m.URI)
		require.NoError(t, err, "raster file should exist at %s", m.URI)
		cfg, derr := png.DecodeConfig(f)
		_ = f.Close()
		require.NoError(t, derr, "raster should be a valid PNG")
		assert.Equal(t, m.Properties["width"], strconv.Itoa(cfg.Width))
		assert.Equal(t, m.Properties["height"], strconv.Itoa(cfg.Height))
		assert.Positive(t, cfg.Width)
		_ = os.Remove(m.URI)
	}
}
