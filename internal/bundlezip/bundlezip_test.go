package bundlezip

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoundTrip(t *testing.T) {
	payload := []byte("{\n  \"kind\": \"kapi-memory\"\n}\n")
	data, err := Marshal("tm.kmb", payload)
	require.NoError(t, err)

	got, err := Unmarshal(data, "tm.kmb")
	require.NoError(t, err)
	assert.Equal(t, payload, got)
}

func TestMarshalIsTimeIndependent(t *testing.T) {
	a, err := Marshal("tm.kmb", []byte("payload"))
	require.NoError(t, err)
	b, err := Marshal("tm.kmb", []byte("payload"))
	require.NoError(t, err)
	assert.Equal(t, a, b, "the fixed epoch modtime must make the bytes reproducible")
}

func TestMarshalRequiresMemberName(t *testing.T) {
	_, err := Marshal("", []byte("payload"))
	require.Error(t, err)
}

// A hand-zipped bundle keeps working even when whoever zipped it named the
// member something else, as long as it is the only file in the archive.
func TestUnmarshalFallsBackToTheSoleMember(t *testing.T) {
	data, err := Marshal("my-memory.kmb", []byte("payload"))
	require.NoError(t, err)

	got, err := Unmarshal(data, "tm.kmb")
	require.NoError(t, err)
	assert.Equal(t, []byte("payload"), got)
}

func TestUnmarshalRejectsAmbiguousArchive(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{"a.kmb", "b.kmb"} {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte("payload"))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())

	_, err := Unmarshal(buf.Bytes(), "tm.kmb")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "found 2 files")
}

func TestUnmarshalRejectsEmptyArchive(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	require.NoError(t, zw.Close())

	_, err := Unmarshal(buf.Bytes(), "tm.kmb")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no other file")
}

func TestUnmarshalRejectsNonZip(t *testing.T) {
	_, err := Unmarshal([]byte("{\"kind\":\"kapi-memory\"}"), "tm.kmb")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open archive")
}
