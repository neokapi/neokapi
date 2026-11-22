// Command keyguard reports map keys built by joining parts with a control
// character delimiter.
//
// A key such as a + "\x00" + b names two different tuples the same way when a
// part contains the delimiter: ("a\x00b", "c") and ("a", "b\x00c") share the
// key "a\x00b\x00c". A struct or array key (struct{a, b string}, [2]string)
// keeps every tuple apart. So this type-checks the packages in targets and
// reports each string joined with a delimiter that is used as a key:
//
//   - a join is a + chain with a delimiter operand, strings.Join(parts,
//     delimiter), or fmt.Sprintf with a delimiter in its format;
//   - a key is the index of a map with string keys, a key in a map literal,
//     the key of delete, or the key argument of a sync.Map method;
//   - the join reaches the key directly, through a variable assigned it, or
//     through a function (or a function literal held in a variable) that
//     returns it as any of its results.
//
// A delimiter is a constant string holding a control character other than
// tab, newline, carriage return and escape, such as "\x00", "\x1f" or
// "\x00kind\x00". A + chain also counts when one operand is a join passed
// through strings.ToLower, strings.ToUpper or strings.TrimSpace. A function
// that returns a join carries it to every caller, in its own package and in
// the packages checked after it.
//
// A join used for something else, such as a log line or a hash input, is not
// reported. A join that is a persisted identity, where a new spelling would
// change stored keys, is exempt when its line holds, or the line above it
// holds, the directive
//
//	//keyguard:persisted <reason>
//
// The reason says where the identity is stored. A directive without a reason,
// or on no delimiter join, is reported too.
//
// Run from the repository root:
//
//	go run ./scripts/keyguard            # check the repository
//	go run ./scripts/keyguard -self-test # prove the check on fixtures
package main

import (
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/scripts/internal/buildconfigs"
	"github.com/neokapi/neokapi/scripts/internal/guardpackages"
)

// targets are the packages checked: the Apache modules.
var targets = []string{
	"./core/...", "./memory/...", "./terms/...", "./providers/...",
	"./host/...", "./cli/...", "./kapi/...", "./apps/kapi-desktop/backend/...",
	"./bowrain/plugin/...",
}

const directive = "//keyguard:persisted"

func main() {
	selfTest := flag.Bool("self-test", false, "prove the check on fixtures")
	configName := flag.String("config", "host", "build configuration to check under (scripts/internal/buildconfigs)")
	flag.Parse()
	var (
		found []string
		err   error
	)
	if *selfTest {
		err = runSelfTest()
	} else {
		var cfgs []buildconfigs.Config
		if cfgs, err = buildconfigs.Named(*configName); err == nil {
			found, err = check(cfgs[0], targets)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "keyguard:", err)
		os.Exit(2)
	}
	if len(found) > 0 {
		fmt.Fprintln(os.Stderr, "keyguard: these keys join parts with a delimiter, so two different tuples share a key when a part contains it:")
		for _, f := range found {
			fmt.Fprintln(os.Stderr, "  "+f)
		}
		fmt.Fprintln(os.Stderr, "Use a struct or array key (struct{a, b string}, [2]string). A persisted identity keeps its spelling with "+directive+" <where it is stored>.")
		os.Exit(1)
	}
}

// check type-checks every target package and reports each keyed join found.
// In a configuration after the first, only the packages holding a file no
// earlier configuration compiles are checked.
func check(cfg buildconfigs.Config, patterns []string) ([]string, error) {
	first := buildconfigs.Index(cfg.Name) == 0
	pkgs, err := guardpackages.ListConfig(cfg, patterns...)
	if err != nil {
		return nil, err
	}
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	imp := pkgs.Importer(fset)
	st := newState(fset, func(pos token.Pos) string {
		rel, rerr := filepath.Rel(root, fset.Position(pos).Filename)
		if rerr != nil {
			return fset.Position(pos).Filename
		}
		return filepath.ToSlash(rel)
	})
	checked := 0
	// The list holds each package after its dependencies, so a function's
	// joins are known before a caller in another package is checked.
	for _, p := range pkgs {
		if p.DepOnly || p.Standard || !strings.HasPrefix(p.ImportPath, "github.com/neokapi/neokapi/") {
			continue
		}
		if len(p.CompiledGoFiles) == 0 {
			continue
		}
		files, err := p.ParseFilesMode(fset, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		if _, err := st.checkPackage(imp, p.ImportPath, files); err != nil {
			return nil, fmt.Errorf("%s: %w", p.ImportPath, err)
		}
		checked++
	}
	if checked == 0 && first {
		return nil, errors.New("no package to check: run from the repository root")
	}
	fmt.Printf("keyguard: checked %d packages (%s)\n", checked, cfg.Name)
	sort.Strings(st.found)
	return st.found, nil
}

// joins is a set of join expressions, in the order they were added.
type joins struct {
	list []ast.Expr
	has  map[ast.Expr]bool
}

// add adds es and reports whether any was new.
func (j *joins) add(es ...ast.Expr) bool {
	added := false
	for _, e := range es {
		if j.has == nil {
			j.has = map[ast.Expr]bool{}
		}
		if !j.has[e] {
			j.has[e] = true
			j.list = append(j.list, e)
			added = true
		}
	}
	return added
}

// state is what one run learns across packages.
type state struct {
	fset    *token.FileSet
	relFile func(token.Pos) string
	// funcs maps a function's full name to the joins each of its results
	// carries.
	funcs map[string]map[int]*joins
	// exempt holds the lines a directive covers, per file.
	exempt   map[string]map[int]bool
	reported map[token.Pos]bool
	found    []string
}

func newState(fset *token.FileSet, relFile func(token.Pos) string) *state {
	return &state{
		fset:     fset,
		relFile:  relFile,
		funcs:    map[string]map[int]*joins{},
		exempt:   map[string]map[int]bool{},
		reported: map[token.Pos]bool{},
	}
}

// pkgCheck holds one package's type information and the joins its variables
// carry.
type pkgCheck struct {
	*state
	info *types.Info
	// vars maps a variable to the joins assigned to it.
	vars map[types.Object]*joins
	// lits maps a variable holding a function literal to the joins each of
	// the literal's results carries.
	lits map[types.Object]map[int]*joins
}

// checkPackage type-checks one package, records the joins its functions
// return, and reports every join used as a key that no directive exempts and
// every directive that is malformed or on no join. It returns the package's
// types.
func (st *state) checkPackage(imp types.Importer, path string, files []*ast.File) (*types.Package, error) {
	fset := st.fset
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Uses:  map[*ast.Ident]types.Object{},
		Defs:  map[*ast.Ident]types.Object{},
	}
	var typeErrs []error
	conf := types.Config{Importer: imp, Error: func(err error) { typeErrs = append(typeErrs, err) }}
	pkg, err := conf.Check(path, fset, files, info)
	if err != nil && len(typeErrs) > 0 {
		return nil, typeErrs[0]
	}
	c := &pkgCheck{state: st, info: info, vars: map[types.Object]*joins{}, lits: map[types.Object]map[int]*joins{}}
	// A variable can take a function's result and a function can return a
	// variable, so the pass repeats until it learns nothing new.
	for range 10 {
		changed := false
		for _, f := range files {
			if c.collect(f) {
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	// The lines that hold a join, per file, for the directive check.
	joinLines := map[string]map[int]bool{}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			if e, ok := n.(ast.Expr); ok && c.isJoin(e) {
				p := fset.Position(e.Pos())
				if joinLines[p.Filename] == nil {
					joinLines[p.Filename] = map[int]bool{}
				}
				joinLines[p.Filename][p.Line] = true
			}
			return true
		})
	}
	for _, f := range files {
		for _, cg := range f.Comments {
			for _, cm := range cg.List {
				st.directive(cm, joinLines)
			}
		}
	}

	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			for _, k := range c.mapKeys(n) {
				for _, j := range c.carried(k) {
					st.report(j, k)
				}
			}
			return true
		})
	}
	return pkg, nil
}

// directive records the line a //keyguard:persisted comment exempts, or
// reports the comment when it has no reason or sits on no join.
func (st *state) directive(cm *ast.Comment, joinLines map[string]map[int]bool) {
	if !strings.HasPrefix(cm.Text, directive) {
		return
	}
	p := st.fset.Position(cm.Pos())
	reason := strings.TrimSpace(strings.TrimPrefix(cm.Text, directive))
	if reason == "" {
		st.found = append(st.found, fmt.Sprintf("%s:%d: %s needs a reason that says where the identity is stored", st.relFile(cm.Pos()), p.Line, directive))
		return
	}
	// The directive covers its own line (a trailing comment) or the line
	// below it.
	line := p.Line
	if !joinLines[p.Filename][line] {
		line++
	}
	if !joinLines[p.Filename][line] {
		st.found = append(st.found, fmt.Sprintf("%s:%d: %s is on no delimiter join; remove it", st.relFile(cm.Pos()), p.Line, directive))
		return
	}
	if st.exempt[p.Filename] == nil {
		st.exempt[p.Filename] = map[int]bool{}
	}
	st.exempt[p.Filename][line] = true
}

// report records a join used as a key, once, unless a directive exempts it.
func (st *state) report(join, use ast.Expr) {
	if st.reported[join.Pos()] {
		return
	}
	jp := st.fset.Position(join.Pos())
	if st.exempt[jp.Filename][jp.Line] {
		return
	}
	st.reported[join.Pos()] = true
	msg := fmt.Sprintf("%s:%d: a key joined with a delimiter", st.relFile(join.Pos()), jp.Line)
	if up := st.fset.Position(use.Pos()); up.Filename != jp.Filename || up.Line != jp.Line {
		msg += fmt.Sprintf(" (used as a key at %s:%d)", st.relFile(use.Pos()), up.Line)
	}
	st.found = append(st.found, msg+"; use a struct or array key")
}

// isDelimiter reports whether e is a constant string that holds a delimiter
// character.
func (c *pkgCheck) isDelimiter(e ast.Expr) bool {
	tv, ok := c.info.Types[e]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return false
	}
	return strings.IndexFunc(constant.StringVal(tv.Value), isDelimiterRune) >= 0
}

// isDelimiterRune reports whether r is a control character other than tab,
// newline, carriage return and escape, which starts a terminal sequence.
func isDelimiterRune(r rune) bool {
	return r < 0x20 && r != '\t' && r != '\n' && r != '\r' && r != 0x1b
}

// isJoin reports whether e builds a string by joining parts with a delimiter.
func (c *pkgCheck) isJoin(e ast.Expr) bool {
	e = ast.Unparen(e)
	switch x := e.(type) {
	case *ast.BinaryExpr:
		if x.Op != token.ADD || !c.isString(x) {
			return false
		}
		for _, op := range flatten(x) {
			if c.isDelimiter(op) || c.isFoldedJoin(op) {
				return true
			}
		}
	case *ast.CallExpr:
		fn := c.calledFunc(x)
		if fn == nil || fn.Pkg() == nil {
			return false
		}
		switch fn.Pkg().Path() + "." + fn.Name() {
		case "strings.Join":
			return len(x.Args) == 2 && c.isDelimiter(x.Args[1])
		case "fmt.Sprintf":
			return len(x.Args) > 0 && c.isDelimiter(x.Args[0])
		}
	}
	return false
}

// isFoldedJoin reports whether e is a join passed through a function that
// keeps its delimiters.
func (c *pkgCheck) isFoldedJoin(e ast.Expr) bool {
	call, ok := ast.Unparen(e).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	fn := c.calledFunc(call)
	if fn == nil || fn.Pkg() == nil || fn.Pkg().Path() != "strings" {
		return false
	}
	switch fn.Name() {
	case "ToLower", "ToUpper", "TrimSpace":
		return c.isJoin(call.Args[0])
	}
	return false
}

func (c *pkgCheck) isString(e ast.Expr) bool {
	t := c.info.TypeOf(e)
	return t != nil && isStringType(t)
}

// flatten returns the operands of a + chain.
func flatten(e *ast.BinaryExpr) []ast.Expr {
	var out []ast.Expr
	var walk func(ast.Expr)
	walk = func(x ast.Expr) {
		x = ast.Unparen(x)
		if b, ok := x.(*ast.BinaryExpr); ok && b.Op == token.ADD {
			walk(b.X)
			walk(b.Y)
			return
		}
		out = append(out, x)
	}
	walk(e)
	return out
}

func (c *pkgCheck) calledFunc(call *ast.CallExpr) *types.Func {
	var id *ast.Ident
	switch f := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		id = f
	case *ast.SelectorExpr:
		id = f.Sel
	case *ast.IndexExpr:
		return c.calledFunc(&ast.CallExpr{Fun: f.X})
	default:
		return nil
	}
	fn, _ := c.info.Uses[id].(*types.Func)
	return fn
}

// object returns the variable or function an identifier, or a selector's
// name, refers to.
func (c *pkgCheck) object(e ast.Expr) types.Object {
	switch x := ast.Unparen(e).(type) {
	case *ast.Ident:
		if o := c.info.Uses[x]; o != nil {
			return o
		}
		return c.info.Defs[x]
	case *ast.SelectorExpr:
		return c.info.Uses[x.Sel]
	}
	return nil
}

// collect records the joins carried by each variable and returned by each
// function in f, and reports whether it learned anything new.
func (c *pkgCheck) collect(f *ast.File) bool {
	changed := false
	addVar := func(lhs ast.Expr, es []ast.Expr) {
		if len(es) == 0 {
			return
		}
		id, ok := ast.Unparen(lhs).(*ast.Ident)
		if !ok {
			return
		}
		o, ok := c.object(id).(*types.Var)
		if !ok {
			return
		}
		if c.vars[o] == nil {
			c.vars[o] = &joins{}
		}
		if c.vars[o].add(es...) {
			changed = true
		}
	}
	assign := func(lhs, rhs []ast.Expr) {
		switch {
		case len(lhs) == len(rhs):
			for i, r := range rhs {
				addVar(lhs[i], c.carried(r))
			}
		case len(rhs) == 1:
			if call, ok := ast.Unparen(rhs[0]).(*ast.CallExpr); ok {
				for i := range lhs {
					addVar(lhs[i], c.result(call, i))
				}
			}
		}
	}
	addFunc := func(results map[int]*joins, body *ast.BlockStmt) {
		ast.Inspect(body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.FuncLit:
				return false
			case *ast.ReturnStmt:
				for i, r := range x.Results {
					es := c.carried(r)
					if len(es) == 0 {
						continue
					}
					if results[i] == nil {
						results[i] = &joins{}
					}
					if results[i].add(es...) {
						changed = true
					}
				}
			}
			return true
		})
	}
	litFor := func(lhs, rhs []ast.Expr) {
		if len(lhs) != len(rhs) {
			return
		}
		for i, r := range rhs {
			lit, ok := ast.Unparen(r).(*ast.FuncLit)
			if !ok {
				continue
			}
			o := c.object(lhs[i])
			if o == nil {
				continue
			}
			if c.lits[o] == nil {
				c.lits[o] = map[int]*joins{}
			}
			addFunc(c.lits[o], lit.Body)
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			if x.Body == nil {
				return true
			}
			if fn, ok := c.info.Defs[x.Name].(*types.Func); ok {
				name := fn.FullName()
				if c.funcs[name] == nil {
					c.funcs[name] = map[int]*joins{}
				}
				addFunc(c.funcs[name], x.Body)
			}
		case *ast.AssignStmt:
			assign(x.Lhs, x.Rhs)
			litFor(x.Lhs, x.Rhs)
		case *ast.ValueSpec:
			lhs := make([]ast.Expr, len(x.Names))
			for i, n := range x.Names {
				lhs[i] = n
			}
			assign(lhs, x.Values)
			litFor(lhs, x.Values)
		}
		return true
	})
	return changed
}

// carried returns the joins an expression carries: itself, the joins
// assigned to the variable it names, or the joins the function it calls
// returns as its first result.
func (c *pkgCheck) carried(e ast.Expr) []ast.Expr {
	e = ast.Unparen(e)
	if c.isJoin(e) {
		return []ast.Expr{e}
	}
	switch x := e.(type) {
	case *ast.Ident:
		if o := c.object(x); o != nil && c.vars[o] != nil {
			return c.vars[o].list
		}
	case *ast.CallExpr:
		if tv, ok := c.info.Types[x.Fun]; ok && tv.IsType() && len(x.Args) == 1 {
			return c.carried(x.Args[0])
		}
		return c.result(x, 0)
	}
	return nil
}

// result returns the joins result i of a call carries.
func (c *pkgCheck) result(call *ast.CallExpr, i int) []ast.Expr {
	var results map[int]*joins
	if fn := c.calledFunc(call); fn != nil {
		results = c.funcs[fn.Origin().FullName()]
	} else if o := c.object(call.Fun); o != nil {
		results = c.lits[o]
	}
	if results[i] == nil {
		return nil
	}
	return results[i].list
}

// syncMapKeyed are the methods of sync.Map whose first argument is a key.
var syncMapKeyed = map[string]bool{
	"Load": true, "Store": true, "LoadOrStore": true, "LoadAndDelete": true,
	"Delete": true, "Swap": true, "CompareAndSwap": true, "CompareAndDelete": true,
}

// mapKeys returns the expressions n uses as keys of a map with string keys or
// of a sync.Map.
func (c *pkgCheck) mapKeys(n ast.Node) []ast.Expr {
	switch x := n.(type) {
	case *ast.IndexExpr:
		if c.stringKeyedMap(x.X) {
			return []ast.Expr{x.Index}
		}
	case *ast.CompositeLit:
		t := c.info.TypeOf(x)
		if t == nil {
			return nil
		}
		if m, ok := t.Underlying().(*types.Map); ok && isStringType(m.Key()) {
			var keys []ast.Expr
			for _, el := range x.Elts {
				if kv, ok := el.(*ast.KeyValueExpr); ok {
					keys = append(keys, kv.Key)
				}
			}
			return keys
		}
	case *ast.CallExpr:
		if id, ok := ast.Unparen(x.Fun).(*ast.Ident); ok && len(x.Args) == 2 {
			if b, ok := c.info.Uses[id].(*types.Builtin); ok && b.Name() == "delete" && c.stringKeyedMap(x.Args[0]) {
				return []ast.Expr{x.Args[1]}
			}
		}
		if fn := c.calledFunc(x); fn != nil && len(x.Args) > 0 && syncMapKeyed[fn.Name()] && strings.HasPrefix(fn.FullName(), "(*sync.Map).") {
			return []ast.Expr{x.Args[0]}
		}
	}
	return nil
}

func (c *pkgCheck) stringKeyedMap(e ast.Expr) bool {
	t := c.info.TypeOf(e)
	if t == nil {
		return false
	}
	m, ok := t.Underlying().(*types.Map)
	return ok && isStringType(m.Key())
}

func isStringType(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsString != 0
}
