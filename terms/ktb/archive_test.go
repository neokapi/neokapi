package ktb

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchiveRoundTripLossless(t *testing.T) {
	concepts := richConcepts()
	data, err := MarshalArchive(FromConcepts(concepts))
	require.NoError(t, err)

	got, err := UnmarshalArchive(data)
	require.NoError(t, err)
	require.Equal(t, canonicalConcepts(concepts), got.Concepts, "concepts must round-trip through .ktz")
}

// The archive carries exactly one member, named termbase.ktb, whose bytes are
// the plain bundle serialization — so unzipping a .ktz by hand yields a valid
// .ktb.
func TestArchiveHoldsOneNamedBundleMember(t *testing.T) {
	concepts := richConcepts()
	bundle, err := Marshal(FromConcepts(concepts))
	require.NoError(t, err)
	data, err := MarshalArchive(FromConcepts(concepts))
	require.NoError(t, err)

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	require.Len(t, zr.File, 1)
	assert.Equal(t, ArchiveMember, zr.File[0].Name)
	assert.Equal(t, "termbase.ktb", zr.File[0].Name)

	rc, err := zr.File[0].Open()
	require.NoError(t, err)
	defer rc.Close()
	var member bytes.Buffer
	_, err = member.ReadFrom(rc)
	require.NoError(t, err)
	assert.Equal(t, string(bundle), member.String())
}

func TestArchiveIsDeterministic(t *testing.T) {
	concepts := richConcepts()
	a, err := MarshalArchive(FromConcepts(concepts))
	require.NoError(t, err)
	reversed := []terms.Concept{concepts[1], concepts[0]}
	b, err := MarshalArchive(FromConcepts(reversed))
	require.NoError(t, err)
	assert.Equal(t, a, b, "MarshalArchive must be order-independent and time-independent")
}

func TestArchiveRoundTripWithRelations(t *testing.T) {
	f := FromConcepts(richConcepts())
	f.Relations = richRelations()
	data, err := MarshalArchive(f)
	require.NoError(t, err)

	got, err := UnmarshalArchive(data)
	require.NoError(t, err)
	require.Equal(t, canonicalRelations(richRelations()), got.Relations, "relations must round-trip through .ktz")
}

func TestUnmarshalArchiveRejectsNonArchive(t *testing.T) {
	bundle, err := Marshal(FromConcepts(richConcepts()))
	require.NoError(t, err)
	_, err = UnmarshalArchive(bundle)
	require.Error(t, err, "a bare .ktb is not a .ktz")
	assert.Contains(t, err.Error(), "open archive")
}

func TestArchiveExtensionsAreDistinct(t *testing.T) {
	assert.Equal(t, ".ktb", Ext)
	assert.Equal(t, ".ktz", ArchiveExt)
}
