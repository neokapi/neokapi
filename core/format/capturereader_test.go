package format

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// onlyReader hides any io.ByteReader/io.Seeker the source implements, forcing
// encoding/xml to use its internal bufio path (the realistic streaming case).
type onlyReader struct{ r io.Reader }

func (o onlyReader) Read(p []byte) (int, error) { return o.r.Read(p) }

func TestCaptureReader_SliceMatchesSource(t *testing.T) {
	src := "the quick brown fox jumps over the lazy dog"
	cr := NewCaptureReader(onlyReader{strings.NewReader(src)})
	// Drain fully.
	all, err := io.ReadAll(cr)
	require.NoError(t, err)
	require.Equal(t, src, string(all))

	got, ok := cr.Slice(4, 9)
	require.True(t, ok)
	assert.Equal(t, "quick", string(got))

	got, ok = cr.Slice(0, int64(len(src)))
	require.True(t, ok)
	assert.Equal(t, src, string(got))

	// Out of range.
	_, ok = cr.Slice(0, int64(len(src)+1))
	assert.False(t, ok)
}

func TestCaptureReader_DiscardBoundsWindow(t *testing.T) {
	src := strings.Repeat("abcdefghij", 10000) // 100 KiB
	cr := NewCaptureReader(onlyReader{strings.NewReader(src)})

	buf := make([]byte, 512)
	var consumed int64
	maxWindow := 0
	for {
		n, err := cr.Read(buf)
		consumed += int64(n)
		// Emulate a walk that flushes everything behind the current position.
		cr.DiscardTo(consumed)
		if w := cr.Window(); w > maxWindow {
			maxWindow = w
		}
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
	}
	assert.Equal(t, int64(len(src)), consumed)
	// With discard tracking the read position, the window never exceeds one read.
	assert.LessOrEqual(t, maxWindow, 512, "window should stay bounded, not grow with the document")
	assert.Equal(t, int64(len(src)), cr.Base())
}

func TestCaptureReader_SliceValidAfterPartialDiscard(t *testing.T) {
	src := "0123456789ABCDEF"
	cr := NewCaptureReader(onlyReader{strings.NewReader(src)})
	_, err := io.ReadAll(cr)
	require.NoError(t, err)

	cr.DiscardTo(10) // drop "0123456789"
	// Bytes below the frontier are gone.
	_, ok := cr.Slice(9, 10)
	assert.False(t, ok)
	// Bytes at/after the frontier remain.
	got, ok := cr.Slice(10, 16)
	require.True(t, ok)
	assert.Equal(t, "ABCDEF", string(got))
}

// TestCaptureReader_XMLDecoderOffsetsResolve is the load-bearing contract: an
// xml.Decoder built over the CaptureReader yields InputOffset() values that
// Slice resolves to the exact raw source bytes (raw entities preserved), even
// with discarding as the walk advances.
func TestCaptureReader_XMLDecoderOffsetsResolve(t *testing.T) {
	doc := `<root><a>Hello &amp; world</a><b>second &lt;x&gt;</b><a>third</a></root>`
	cr := NewCaptureReader(onlyReader{strings.NewReader(doc)})
	dec := xml.NewDecoder(cr)

	var got []string
	var emitPos int64
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "a" {
			start := dec.InputOffset()
			// consume to matching end
			for {
				tk, err := dec.Token()
				require.NoError(t, err)
				if ee, ok := tk.(xml.EndElement); ok && ee.Name.Local == "a" {
					break
				}
			}
			end := dec.InputOffset() - int64(len("</a>"))
			raw, ok := cr.Slice(start, end)
			require.True(t, ok, "range [%d,%d) must be in window", start, end)
			got = append(got, string(raw))
			emitPos = end
			cr.DiscardTo(emitPos)
		}
	}
	// Raw slices preserve the source entity forms exactly.
	assert.Equal(t, []string{"Hello &amp; world", "third"}, got)
}
