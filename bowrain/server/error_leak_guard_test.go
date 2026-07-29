package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The rule this file enforces
//
// A 5xx says the failure is ours. Whatever produced it — a driver, a migration,
// the filesystem, a connector talking to someone else's API — wrote its message
// for an operator reading a log, not for whoever provoked it over the network.
// Postgres names the relation and the constraint; a path error names the path;
// an OIDC client quotes the issuer it could not reach.
//
// httpErrorHandler already gets this right: it logs the raw error against the
// request's reference ID and answers with a generic envelope. But it only sees
// errors that are *returned* up the Echo chain. A handler that writes its own
// response with c.JSON and returns nil never reaches it, and 370 call sites had
// quietly grown that way — the sanitiser was correct and bypassed.
//
// So the rule is not "call the sanitiser", which is unenforceable, but the
// shape that makes bypassing it visible: no directly-written 5xx body may carry
// error text that came from anywhere but this package's own authored strings.
// Use serverErr / serverErrStatus, which return.
//
// What counts as safe: a package-level sentinel — `var errX = errors.New("…")`.
// Its text is a literal in this repository, written for a reader and reviewed
// like any other prose, and it is often the most useful thing we can say (a 503
// carrying "brand knowledge graph not configured" tells an operator exactly
// what to do). Those are collected from the source rather than listed by hand,
// so adding one needs no edit here.
//
// What this cannot catch: an error text laundered through a variable —
//
//	msg := err.Error()
//	return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: msg})
//
// Following that needs type and flow analysis this deliberately does not do. It
// catches the direct shape, which is what every one of the 370 sites was and
// what a copied neighbour will be.

// fiveXX is the set of status constants this guard treats as "ours".
var fiveXX = map[string]bool{
	"StatusInternalServerError":           true,
	"StatusNotImplemented":                true,
	"StatusBadGateway":                    true,
	"StatusServiceUnavailable":            true,
	"StatusGatewayTimeout":                true,
	"StatusHTTPVersionNotSupported":       true,
	"StatusVariantAlsoNegotiates":         true,
	"StatusInsufficientStorage":           true,
	"StatusLoopDetected":                  true,
	"StatusNotExtended":                   true,
	"StatusNetworkAuthenticationRequired": true,
}

// responseWriters are the echo.Context methods that commit a body, plus this
// package's own direct writer. A call to one of these with a 5xx status has
// bypassed httpErrorHandler by construction.
var responseWriters = map[string]bool{
	"JSON": true, "JSONPretty": true, "JSONBlob": true,
	"String": true, "HTML": true, "HTMLBlob": true,
	"XML": true, "XMLBlob": true, "Blob": true,
}

func TestNo5xxResponseCarriesInternalDetail(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)

	fset := token.NewFileSet()
	parsed := map[string]*ast.File{}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		require.NoError(t, perr, "parse %s", name)
		parsed[name] = f
	}
	require.NotEmpty(t, parsed, "guard found no source to check — it has been orphaned")

	// Collection is proved on a fixture by the companion test rather than
	// against a named production sentinel, which would couple this guard to an
	// identifier that may legitimately be renamed.
	sentinels := packageSentinels(parsed)

	var findings []string
	for _, f := range parsed {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			payload, ok := fiveXXPayload(call)
			if !ok {
				return true
			}
			for _, arg := range payload {
				for _, leak := range dynamicErrorText(arg, sentinels) {
					findings = append(findings, fileLine(fset, leak.Pos())+": "+
						exprString(leak))
				}
			}
			return true
		})
	}

	if len(findings) > 0 {
		t.Errorf("a 5xx response body carries error text that this package did not author "+
			"(%d site(s)).\n\n"+
			"Return the error instead of writing it — serverErr(c, err), or "+
			"serverErrStatus(c, http.StatusBadGateway, err) when the status is a\n"+
			"deliberate statement about where the failure was. httpErrorHandler then "+
			"logs the cause against the request's reference ID and answers\n"+
			"the client generically. Add server-side context with "+
			"fmt.Errorf(\"doing the thing: %%w\", err); it reaches the log, not the wire.\n\n"+
			"  %s", len(findings), strings.Join(findings, "\n  "))
	}
}

// packageSentinels collects package-level `var x = errors.New("literal")` and
// `var x = fmt.Errorf("literal")` names. Their text is authored here, so it is
// safe — and often the most useful thing — to put on the wire.
func packageSentinels(files map[string]*ast.File) map[string]bool {
	out := map[string]bool{}
	for _, f := range files {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if i >= len(vs.Values) {
						continue
					}
					if isStaticErrorCtor(vs.Values[i]) {
						out[name.Name] = true
					}
				}
			}
		}
	}
	return out
}

// isStaticErrorCtor reports whether e is errors.New("…") or fmt.Errorf("…")
// with no interpolated operands — i.e. an error whose text is a literal.
func isStaticErrorCtor(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	if !(pkg.Name == "errors" && sel.Sel.Name == "New") &&
		!(pkg.Name == "fmt" && sel.Sel.Name == "Errorf") {
		return false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	return ok && lit.Kind == token.STRING
}

// fiveXXPayload reports the body arguments of a call that writes a 5xx response
// directly, and whether the call is one.
func fiveXXPayload(call *ast.CallExpr) ([]ast.Expr, bool) {
	// apiErr(c, http.Status5xx, code, …)
	if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "apiErr" {
		if len(call.Args) >= 3 && isFiveXX(call.Args[1]) {
			return call.Args[2:], true
		}
		return nil, false
	}
	// c.JSON(http.Status5xx, body) and friends.
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !responseWriters[sel.Sel.Name] {
		return nil, false
	}
	if len(call.Args) >= 2 && isFiveXX(call.Args[0]) {
		return call.Args[1:], true
	}
	return nil, false
}

func isFiveXX(e ast.Expr) bool {
	if sel, ok := e.(*ast.SelectorExpr); ok {
		if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "http" {
			return fiveXX[sel.Sel.Name]
		}
		return false
	}
	// A bare numeric status, should one ever be written.
	if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.INT {
		n, err := strconv.Atoi(lit.Value)
		return err == nil && n >= 500 && n < 600
	}
	return false
}

// dynamicErrorText returns the sub-expressions of arg that put error text on
// the wire — an `x.Error()` whose receiver is not a package sentinel, or an
// error value handed to fmt.Sprintf.
func dynamicErrorText(arg ast.Expr, sentinels map[string]bool) []ast.Expr {
	var out []ast.Expr
	ast.Inspect(arg, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel.Name == "Error" && len(call.Args) == 0 {
			if id, ok := sel.X.(*ast.Ident); ok && sentinels[id.Name] {
				return true // authored, static text
			}
			out = append(out, call)
			return true
		}
		// fmt.Sprintf("…: %s", err) — the same leak with a different spelling.
		if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "fmt" &&
			(sel.Sel.Name == "Sprintf" || sel.Sel.Name == "Sprint") {
			for _, a := range call.Args {
				if id, ok := a.(*ast.Ident); ok && looksLikeError(id.Name) && !sentinels[id.Name] {
					out = append(out, call)
					break
				}
			}
		}
		return true
	})
	return out
}

// looksLikeError is a name heuristic, used only for the fmt.Sprintf case where
// there is no call to .Error() to key on. Go's conventions make it reliable
// enough for a guard: an error value in this package is err, mergeErr, rtErr.
func looksLikeError(name string) bool {
	return name == "err" || strings.HasSuffix(name, "Err") ||
		strings.HasSuffix(name, "Error")
}

func fileLine(fset *token.FileSet, p token.Pos) string {
	pos := fset.Position(p)
	return filepath.Base(pos.Filename) + ":" + strconv.Itoa(pos.Line)
}

func exprString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.CallExpr:
		if sel, ok := v.Fun.(*ast.SelectorExpr); ok {
			return exprString(sel.X) + "." + sel.Sel.Name + "(…)"
		}
		return "call(…)"
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return exprString(v.X) + "." + v.Sel.Name
	default:
		return "expression"
	}
}

// TestErrorLeakGuardCatchesTheShapeItClaimsTo proves the matcher both ways. A
// guard that has silently stopped matching is worse than none: it reads as
// evidence the category is closed.
func TestErrorLeakGuardCatchesTheShapeItClaimsTo(t *testing.T) {
	const src = `package server

import (
	"errors"
	"fmt"
	"net/http"
)

var errStatic = errors.New("the store is not configured")

func dirty(c echo.Context, err error) error {
	return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
}

func dirtyPrefixed(c echo.Context, err error) error {
	return apiErr(c, http.StatusBadGateway, "upstream: "+err.Error())
}

func dirtySprintf(c echo.Context, err error) error {
	return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: fmt.Sprintf("x: %s", err)})
}

func cleanSentinel(c echo.Context) error {
	return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errStatic.Error()})
}

func cleanReturned(c echo.Context, err error) error {
	return serverErr(c, fmt.Errorf("doing the thing: %w", err))
}

func cleanClientError(c echo.Context, err error) error {
	return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "sample.go", src, parser.SkipObjectResolution)
	require.NoError(t, err)

	files := map[string]*ast.File{"sample.go": f}
	sentinels := packageSentinels(files)
	require.True(t, sentinels["errStatic"], "sentinel collection missed a static error")

	var lines []int
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		payload, ok := fiveXXPayload(call)
		if !ok {
			return true
		}
		for _, arg := range payload {
			for _, leak := range dynamicErrorText(arg, sentinels) {
				lines = append(lines, fset.Position(leak.Pos()).Line)
			}
		}
		return true
	})

	require.Len(t, lines, 3,
		"expected exactly the three dirty shapes to be flagged, got lines %v", lines)
	// A 4xx echoing the caller's own error is fine; a 5xx sentinel is authored
	// text; a returned error never reaches a response writer at all.
	require.Equal(t, []int{12, 16, 20}, lines,
		"the guard flagged the wrong lines — check the clean cases have not started matching")
}
