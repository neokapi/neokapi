package container_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type entry struct {
	name string
	data []byte
}

func makeZip(t *testing.T, entries []entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		require.NoError(t, err)
		_, err = w.Write(e.data)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func makeTar(t *testing.T, entries []entry, gzipped bool) []byte {
	t.Helper()
	var buf bytes.Buffer
	var out io.Writer = &buf
	var gz *gzip.Writer
	if gzipped {
		gz = gzip.NewWriter(&buf)
		out = gz
	}
	tw := tar.NewWriter(out)
	for _, e := range entries {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: e.name, Mode: 0o644, Size: int64(len(e.data))}))
		_, err := tw.Write(e.data)
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	if gz != nil {
		require.NoError(t, gz.Close())
	}
	return buf.Bytes()
}

func names(entries []container.Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name
	}
	return out
}

func TestIsContainerPath(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"a.zip", true}, {"a.tar", true}, {"a.tgz", true}, {"a.tar.gz", true},
		{"A.ZIP", true}, {"deep/b.tar.gz", true},
		{"a.json", false}, {"a.docx", false}, {"a.gz", false}, {"a", false},
	} {
		assert.Equalf(t, tc.want, container.IsContainerPath(tc.path), "path %q", tc.path)
	}
}

func TestDetect(t *testing.T) {
	assert.Equal(t, container.KindZip, container.Detect(makeZip(t, []entry{{"a", []byte("x")}})))
	assert.Equal(t, container.KindTar, container.Detect(makeTar(t, []entry{{"a", []byte("x")}}, false)))
	assert.Equal(t, container.KindTarGz, container.Detect(makeTar(t, []entry{{"a", []byte("x")}}, true)))
	assert.Equal(t, container.KindUnknown, container.Detect([]byte("not an archive")))
}

func TestEnumerateZip(t *testing.T) {
	data := makeZip(t, []entry{
		{"a/en.json", []byte(`{"k":"v"}`)},
		{"b/readme.md", []byte("# hi")},
	})
	kind, entries, err := container.Enumerate(data)
	require.NoError(t, err)
	assert.Equal(t, container.KindZip, kind)
	assert.Equal(t, []string{"a/en.json", "b/readme.md"}, names(entries))
	assert.Equal(t, []byte(`{"k":"v"}`), entries[0].Data)
}

func TestEnumerateUnknownErrors(t *testing.T) {
	_, _, err := container.Enumerate([]byte("garbage bytes that are not an archive"))
	require.Error(t, err)
}

// TestRepackReplacesAndPreserves is the barrier-sink fidelity guarantee: a
// replaced entry carries new bytes, every other member is byte-for-byte intact.
func TestRepackZip(t *testing.T) {
	jsonSrc := []byte(`{"greeting":"Hello"}`)
	pngSrc := []byte("\x89PNG\r\n\x1a\nbinary-1234")
	binSrc := []byte("\x00\x01opaque")
	data := makeZip(t, []entry{
		{"en.json", jsonSrc},
		{"logo.png", pngSrc},
		{"data.bin", binSrc},
	})

	var out bytes.Buffer
	require.NoError(t, container.Repack(container.KindZip, data,
		map[string][]byte{"en.json": []byte(`{"greeting":"BONJOUR"}`)}, &out))

	got := readZip(t, out.Bytes())
	assert.Equal(t, `{"greeting":"BONJOUR"}`, string(got["en.json"]))
	assert.Equal(t, pngSrc, got["logo.png"], "untouched binary must be byte-identical")
	assert.Equal(t, binSrc, got["data.bin"])
}

func TestRepackTar(t *testing.T) {
	for _, gz := range []bool{false, true} {
		name := map[bool]string{false: "tar", true: "tar.gz"}[gz]
		t.Run(name, func(t *testing.T) {
			jsonSrc := []byte(`{"greeting":"Hello"}`)
			binSrc := []byte("\x00opaque")
			data := makeTar(t, []entry{{"en.json", jsonSrc}, {"data.bin", binSrc}}, gz)
			kind := container.Detect(data)

			var out bytes.Buffer
			require.NoError(t, container.Repack(kind, data,
				map[string][]byte{"en.json": []byte(`{"greeting":"HEI"}`)}, &out))

			got := readTar(t, out.Bytes(), gz)
			assert.Equal(t, `{"greeting":"HEI"}`, string(got["en.json"]))
			assert.Equal(t, binSrc, got["data.bin"])
		})
	}
}

func TestRepackNoReplacementsRoundTrips(t *testing.T) {
	data := makeZip(t, []entry{{"a.json", []byte(`{"k":"v"}`)}, {"b.bin", []byte("\x00\x01")}})
	var out bytes.Buffer
	require.NoError(t, container.Repack(container.KindZip, data, nil, &out))
	got := readZip(t, out.Bytes())
	assert.Equal(t, []byte(`{"k":"v"}`), got["a.json"])
	assert.Equal(t, []byte("\x00\x01"), got["b.bin"])
}

func readZip(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	out := make(map[string][]byte)
	for _, f := range zr.File {
		rc, err := f.Open()
		require.NoError(t, err)
		b, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = b
	}
	return out
}

func readTar(t *testing.T, data []byte, gzipped bool) map[string][]byte {
	t.Helper()
	var src io.Reader = bytes.NewReader(data)
	if gzipped {
		gz, err := gzip.NewReader(bytes.NewReader(data))
		require.NoError(t, err)
		defer gz.Close()
		src = gz
	}
	tr := tar.NewReader(src)
	out := make(map[string][]byte)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		b, _ := io.ReadAll(tr)
		out[hdr.Name] = b
	}
	return out
}
