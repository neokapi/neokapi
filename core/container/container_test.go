package container_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
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

// writeFile writes data to a temp file and returns its path. Transform is
// path-based because it streams the archive from disk rather than buffering it.
func writeFile(t *testing.T, data []byte, ext string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "arc"+ext)
	require.NoError(t, os.WriteFile(p, data, 0o644))
	return p
}

// upperJSON replaces .json entries with uppercased bytes and copies everything
// else through (without reading it).
func upperJSON(name string, read func() ([]byte, error)) ([]byte, bool, error) {
	if filepath.Ext(name) != ".json" {
		return nil, false, nil
	}
	b, err := read()
	if err != nil {
		return nil, false, err
	}
	return bytes.ToUpper(b), true, nil
}

// TestTransform* are the barrier-sink fidelity guarantee: a replaced entry
// carries new bytes; every other member is byte-for-byte intact.
func TestTransformZip(t *testing.T) {
	pngSrc := []byte("\x89PNG\r\n\x1a\nbinary-1234")
	binSrc := []byte("\x00\x01opaque")
	path := writeFile(t, makeZip(t, []entry{
		{"en.json", []byte(`{"greeting":"hello"}`)},
		{"logo.png", pngSrc},
		{"data.bin", binSrc},
	}), ".zip")

	var out bytes.Buffer
	require.NoError(t, container.Transform(path, &out, upperJSON))

	got := readZip(t, out.Bytes())
	assert.Equal(t, `{"GREETING":"HELLO"}`, string(got["en.json"]))
	assert.Equal(t, pngSrc, got["logo.png"], "untouched binary must be byte-identical")
	assert.Equal(t, binSrc, got["data.bin"])
}

func TestTransformTar(t *testing.T) {
	for _, gz := range []bool{false, true} {
		name := map[bool]string{false: "tar", true: "tar.gz"}[gz]
		ext := map[bool]string{false: ".tar", true: ".tar.gz"}[gz]
		t.Run(name, func(t *testing.T) {
			binSrc := []byte("\x00opaque")
			path := writeFile(t, makeTar(t, []entry{
				{"en.json", []byte(`{"greeting":"hei"}`)},
				{"data.bin", binSrc},
			}, gz), ext)

			var out bytes.Buffer
			require.NoError(t, container.Transform(path, &out, upperJSON))

			got := readTar(t, out.Bytes(), gz)
			assert.Equal(t, `{"GREETING":"HEI"}`, string(got["en.json"]))
			assert.Equal(t, binSrc, got["data.bin"])
		})
	}
}

func TestTransformRejectsPathTraversal(t *testing.T) {
	path := writeFile(t, makeZip(t, []entry{{"../evil", []byte("x")}}), ".zip")
	var out bytes.Buffer
	err := container.Transform(path, &out, upperJSON)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsafe entry name")
}

func TestEnumerateRejectsPathTraversal(t *testing.T) {
	for _, bad := range []string{"../evil.txt", "a/../../evil", "/etc/passwd", "..\\evil"} {
		t.Run(bad, func(t *testing.T) {
			data := makeZip(t, []entry{{bad, []byte("x")}})
			_, _, err := container.Enumerate(data)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unsafe entry name")
		})
	}
	// A normal nested path is fine.
	data := makeZip(t, []entry{{"a/b/ok.json", []byte("{}")}})
	_, entries, err := container.Enumerate(data)
	require.NoError(t, err)
	assert.Equal(t, []string{"a/b/ok.json"}, names(entries))
}

func TestTransformNoReplacementsRoundTrips(t *testing.T) {
	path := writeFile(t, makeZip(t, []entry{
		{"a.json", []byte(`{"k":"v"}`)}, {"b.bin", []byte("\x00\x01")},
	}), ".zip")
	var out bytes.Buffer
	require.NoError(t, container.Transform(path, &out, func(string, func() ([]byte, error)) ([]byte, bool, error) {
		return nil, false, nil
	}))
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
