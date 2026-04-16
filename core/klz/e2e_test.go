package klz

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/klf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEndToEnd_VerifyAnnotateOrphanCheckMerge exercises the issue
// #368 acceptance flow for Phase 1: round-trip a .klz through
// verify → annotate → orphan-check → merge.
func TestEndToEnd_VerifyAnnotateOrphanCheckMerge(t *testing.T) {
	// 1. Build a fresh archive.
	src := buildExampleArchive(t)
	r, err := NewReader(bytes.NewReader(src), int64(len(src)))
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })

	// 2. Verify: fresh archive must report no errors.
	require.Empty(t, r.VerifyAll(), "verify should pass on fresh archive")

	// 3. Annotate: load the example sidecar and resolve every
	// anchor against the archive's blocks.
	files, err := r.AnnotationFiles()
	require.NoError(t, err)
	require.Len(t, files, 1)

	blocks := make(map[string]*klf.Block)
	docs, err := r.Documents()
	require.NoError(t, err)
	for _, doc := range docs {
		for _, d := range doc.Documents {
			for i := range d.Blocks {
				blocks[d.Blocks[i].ID] = &d.Blocks[i]
			}
		}
	}
	for _, ann := range files[0].File.Annotations {
		b, ok := blocks[ann.Anchor.Block]
		require.Truef(t, ok, "annotation %q targets unknown block", ann.ID)
		res := klf.ResolveAnchor(b, ann.Anchor)
		require.Truef(t, res.OK, "annotation %q failed to resolve: %s", ann.ID, res.Err)
	}

	// 4. Orphan check: modify the archive by removing a block and
	// confirm the affected annotations get flagged as orphans.
	mutated := mutateDropBlock(t, r, "tag-chip")
	r2, err := NewReader(bytes.NewReader(mutated), int64(len(mutated)))
	require.NoError(t, err)
	t.Cleanup(func() { _ = r2.Close() })

	blocks2 := make(map[string]*klf.Block)
	docs2, err := r2.Documents()
	require.NoError(t, err)
	for _, doc := range docs2 {
		for _, d := range doc.Documents {
			for i := range d.Blocks {
				blocks2[d.Blocks[i].ID] = &d.Blocks[i]
			}
		}
	}
	files2, err := r2.AnnotationFiles()
	require.NoError(t, err)
	orphans := 0
	for _, ann := range files2[0].File.Annotations {
		b, ok := blocks2[ann.Anchor.Block]
		if !ok {
			orphans++
			continue
		}
		if err := klf.ValidateAnchor(b, ann); err != nil {
			orphans++
		}
	}
	assert.Equal(t, 1, orphans, "exactly one annotation should be orphaned by removing tag-chip")

	// 5. Merge: for Phase 1 the merge path delegates to the
	// extractor named in the manifest. With no registered extractor
	// we can at least verify the manifest carries the generator id
	// a kapi klz merge subcommand would look up.
	assert.Equal(t, "@neokapi/kapi-format-examples", r.Manifest().Generator.ID)
}

// mutateDropBlock rewrites the example archive with tag-chip
// removed. Used to force an orphaned annotation for the orphan-check
// step of the end-to-end flow.
func mutateDropBlock(t *testing.T, r *Reader, blockID string) []byte {
	t.Helper()

	w := NewWriter(WriterOptions{
		Generator: r.Manifest().Generator,
		Project:   r.Manifest().Project,
		Created:   r.Manifest().Created,
	})

	docs, err := r.Documents()
	require.NoError(t, err)
	for i, doc := range docs {
		for j := range doc.Documents {
			filtered := doc.Documents[j].Blocks[:0]
			for _, b := range doc.Documents[j].Blocks {
				if b.ID == blockID {
					continue
				}
				filtered = append(filtered, b)
			}
			doc.Documents[j].Blocks = filtered
		}
		require.NoError(t, w.AddDocument(r.Manifest().Parts[i].Path, doc, nil))
	}

	// Copy non-document parts (target, skeleton, annotation) verbatim.
	for _, p := range r.Manifest().Parts {
		if p.Role == RoleDocument {
			continue
		}
		data, err := r.ReadPart(p.Path)
		require.NoError(t, err)
		switch p.Role {
		case RoleTarget:
			require.NoError(t, w.addPart(p.Path, data, RoleTarget, p.Attributes))
		case RoleSkeleton:
			require.NoError(t, w.AddSkeleton(p.Path, data, p.Attributes))
		case RoleAnnotation:
			require.NoError(t, w.addPart(p.Path, data, RoleAnnotation, p.Attributes))
		case RoleVocabulary:
			require.NoError(t, w.AddVocabulary(p.Path, data, p.Attributes))
		case RoleAsset:
			require.NoError(t, w.AddAsset(p.Path, data, p.Attributes))
		}
	}

	var buf bytes.Buffer
	_, err = w.Write(&buf)
	require.NoError(t, err)
	return buf.Bytes()
}
