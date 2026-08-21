package client

import (
	"testing"

	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
)

// A pull carries what a write-out needs to place an asset, or the client has to
// ask for the same list again over the network.
//
// SourceID is the asset's durable name within its item — for a packaged format,
// the path inside the archive the variant replaces — and the write-out reads it
// to know WHERE the variant goes. The converter carried everything else about
// an asset and dropped this one, so the media a pull already delivered could
// not be used and was re-fetched per item, per locale.
func TestPulledMediaCarriesWhereTheAssetGoes(t *testing.T) {
	got := AssetToSyncMedia(&venue.Asset{
		ID:         "asset-1",
		ItemName:   "docs/guide.docx",
		SourceID:   "media:word/media/image1.png",
		BlobKey:    "b6f1c0de",
		MimeType:   "image/png",
		Filename:   "image1.png",
		SizeBytes:  2048,
		AltText:    "a diagram",
		Properties: map[string]string{"zipPath": "word/media/image1.png"},
	})

	assert.Equal(t, "media:word/media/image1.png", got.SourceID,
		"the write-out reads this to know where inside the package the variant goes")
	assert.Equal(t, "asset-1", got.ID)
	assert.Equal(t, "docs/guide.docx", got.ItemName)
	assert.Equal(t, "b6f1c0de", got.BlobKey)
	assert.Equal(t, "a diagram", got.AltText)
	assert.Equal(t, map[string]string{"zipPath": "word/media/image1.png"}, got.Properties)
}
