package xcstrings_test

import (
	"testing"

	xcstrings "github.com/neokapi/neokapi/core/formats/xcstrings"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadInvalidCatalog feeds malformed input and asserts that Open/Read fail
// cleanly — surfacing an error on the result channel — rather than panicking.
func TestReadInvalidCatalog(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			// Not JSON at all.
			name:  "non-json blob",
			input: "this is not json {[<",
		},
		{
			// Valid JSON, but the top-level value is an array, not a catalog
			// object — parseCatalog expects an object at the top level.
			name:  "json array not object",
			input: `["a", "b", "c"]`,
		},
		{
			// Valid JSON object, but truncated mid-value (unterminated string).
			name:  "truncated object",
			input: `{"sourceLanguage": "en", "strings": {"key`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			reader := xcstrings.NewReader()
			// Open should succeed (it only validates the document/reader);
			// the parse error surfaces during Read.
			require.NotPanics(t, func() {
				err := reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
				require.NoError(t, err)
			})
			defer reader.Close()

			var foundError bool
			require.NotPanics(t, func() {
				for result := range reader.Read(ctx) {
					if result.Error != nil {
						foundError = true
					}
				}
			})
			assert.True(t, foundError, "expected a clean error for malformed catalog input")
		})
	}
}

// TestReadNilDocument verifies Open rejects a nil document without panicking.
func TestReadNilDocument(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := xcstrings.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

// TestReaderSignature asserts the reader advertises the .xcstrings extension and
// JSON MIME type for detection.
func TestReaderSignature(t *testing.T) {
	t.Parallel()
	reader := xcstrings.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.Extensions, ".xcstrings")
	assert.Contains(t, sig.MIMETypes, "application/json")
}

// TestReaderMetadata asserts the reader's name and display name.
func TestReaderMetadata(t *testing.T) {
	t.Parallel()
	reader := xcstrings.NewReader()
	assert.Equal(t, "xcstrings", reader.Name())
	assert.Equal(t, "Apple String Catalog", reader.DisplayName())
}
