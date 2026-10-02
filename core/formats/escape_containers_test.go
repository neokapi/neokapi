package formats

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/registry"
)

// The escape sweep covers every format whose reader and writer share a
// skeleton: every writer that writes an edited document back in place renders
// edited text, so each one is held to the escaping contract
// (docs/internals/edit-model.md, section 2.4, rule 6). The ZIP containers
// carry their text in XML members and are swept with the operations matrix's
// container fixtures.

// containerSweepFormats are the ZIP container formats of the sweep.
func containerSweepFormats(t *testing.T) []escapeSweepEntry {
	t.Helper()
	return []escapeSweepEntry{
		{id: "epub", source: zipFixture(t, "epub.tmpl"), rejects: notRepresentableInXML, skip: xmlFlowWhitespace},
		{id: "odf", source: zipFixture(t, "odf.tmpl"), rejects: notRepresentableInXML, skip: xmlFlowWhitespace},
		{id: "openxml", source: zipFixture(t, "openxml.tmpl"), rejects: notRepresentableInXML, skip: xmlFlowWhitespace},
	}
}

// xmlFlowWhitespace is the whitespace a container's text flow does not carry
// as a character.
var xmlFlowWhitespace = map[rune]string{
	'\t': "the text flow carries a tab as an element of its own or collapses it",
	'\n': "the text flow carries a line break as an element of its own or collapses it",
	'\r': "the text flow carries a line break as an element of its own or collapses it",
}

// zipFixture packs an operations-matrix container template into a ZIP, with
// its marked words written plainly: a stored mimetype member first, then the
// rest by name.
func zipFixture(t *testing.T, dir string) string {
	t.Helper()
	root := filepath.Join("testdata", "opsmatrix", dir)
	members := map[string][]byte{}
	unmark := strings.NewReplacer("«", "", "»", "")
	require.NoError(t, filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		members[strings.TrimSuffix(filepath.ToSlash(rel), ".tmpl")] = []byte(unmark.Replace(string(raw)))
		return nil
	}))
	names := make([]string, 0, len(members))
	for name := range members {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if (names[i] == "mimetype") != (names[j] == "mimetype") {
			return names[i] == "mimetype"
		}
		return names[i] < names[j]
	})
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range names {
		method := zip.Deflate
		if name == "mimetype" {
			method = zip.Store
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method})
		require.NoError(t, err)
		_, err = w.Write(members[name])
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.String()
}

// TestEscapeSweepCoversEverySkeletonPair keeps the sweep exhaustive: a format
// that gains a skeleton pair writes edited text back in place, so it joins
// the sweep.
func TestEscapeSweepCoversEverySkeletonPair(t *testing.T) {
	reg := registry.NewFormatRegistry()
	RegisterAll(reg)
	swept := map[string]bool{}
	for _, e := range append(escapeSweepFormats(), containerSweepFormats(t)...) {
		swept[e.id] = true
	}
	for _, name := range reg.WriterNames() {
		writer, err := reg.NewWriter(name)
		if err != nil {
			continue
		}
		reader, err := reg.NewReader(name)
		if err != nil {
			continue
		}
		if format.SkeletonPairEligible(reader, writer) {
			assert.True(t, swept[string(name)], "%s writes edited text back through its skeleton and is not in the escape sweep", name)
		}
	}
}
