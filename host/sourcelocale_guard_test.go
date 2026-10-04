package host

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSourceLocaleIsSetThroughFileUnderSource holds every assignment to a
// SourceLocale field in the host's own code to the sites listed here.
//
// A project read files each block under the project's source language, and
// fileUnderSource does it while keeping a language the block's reader declared
// (model.PropReadSourceLocale). The flow's history basis and a decision made
// through the change service are taken under that declared language, and
// model.Block.SourceRevisions matches them there. A read that sets the project's
// language on its own drops the declared one, and every basis and decision
// taken under it reads stale on the blocks that read hands out: the parked
// drafts of an ARB file declaring `en` in an `en-US` project would read
// untranslated, and no approval of them would count
// (TestConverge_ParkedDraftsOfADeclaredSourceLanguageStayCurrent).
//
// The other sites copy or fill a language rather than override one:
// analysisBlock copies the language of the block it stands in for,
// extractedRevision gives a block its reader left with none the project's, and
// Locate sets the language of a document (filehome.Doc), which its reader then
// applies or overrides with one the file declares. The check reads syntax
// alone, so a field of another type with the same name is listed too. A new
// site goes through fileUnderSource, or joins the list with its reason.
func TestSourceLocaleIsSetThroughFileUnderSource(t *testing.T) {
	allowed := []string{
		"changes.go:Locate",
		"commitcheck.go:analysisBlock",
		"merge_changes.go:extractedRevision",
		"verify.go:fileUnderSource",
	}
	fset := token.NewFileSet()
	var found []string
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != "." && (d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				as, ok := n.(*ast.AssignStmt)
				if !ok {
					return true
				}
				for _, lhs := range as.Lhs {
					if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "SourceLocale" {
						found = append(found, filepath.ToSlash(p)+":"+fn.Name.Name)
					}
				}
				return true
			})
		}
		return nil
	})
	require.NoError(t, err)

	for _, site := range found {
		assert.Contains(t, allowed, site,
			"%s sets a SourceLocale; file a read's blocks under the project's language with fileUnderSource", site)
	}
	for _, site := range allowed {
		assert.True(t, slices.Contains(found, site), "%s is listed and sets no SourceLocale any more", site)
	}
}
