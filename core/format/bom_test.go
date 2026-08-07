package format_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/stretchr/testify/require"
)

func TestSplitBOM(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantBOM string
		wantRes string
	}{
		{name: "absent", in: "key=value\n", wantRes: "key=value\n"},
		{name: "present", in: format.UTF8BOM + "key=value\n", wantBOM: format.UTF8BOM, wantRes: "key=value\n"},
		{name: "empty", in: ""},
		{name: "shorter_than_mark", in: "\xef", wantRes: "\xef"},
		{name: "prefix_only", in: "\xef\xbb", wantRes: "\xef\xbb"},
		{name: "doubled", in: format.UTF8BOM + format.UTF8BOM + "x", wantBOM: format.UTF8BOM, wantRes: format.UTF8BOM + "x"},
		{name: "interior_not_stripped", in: "a" + format.UTF8BOM + "b", wantRes: "a" + format.UTF8BOM + "b"},
		{name: "utf16_le_mark_untouched", in: "\xff\xfek\x00", wantRes: "\xff\xfek\x00"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bom, rest := format.SplitBOM([]byte(tc.in))
			require.Equal(t, tc.wantBOM, string(bom))
			require.Equal(t, tc.wantRes, string(rest))
			require.Equal(t, tc.in, string(bom)+string(rest), "the split must lose no bytes")
			require.Equal(t, tc.wantRes, string(format.TrimBOM([]byte(tc.in))))
			require.Equal(t, tc.wantRes, format.TrimBOMString(tc.in))
		})
	}
}

func TestSplitBOMReader(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantBOM string
	}{
		{name: "absent", in: "key=value\n"},
		{name: "present", in: format.UTF8BOM + "key=value\n", wantBOM: format.UTF8BOM},
		{name: "mark_only", in: format.UTF8BOM, wantBOM: format.UTF8BOM},
		{name: "empty", in: ""},
		{name: "shorter_than_mark", in: "ab"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bom, rest, err := format.SplitBOMReader(strings.NewReader(tc.in))
			require.NoError(t, err)
			require.Equal(t, tc.wantBOM, bom)
			body, err := io.ReadAll(rest)
			require.NoError(t, err)
			require.Equal(t, tc.in, bom+string(body), "the split must lose no bytes")
		})
	}
}

// TestSplitBOMReaderSurfacesReadError checks that a stream that fails before
// the mark can be decided reports the failure rather than silently reporting
// "no mark".
func TestSplitBOMReaderSurfacesReadError(t *testing.T) {
	_, _, err := format.SplitBOMReader(failingReader{})
	require.Error(t, err)
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errBoom }

var errBoom = bytes.ErrTooLarge
