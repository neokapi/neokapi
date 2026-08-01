package kpz

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/safeio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A .kpz is handed to an outside translator and comes back. These tests hold
// the two invariants that makes safe: a package may only NAME paths inside the
// project it is merged into, and parsing one is BOUNDED regardless of what the
// archive claims about itself.

// hostileArchive builds a .kpz whose manifest is written by hand, so a test can
// declare things Marshal would never produce. Members are stored uncompressed
// and their checksums computed for real, so the package fails only on the thing
// the test is about.
func hostileArchive(t *testing.T, manifest Manifest, members map[string][]byte) []byte {
	t.Helper()

	full := make([]Member, 0, len(manifest.Members))
	verify := make([]memberContent, 0, len(manifest.Members))
	for i, m := range manifest.Members {
		body, ok := members[m.Path]
		require.Truef(t, ok, "test declared member %q with no bytes", m.Path)
		if m.SHA256 == "" {
			s := sha256.Sum256(body)
			m.SHA256 = hex.EncodeToString(s[:])
		}
		manifest.Members[i] = m
		full = append(full, m)
		verify = append(verify, memberContent{Member: m})
	}
	if manifest.SchemaVersion == "" {
		manifest.SchemaVersion = SchemaVersion
	}
	if manifest.Kind == "" {
		manifest.Kind = KindProject
	}
	if manifest.RootHash == "" {
		manifest.RootHash = rootHash(verify)
	}
	manifest.Members = full

	data, err := json.MarshalIndent(manifest, "", "  ")
	require.NoError(t, err)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	write := func(name string, body []byte) {
		w, werr := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store, Modified: zipEpoch})
		require.NoError(t, werr)
		_, werr = w.Write(body)
		require.NoError(t, werr)
	}
	write(ManifestPath, data)
	for path, body := range members {
		write(path, body)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// TestUnmarshalRejectsTraversalSourcePath: a package names which of the
// project's sources it carries work for, and that name is joined onto the
// project root to re-read the file and to write the merged result. A name that
// climbs out reads and writes outside the project, so the package is refused
// rather than opened.
func TestUnmarshalRejectsTraversalSourcePath(t *testing.T) {
	body := []byte(`{"overlays":[]}`)

	for _, tc := range []struct {
		name     string
		mutate   func(*Manifest)
		wantPart string
	}{
		{
			name:     "source path climbs out",
			mutate:   func(m *Manifest) { m.Sources = []SourceIdentity{{SourcePath: "../../../../etc/passwd"}} },
			wantPart: "source path",
		},
		{
			name:     "source path is absolute",
			mutate:   func(m *Manifest) { m.Sources = []SourceIdentity{{SourcePath: "/etc/cron.d/kapi"}} },
			wantPart: "source path",
		},
		{
			name: "traversal hides mid-path",
			mutate: func(m *Manifest) {
				m.Sources = []SourceIdentity{{SourcePath: "src/nested/../../../../etc/passwd"}}
			},
			wantPart: "source path",
		},
		{
			name: "skeleton pointer climbs out",
			mutate: func(m *Manifest) {
				m.Sources = []SourceIdentity{{SourcePath: "src/en.json", SkeletonPath: "../../evil.bin"}}
			},
			wantPart: "skeleton path",
		},
		{
			name: "member path climbs out",
			mutate: func(m *Manifest) {
				m.Members = append(m.Members, Member{Path: "../../evil.json", ContentType: ContentTypeOverlays})
			},
			wantPart: "member path",
		},
		{
			name: "interchange task names a file outside",
			mutate: func(m *Manifest) {
				m.Kind = KindInterchange
				m.Task = &InterchangeTask{TargetLocale: "fr", SourceFiles: []string{"../../../etc/passwd"}}
			},
			wantPart: "interchange source file",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifest := Manifest{Members: []Member{{Path: OverlaysPath, ContentType: ContentTypeOverlays}}}
			tc.mutate(&manifest)
			members := map[string][]byte{OverlaysPath: body}
			if len(manifest.Members) > 1 {
				members[manifest.Members[len(manifest.Members)-1].Path] = body
			}

			_, err := Unmarshal(hostileArchive(t, manifest, members))
			require.Error(t, err, "a package naming a path outside the project must not open")
			assert.Contains(t, err.Error(), tc.wantPart)
			assert.Contains(t, err.Error(), "not a path inside the project")
		})
	}
}

// TestUnmarshalRejectsTraversalOverlaySource: an overlay's Source scopes it to
// one source/<name> member, so it is part of the same path surface.
func TestUnmarshalRejectsTraversalOverlaySource(t *testing.T) {
	overlays, err := marshalOverlaySet([]OverlayDoc{{
		Kind: "targets/fr", BlockHash: "b1", Payload: json.RawMessage(`{}`),
		Source: "../../../etc/passwd",
	}})
	require.NoError(t, err)

	manifest := Manifest{Members: []Member{{Path: OverlaysPath, ContentType: ContentTypeOverlays}}}
	_, err = Unmarshal(hostileArchive(t, manifest, map[string][]byte{OverlaysPath: overlays}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "overlay source")
	assert.Contains(t, err.Error(), "not a path inside the project")
}

// TestUnmarshalAcceptsOrdinaryPaths guards the other side of the rule: the
// paths a real `pack` produces — nested directories, the skeletons/ and source/
// prefixes — are ordinary relative paths and must keep working.
func TestUnmarshalAcceptsOrdinaryPaths(t *testing.T) {
	pkg := &Package{
		Kind:   KindProject,
		Blocks: []BlockDoc{{Path: "blocks/deep/nested/app.kbf.json", File: sampleBlocks()}},
		Sources: []SourceIdentity{{
			SourcePath:   "src/deep/nested/en.json",
			SkeletonPath: SkeletonDir + "src/deep/nested/en.json",
		}},
		Skeletons: []SkeletonDoc{{
			Path:       SkeletonDir + "src/deep/nested/en.json",
			SourcePath: "src/deep/nested/en.json",
			Content:    BytesContent([]byte("skel")),
		}},
		Source: []SourceDoc{{Path: SourceDir + "src/deep/nested/en.json", Content: BytesContent([]byte("{}"))}},
	}
	data, err := pkg.Marshal()
	require.NoError(t, err)

	got, err := Unmarshal(data)
	require.NoError(t, err, "an ordinary nested layout must round-trip")
	assert.Equal(t, "src/deep/nested/en.json", got.Sources[0].SourcePath)
	require.Len(t, got.Skeletons, 1)
	assert.Equal(t, "src/deep/nested/en.json", got.Skeletons[0].SourcePath)
}

// TestUnmarshalRejectsDecompressionBomb: `kapi merge` and `kapi info` open a
// package that arrived from outside, so a few-MB archive must not be able to
// expand into memory until the process dies. The declared-header check catches
// an honestly-labelled bomb before a byte is decompressed.
func TestUnmarshalRejectsDecompressionBomb(t *testing.T) {
	// 8 MiB of zeros deflates to a few KiB — a ratio far past the 100:1 bomb
	// threshold, and well above the 100 KiB grace size.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.CreateHeader(&zip.FileHeader{Name: ManifestPath, Method: zip.Deflate, Modified: zipEpoch})
	require.NoError(t, err)
	_, err = w.Write(make([]byte, 8<<20))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	assert.Less(t, buf.Len(), 1<<20, "the bomb itself must be small — that is the point")

	_, err = Unmarshal(buf.Bytes())
	require.Error(t, err, "a package that inflates 100:1 must be refused")
	assert.ErrorIs(t, err, safeio.ErrInflateRatio, "want a typed inflate-ratio breach")
}

// TestUnmarshalRejectsOversizedDeclaredPayload: a Zip64 header can claim a
// petabyte member. CheckReader reads the declared sizes, so the claim is
// refused up front rather than after an allocation attempt.
func TestUnmarshalRejectsOversizedDeclaredPayload(t *testing.T) {
	data := hostileArchive(t, Manifest{
		Members: []Member{{Path: OverlaysPath, ContentType: ContentTypeOverlays}},
	}, map[string][]byte{OverlaysPath: []byte(`{"overlays":[]}`)})

	// Rewrite the central-directory record's uncompressed size to something far
	// past the per-entry cap. Locating it by the end-of-central-directory
	// signature keeps this independent of member ordering.
	idx := bytes.LastIndex(data, []byte("PK\x01\x02"))
	require.Positive(t, idx, "central directory header not found")
	// Central-directory layout: signature(4) ... uncompressed size at offset 24.
	huge := uint32(0xFFFFFFF0)
	for i := range 4 {
		data[idx+24+i] = byte(huge >> (8 * i))
	}

	_, err := Unmarshal(data)
	require.Error(t, err, "a member declaring an absurd size must be refused")
	assert.True(t,
		errors.Is(err, safeio.ErrEntryTooLarge) ||
			errors.Is(err, safeio.ErrTotalTooLarge) ||
			errors.Is(err, safeio.ErrInflateRatio),
		"want a typed safeio limit breach, got %v", err)
}

// TestReadAllIsBounded: ReadAll is the one place a member is deliberately
// materialized (the skeleton handed to a reconstructing writer), so it carries
// its own cap rather than trusting the caller to have checked.
func TestReadAllIsBounded(t *testing.T) {
	oversized := BytesContent(bytes.Repeat([]byte("x"), int(MaxInlineMemberSize)+1))
	_, err := ReadAll(oversized)
	require.Error(t, err)
	require.ErrorIs(t, err, safeio.ErrByteBudget, "want a byte-budget breach")

	fits, err := ReadAll(BytesContent([]byte("skeleton")))
	require.NoError(t, err)
	assert.Equal(t, "skeleton", string(fits))
}

// TestPackageZipLimitsAdmitARealisticPackage: the caps exist to bound a hostile
// archive, not to reject an honest one. A media-heavy `pack --with-source` is a
// legitimate multi-gigabyte package, so the limits must sit above that.
func TestPackageZipLimitsAdmitARealisticPackage(t *testing.T) {
	assert.Greater(t, PackageZipLimits.MaxTotalSize, safeio.DefaultMaxTotalSize,
		"a .kpz carries whole source documents and media; the container default is too tight")
	assert.Greater(t, PackageZipLimits.MaxEntrySize, int64(1<<30),
		"one member is a whole document or media blob")
	assert.Equal(t, safeio.DefaultMinInflateRatio, PackageZipLimits.MinInflateRatio,
		"the ratio is what actually stops a bomb and must not be loosened")
}

// TestUnmarshalStreamsWholeAssetMembers: the members that can be huge (media,
// raw source, skeletons) are verified by streaming, never buffered — so their
// content is still checksummed, and a mismatch is still caught.
func TestUnmarshalStreamsWholeAssetMembers(t *testing.T) {
	pkg := &Package{
		Kind:  KindProject,
		Media: []Media{{Path: "media/logo.png", Content: BytesContent([]byte("PNGDATA"))}},
	}
	data, err := pkg.Marshal()
	require.NoError(t, err)

	got, err := Unmarshal(data)
	require.NoError(t, err)
	body, err := ReadAll(got.Media[0].Content)
	require.NoError(t, err)
	assert.Equal(t, "PNGDATA", string(body))

	// A member whose bytes do not match its declared digest is still caught:
	// streaming verifies exactly what buffering did.
	broken := hostileArchive(t, Manifest{
		Members: []Member{{
			Path: "media/logo.png", ContentType: ContentTypeMedia,
			SHA256: strings.Repeat("0", 64),
		}},
	}, map[string][]byte{"media/logo.png": []byte("PNGDATA")})

	_, err = Unmarshal(broken)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")
}
