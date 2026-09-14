package server

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListAutomationEvents_EveryOfferedTriggerHasAPublisher reads the
// platform's non-test Go source for the events it publishes and requires one
// for every event type the automation trigger picker offers. A rule saved on a
// trigger nothing publishes never runs, and nothing reports that it did not.
//
// A publisher writes the constant into an event's Type field: as the Type of an
// Event literal, or by assigning it to a Type field.
func TestListAutomationEvents_EveryOfferedTriggerHasAPublisher(t *testing.T) {
	s := &Server{}
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	require.NoError(t, s.HandleListAutomationEvents(c))
	var offered []struct {
		Type string `json:"type"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &offered))
	require.NotEmpty(t, offered)

	constants := eventTypeConstants(t, filepath.Join("..", "core", "event", "event.go"))
	published := publishedEventTypes(t, "..", constants)
	for _, o := range offered {
		assert.True(t, published[o.Type], "%s is offered as an automation trigger but no non-test code publishes it", o.Type)
	}
}

// eventTypeConstants maps each EventType constant declared in path to its
// string value.
func eventTypeConstants(t *testing.T, path string) map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	require.NoError(t, err)
	out := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) != len(vs.Values) {
				continue
			}
			for i, name := range vs.Names {
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				value, err := strconv.Unquote(lit.Value)
				require.NoError(t, err)
				out[name.Name] = value
			}
		}
	}
	require.NotEmpty(t, out, "no event type constants found in %s", path)
	return out
}

// publishedEventTypes walks the Go source under root, skipping tests, and
// returns the event types written into an event's Type field.
func publishedEventTypes(t *testing.T, root string, constants map[string]string) map[string]bool {
	t.Helper()
	published := map[string]bool{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); name == "node_modules" || name == "testdata" || (strings.HasPrefix(name, ".") && path != root) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		record := func(expr ast.Expr) {
			var name string
			switch v := expr.(type) {
			case *ast.SelectorExpr:
				name = v.Sel.Name
			case *ast.Ident:
				name = v.Name
			}
			if value, ok := constants[name]; ok {
				published[value] = true
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CompositeLit:
				if !isEventType(node.Type) {
					return true
				}
				for _, elt := range node.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Type" {
						record(kv.Value)
					}
				}
			case *ast.AssignStmt:
				if len(node.Lhs) != len(node.Rhs) {
					return true
				}
				for i, lhs := range node.Lhs {
					if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "Type" {
						record(node.Rhs[i])
					}
				}
			}
			return true
		})
		return nil
	})
	require.NoError(t, err)
	return published
}

// isEventType reports whether a composite literal's type is the platform Event.
func isEventType(expr ast.Expr) bool {
	switch v := expr.(type) {
	case *ast.SelectorExpr:
		return v.Sel.Name == "Event"
	case *ast.Ident:
		return v.Name == "Event"
	}
	return false
}
