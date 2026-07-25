package openxml

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A caller with no reader to wire — the engine's Merge RPC receives a Part stream
// and the original document over gRPC — used to get its package copied back
// verbatim, with every text edit and every translation in the stream dropped and
// the write reported as a success (#1482).
//
// The writer declares RequiresSkeleton, so it genuinely cannot serialize
// WordprocessingML from the content model. What it CAN do is rebuild the skeleton
// it was not given: it holds the original package, and the skeleton is a
// deterministic function of those bytes and the extraction config. These tests
// pin both halves of that claim — the write carries content, and it is the same
// write the wired path performs.

// noSkeletonWrite writes parts with no skeleton store wired, which is the path
// under test.
func noSkeletonWrite(t *testing.T, original []byte, parts []*model.Part, locale model.LocaleID) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := NewWriter()
	w.SetOriginalContent(original)
	w.SetLocale(locale)
	require.NoError(t, w.SetOutputWriter(&buf))
	require.NoError(t, w.Write(t.Context(), testutil.PartsToChannel(parts)))
	require.NoError(t, w.Close())
	return append([]byte(nil), buf.Bytes()...)
}

// readParts reads original with no skeleton store (the caller has no reader in
// the real scenario; here it stands in for the client that produced the stream).
func readParts(t *testing.T, original []byte, uri string) []*model.Part {
	t.Helper()
	r := NewReader()
	require.NoError(t, r.Open(t.Context(), &model.RawDocument{
		URI:          uri,
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(original),
	}))
	parts := testutil.CollectParts(t, r.Read(t.Context()))
	require.NoError(t, r.Close())
	return parts
}

// zipEntries maps member name → uncompressed content.
func zipEntries(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	out := make(map[string][]byte, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(rc)
		rc.Close()
		require.NoError(t, err)
		out[f.Name] = content
	}
	return out
}

// The headline: a translation in the Part stream reaches the package even though
// the caller wired no skeleton store.
func TestNoSkeletonWriteCarriesTheTranslation(t *testing.T) {
	for _, name := range []string{"simple.docx", "formatted.docx"} {
		t.Run(name, func(t *testing.T) {
			original, err := os.ReadFile("testdata/" + name)
			require.NoError(t, err)

			parts := readParts(t, original, name)
			target := model.LocaleID("qps")
			translated := 0
			for _, p := range parts {
				if b, ok := p.Resource.(*model.Block); ok && b.Translatable && b.SourceText() != "" {
					b.SetTargetText(target, "["+b.SourceText()+"]")
					translated++
				}
			}
			require.Positive(t, translated, "the fixture produced no translatable block")

			out := noSkeletonWrite(t, original, parts, target)
			require.NotEmpty(t, out)

			// Read the result back: every block must carry the bracketed text.
			back := readParts(t, out, name)
			seen := 0
			for _, p := range back {
				if b, ok := p.Resource.(*model.Block); ok && b.Translatable && b.SourceText() != "" {
					assert.Contains(t, b.SourceText(), "[",
						"a translation in the Part stream was dropped by the no-skeleton write")
					seen++
				}
			}
			assert.Equal(t, translated, seen, "the block count changed across the write")
		})
	}
}

// The equivalence claim, member for member: deriving the skeleton produces the
// same package as being handed one. This is what makes the fix a completion of
// the write path rather than a second, lower-fidelity one.
func TestDerivedSkeletonMatchesTheWiredWrite(t *testing.T) {
	for _, name := range []string{"simple.docx", "formatted.docx"} {
		t.Run(name, func(t *testing.T) {
			original, err := os.ReadFile("testdata/" + name)
			require.NoError(t, err)

			wired := skeletonWriteOnce(t, original, name)
			derived := noSkeletonWrite(t, original, readParts(t, original, name), "")

			wantMembers := zipEntries(t, wired)
			gotMembers := zipEntries(t, derived)
			require.Len(t, gotMembers, len(wantMembers), "the member set differs")
			for member, want := range wantMembers {
				got, ok := gotMembers[member]
				require.True(t, ok, "member %s missing from the derived write", member)
				assert.Equal(t, string(want), string(got),
					"member %s differs between the wired and derived skeleton writes", member)
			}
			t.Logf("%s: %d members identical between the wired and derived writes", name, len(wantMembers))
		})
	}
}

// A stream that omits some of the package's blocks must not delete their text.
// The derived reparse keeps the block as read for any ref the caller did not
// send, so a partial stream reproduces the original for the parts it left out.
func TestNoSkeletonWriteKeepsBlocksTheStreamOmitted(t *testing.T) {
	original, err := os.ReadFile("testdata/formatted.docx")
	require.NoError(t, err)

	parts := readParts(t, original, "formatted.docx")
	var kept []*model.Part
	dropped := 0
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && b.Translatable && dropped == 0 {
			dropped++
			continue // omit exactly one block from the stream
		}
		kept = append(kept, p)
	}
	require.Equal(t, 1, dropped)

	out := noSkeletonWrite(t, original, kept, "")
	before := readParts(t, original, "formatted.docx")
	after := readParts(t, out, "formatted.docx")

	texts := func(ps []*model.Part) []string {
		var got []string
		for _, p := range ps {
			if b, ok := p.Resource.(*model.Block); ok {
				got = append(got, b.SourceText())
			}
		}
		return got
	}
	assert.Equal(t, texts(before), texts(after),
		"a block the caller's stream omitted must keep its original text, not vanish")
}
