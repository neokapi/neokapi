package safeio_test

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/safeio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// zipEntry describes one entry to write into a test archive.
type zipEntry struct {
	name  string
	data  []byte
	store bool // true → zip.Store (no compression), false → zip.Deflate
}

// makeZip builds an in-memory zip archive and returns a *zip.Reader over it.
func makeZip(t *testing.T, entries ...zipEntry) *zip.Reader {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		method := zip.Deflate
		if e.store {
			method = zip.Store
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: e.name, Method: method})
		require.NoError(t, err)
		_, err = w.Write(e.data)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	return zr
}

func fileByName(zr *zip.Reader, name string) *zip.File {
	for _, f := range zr.File {
		if f.Name == name {
			return f
		}
	}
	return nil
}

func TestZipLimits_ReadEntry_HappyPath(t *testing.T) {
	t.Parallel()
	want := []byte("hello, faithful round-trip")
	zr := makeZip(t, zipEntry{name: "doc.xml", data: want})
	got, err := safeio.DefaultZipLimits.ReadEntry(fileByName(zr, "doc.xml"))
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestZipLimits_ZipBombRejected(t *testing.T) {
	t.Parallel()
	// 2 MiB of zeros deflates to a few KB → ratio far below 0.01, and the
	// uncompressed size is well past the 100 KiB grace window.
	bomb := make([]byte, 2<<20)
	zr := makeZip(t, zipEntry{name: "bomb.bin", data: bomb})
	f := fileByName(zr, "bomb.bin")

	// Header-level check (CheckReader / CheckEntry) catches the declared ratio.
	assert.ErrorIs(t, safeio.DefaultZipLimits.CheckReader(zr), safeio.ErrInflateRatio)
	assert.ErrorIs(t, safeio.DefaultZipLimits.CheckEntry(f), safeio.ErrInflateRatio)

	// Opening/reading the entry also fails with the typed error.
	_, err := safeio.DefaultZipLimits.ReadEntry(f)
	require.Error(t, err)
	assert.ErrorIs(t, err, safeio.ErrInflateRatio)
	var le *safeio.LimitError
	assert.ErrorAs(t, err, &le)
	assert.Equal(t, "bomb.bin", le.Name)
}

func TestZipLimits_EntryTooLarge(t *testing.T) {
	t.Parallel()
	// Stored (incompressible) 200 KiB entry, but per-entry cap is 64 KiB.
	data := bytes.Repeat([]byte("A"), 200<<10)
	zr := makeZip(t, zipEntry{name: "big.txt", data: data, store: true})
	limits := safeio.ZipLimits{MaxEntrySize: 64 << 10}
	f := fileByName(zr, "big.txt")

	assert.ErrorIs(t, limits.CheckEntry(f), safeio.ErrEntryTooLarge)
	_, err := limits.ReadEntry(f)
	require.Error(t, err)
	assert.ErrorIs(t, err, safeio.ErrEntryTooLarge)
}

func TestZipLimits_TotalTooLarge_Declared(t *testing.T) {
	t.Parallel()
	// Two 60 KiB stored entries (each under the 100 KiB grace + entry cap),
	// total 120 KiB > MaxTotalSize 100 KiB.
	e := bytes.Repeat([]byte("B"), 60<<10)
	zr := makeZip(t,
		zipEntry{name: "a.txt", data: e, store: true},
		zipEntry{name: "b.txt", data: e, store: true},
	)
	limits := safeio.ZipLimits{MaxTotalSize: 100 << 10}
	assert.ErrorIs(t, limits.CheckReader(zr), safeio.ErrTotalTooLarge)
}

func TestZipGuard_TotalTooLarge_Streaming(t *testing.T) {
	t.Parallel()
	// Same shape, but exercise the streaming cumulative total across two
	// reads through one guard (no up-front CheckReader).
	e := bytes.Repeat([]byte("C"), 60<<10)
	zr := makeZip(t,
		zipEntry{name: "a.txt", data: e, store: true},
		zipEntry{name: "b.txt", data: e, store: true},
	)
	guard := safeio.ZipLimits{MaxTotalSize: 100 << 10}.NewGuard()

	_, err := guard.ReadEntry(fileByName(zr, "a.txt"))
	require.NoError(t, err, "first 60 KiB entry fits")

	_, err = guard.ReadEntry(fileByName(zr, "b.txt"))
	require.Error(t, err, "second entry pushes cumulative total over cap")
	assert.ErrorIs(t, err, safeio.ErrTotalTooLarge)
}

func TestZipLimits_TooManyEntries(t *testing.T) {
	t.Parallel()
	entries := make([]zipEntry, 5)
	for i := range entries {
		entries[i] = zipEntry{name: string(rune('a'+i)) + ".txt", data: []byte("x"), store: true}
	}
	zr := makeZip(t, entries...)
	limits := safeio.ZipLimits{MaxEntries: 3}
	err := limits.CheckReader(zr)
	require.Error(t, err)
	assert.ErrorIs(t, err, safeio.ErrTooManyEntries)
}

func TestZipGuard_TooManyEntries_Streaming(t *testing.T) {
	t.Parallel()
	zr := makeZip(t,
		zipEntry{name: "a.txt", data: []byte("x"), store: true},
		zipEntry{name: "b.txt", data: []byte("y"), store: true},
	)
	guard := safeio.ZipLimits{MaxEntries: 1}.NewGuard()
	_, err := guard.ReadEntry(fileByName(zr, "a.txt"))
	require.NoError(t, err)
	_, err = guard.Open(fileByName(zr, "b.txt"))
	require.Error(t, err)
	assert.ErrorIs(t, err, safeio.ErrTooManyEntries)
}

func TestZipLimits_ValidArchivePasses(t *testing.T) {
	t.Parallel()
	// A normal small archive must pass every check unchanged — the
	// "behavior identical for valid inputs" contract.
	zr := makeZip(t,
		zipEntry{name: "[Content_Types].xml", data: []byte(`<?xml version="1.0"?><Types/>`)},
		zipEntry{name: "word/document.xml", data: []byte(`<w:document><w:body/></w:document>`)},
	)
	require.NoError(t, safeio.DefaultZipLimits.CheckReader(zr))
	for _, f := range zr.File {
		_, err := safeio.DefaultZipLimits.ReadEntry(f)
		require.NoError(t, err, f.Name)
	}
}

func TestZipLimits_DefaultsBackfilled(t *testing.T) {
	t.Parallel()
	// A partially-specified ZipLimits still enforces the unspecified caps from
	// the package defaults (zero fields are not "unlimited").
	zr := makeZip(t, zipEntry{name: "bomb.bin", data: make([]byte, 2<<20)})
	limits := safeio.ZipLimits{MaxEntrySize: 8 << 20} // only entry-size set
	// MinInflateRatio is zero → backfilled to the default → bomb still caught.
	assert.ErrorIs(t, limits.CheckEntry(fileByName(zr, "bomb.bin")), safeio.ErrInflateRatio)
}

// TestZipGuard_OpenReadClose covers the streaming reader's Read/Close on a
// valid entry (the happy streaming path).
func TestZipGuard_OpenReadClose(t *testing.T) {
	t.Parallel()
	want := bytes.Repeat([]byte("payload"), 1000)
	zr := makeZip(t, zipEntry{name: "p.txt", data: want, store: true})
	rc, err := safeio.DefaultZipLimits.OpenEntry(fileByName(zr, "p.txt"))
	require.NoError(t, err)
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())
	assert.Equal(t, want, got)
}
