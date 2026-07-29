package xml_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	xmlfmt "github.com/neokapi/neokapi/core/formats/xml"
	"github.com/neokapi/neokapi/core/model"
)

// External ITS rules are pulled in by the document being read, so the href is
// content. The parity contract scopes them to sibling files in the project
// tree; these hold that boundary rather than leaving it to the doc comment.

// readXMLFileBlocks reads an XML file from disk (so r.Doc.URI names a real
// directory, which is what external-href resolution keys off) and returns the
// translatable block texts.
func readXMLFileBlocks(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	reader := xmlfmt.NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)

	ctx := context.Background()
	require.NoError(t, reader.Open(ctx, &model.RawDocument{
		URI: path, SourceLocale: "en", FormatID: "xml", Reader: f,
	}))
	defer reader.Close()

	var texts []string
	for res := range reader.Read(ctx) {
		require.NoError(t, res.Error)
		if res.Part.Type != model.PartBlock {
			continue
		}
		if b, ok := res.Part.Resource.(*model.Block); ok && b.Translatable {
			texts = append(texts, b.SourceText())
		}
	}
	return texts
}

const itsDoc = `<?xml version="1.0"?>
<doc xmlns:its="http://www.w3.org/2005/11/its" xmlns:xlink="http://www.w3.org/1999/xlink">
  <its:rules version="2.0" xlink:href="%HREF%"/>
  <p>Translate me</p>
  <code>do not translate</code>
</doc>`

const itsRules = `<?xml version="1.0"?>
<its:rules version="2.0" xmlns:its="http://www.w3.org/2005/11/its">
  <its:translateRule selector="//code" translate="no"/>
</its:rules>`

// writeITSCase writes the document with the given href into docDir and returns
// its path.
func writeITSCase(t *testing.T, docDir, href string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(docDir, 0o755))
	path := filepath.Join(docDir, "doc.xml")
	content := strings.ReplaceAll(itsDoc, "%HREF%", href)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// TestExternalITSRulesLoadFromASibling is the control: an href naming a file
// beside the document still works, so the containment below is not just
// breaking the feature.
func TestExternalITSRulesLoadFromASibling(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rules.xml"), []byte(itsRules), 0o644))
	doc := writeITSCase(t, dir, "rules.xml")

	texts := readXMLFileBlocks(t, doc)
	assert.Contains(t, texts, "Translate me")
	assert.NotContains(t, texts, "do not translate",
		"a sibling rules file must still suppress the element it names")
}

// TestExternalITSRulesLoadFromASubdirectory: nesting below the document's own
// directory is inside the tree, so it stays supported.
func TestExternalITSRulesLoadFromASubdirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "its"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "its", "rules.xml"), []byte(itsRules), 0o644))
	doc := writeITSCase(t, dir, "its/rules.xml")

	texts := readXMLFileBlocks(t, doc)
	assert.NotContains(t, texts, "do not translate")
}

// TestExternalITSRulesCannotClimbOutOfTheDocumentDirectory: the href is written
// by whoever wrote the document, so it must not be able to reach a file the
// project never offered — the same rule the absolute-path and URI cases already
// followed. An unreadable reference degrades to "no external rules", which is
// this reader's documented behaviour for one that does not resolve.
func TestExternalITSRulesCannotClimbOutOfTheDocumentDirectory(t *testing.T) {
	root := t.TempDir()
	// The rules live OUTSIDE the document's directory.
	require.NoError(t, os.WriteFile(filepath.Join(root, "rules.xml"), []byte(itsRules), 0o644))
	doc := writeITSCase(t, filepath.Join(root, "docs"), "../rules.xml")

	texts := readXMLFileBlocks(t, doc)
	assert.Contains(t, texts, "Translate me")
	assert.Contains(t, texts, "do not translate",
		"rules reached by climbing out of the document's directory must not apply")
}

// TestExternalITSRulesCannotFollowASymlinkOutOfTheTree: a link inside the
// directory is still a way out of it, which a path-string check would miss.
// OpenInRoot confines the open at the OS level, so it does not.
func TestExternalITSRulesCannotFollowASymlinkOutOfTheTree(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "rules.xml"), []byte(itsRules), 0o644))

	docDir := filepath.Join(root, "docs")
	doc := writeITSCase(t, docDir, "rules.xml")
	// "rules.xml" beside the document is a link to the one outside it.
	if err := os.Symlink(filepath.Join(root, "rules.xml"), filepath.Join(docDir, "rules.xml")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	texts := readXMLFileBlocks(t, doc)
	assert.Contains(t, texts, "do not translate",
		"a symlink leading out of the document's directory must not be followed")
}
