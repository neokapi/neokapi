package kmb

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/sievepen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchiveRoundTripLossless(t *testing.T) {
	entries, sessions := richEntries()
	data, err := MarshalArchive(FromModel(entries, sessions))
	require.NoError(t, err)

	got, err := UnmarshalArchive(data)
	require.NoError(t, err)
	require.Equal(t, entries, got.ModelEntries(), "TM entries must round-trip through .kmz")
	require.Equal(t, sessions, got.ModelImportSessions(), "import sessions must round-trip through .kmz")
}

// The archive carries exactly one member, named tm.kmb, whose bytes are the
// plain bundle serialization — so unzipping a .kmz by hand yields a valid .kmb.
func TestArchiveHoldsOneNamedBundleMember(t *testing.T) {
	entries, sessions := richEntries()
	bundle, err := Marshal(FromModel(entries, sessions))
	require.NoError(t, err)
	data, err := MarshalArchive(FromModel(entries, sessions))
	require.NoError(t, err)

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	require.Len(t, zr.File, 1)
	assert.Equal(t, ArchiveMember, zr.File[0].Name)
	assert.Equal(t, "tm.kmb", zr.File[0].Name)

	rc, err := zr.File[0].Open()
	require.NoError(t, err)
	defer rc.Close()
	var member bytes.Buffer
	_, err = member.ReadFrom(rc)
	require.NoError(t, err)
	assert.Equal(t, string(bundle), member.String())
}

func TestArchiveIsDeterministic(t *testing.T) {
	entries, sessions := richEntries()
	a, err := MarshalArchive(FromModel(entries, sessions))
	require.NoError(t, err)
	reversed := []sievepen.TMEntry{entries[1], entries[0]}
	b, err := MarshalArchive(FromModel(reversed, sessions))
	require.NoError(t, err)
	assert.Equal(t, a, b, "MarshalArchive must be order-independent and time-independent")
}

func TestArchiveCompressesTheBundle(t *testing.T) {
	entries, sessions := richEntries()
	bundle, err := Marshal(FromModel(entries, sessions))
	require.NoError(t, err)
	archive, err := MarshalArchive(FromModel(entries, sessions))
	require.NoError(t, err)
	assert.Less(t, len(archive), len(bundle),
		"the archive exists for compression: it must be smaller than the bundle it wraps")
}

func TestUnmarshalArchiveRejectsNonArchive(t *testing.T) {
	entries, sessions := richEntries()
	bundle, err := Marshal(FromModel(entries, sessions))
	require.NoError(t, err)
	_, err = UnmarshalArchive(bundle)
	require.Error(t, err, "a bare .kmb is not a .kmz")
	assert.Contains(t, err.Error(), "open archive")
}

func TestArchiveExtensionsAreDistinct(t *testing.T) {
	assert.Equal(t, ".kmb", Ext)
	assert.Equal(t, ".kmz", ArchiveExt)
}
