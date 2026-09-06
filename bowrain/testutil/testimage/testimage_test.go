package testimage

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The list is what CI fetches before the suites run (make -C bowrain
// pull-test-images). A test that names its image inline is fetched by nothing,
// so it pulls at the moment it runs and a registry outage lands inside the
// test rather than at the step that exists to catch it. These read the module's
// own source and hold every container start to the list.

func TestAll_NamesEveryConstantOnce(t *testing.T) {
	all := All()
	assert.Equal(t, []string{Redis, Postgres, MinIO, ElasticMQ}, all)

	seen := map[string]bool{}
	for _, img := range all {
		require.NotEmpty(t, img)
		assert.False(t, seen[img], "%s is listed twice", img)
		seen[img] = true
	}
}

// A released tag, never `latest`: an image that changes under the suite makes
// a run irreproducible, and a pre-pull of `latest` warms the cache with
// whatever was published that morning.
func TestAll_PinsEveryTag(t *testing.T) {
	for _, img := range All() {
		name, tag, ok := strings.Cut(img, ":")
		require.True(t, ok, "%s names no tag", img)
		assert.NotEqual(t, "latest", tag, "%s is not pinned", name)
	}
}

// Every container a bowrain test starts comes from this package, so the fetch
// step and the test can never name different images.
func TestEveryContainerStartUsesTheList(t *testing.T) {
	root := bowrainModuleRoot(t)
	fset := token.NewFileSet()
	var offenders []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(root, path, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return nil // a file this package cannot parse is not this test's business
		}
		rel, _ := filepath.Rel(root, path)
		offenders = append(offenders, inlineImages(fset, file, rel)...)
		return nil
	})
	require.NoError(t, err)

	assert.Empty(t, offenders,
		"a container start names its image inline; add it to this package and reference the constant, "+
			"so `make -C bowrain pull-test-images` fetches it")
}

// inlineImages reports each place the file starts a container with a literal
// image name: the Image field of a testcontainers request, and the image
// argument of a testcontainers module's Run.
func inlineImages(fset *token.FileSet, file *ast.File, rel string) []string {
	var found []string
	report := func(pos token.Pos, lit *ast.BasicLit) {
		name, err := strconv.Unquote(lit.Value)
		if err != nil {
			name = lit.Value
		}
		found = append(found, rel+":"+strconv.Itoa(fset.Position(pos).Line)+" "+name)
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CompositeLit:
			if !isSelectorOn(node.Type, "testcontainers") {
				return true
			}
			for _, elt := range node.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok || key.Name != "Image" {
					continue
				}
				if lit, ok := kv.Value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					report(lit.Pos(), lit)
				}
			}
		case *ast.CallExpr:
			sel, ok := node.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Run" || len(node.Args) < 2 {
				return true
			}
			if lit, ok := node.Args[1].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				report(lit.Pos(), lit)
			}
		}
		return true
	})
	return found
}

// isSelectorOn reports whether expr is `pkg.Something`, possibly behind a
// pointer or a composite-literal element type.
func isSelectorOn(expr ast.Expr, pkg string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == pkg
}

// skipDir reports whether the walk should leave a directory alone: the nested
// modules under bowrain/ are built and tested on their own, and node_modules
// and build output hold no Go the suites run.
func skipDir(root, path, name string) bool {
	if path == root {
		return false
	}
	switch name {
	case "node_modules", "dist", "build", ".git", "testdata":
		return true
	}
	if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
		return true
	}
	return false
}

// bowrainModuleRoot walks up from the test's directory to the bowrain module.
func bowrainModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, "no go.mod above %s", dir)
		dir = parent
	}
}
