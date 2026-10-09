package kbf

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// goDocumentTypes lists every DocumentType constant this package declares.
// A new constant is added here beside its declaration.
var goDocumentTypes = []DocumentType{DocumentTypeJSX}

// TestDocumentTypeMatchesTheTypeScriptMirror holds the Go constants and the
// `DocumentType` union in packages/kapi-format/src/block.ts to one list, so a
// value one side writes is a value the other side names.
func TestDocumentTypeMatchesTheTypeScriptMirror(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "packages", "kapi-format", "src", "block.ts"))
	require.NoError(t, err)

	decl := regexp.MustCompile(`export type DocumentType = ([^;]+);`).FindSubmatch(src)
	require.NotNil(t, decl, "block.ts declares `export type DocumentType = ...`")

	var ts []string
	for _, m := range regexp.MustCompile(`"([^"]+)"`).FindAllSubmatch(decl[1], -1) {
		ts = append(ts, string(m[1]))
	}
	var goValues []string
	for _, v := range goDocumentTypes {
		goValues = append(goValues, string(v))
	}
	assert.Equal(t, goValues, ts, "TypeScript union %s and Go constants disagree", strings.TrimSpace(string(decl[1])))
}
