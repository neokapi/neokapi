package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/scripts/internal/guardpackages"
)

// marker starts a comment that allows the reference on its line.
const marker = "//pathguard:"

// containsMarker, in a function's doc comment, says the function contains the
// paths it reaches itself, so it is not a wrapper and its callers need no
// comment. Its own sink calls still need theirs.
const containsMarker = marker + "contains"

// finding is one reported line and the package it is in.
type finding struct {
	pkg  string
	text string
}

// place is one line of one file, as the findings name it.
type place struct {
	file string
	line int
}

// ref is one reference the walk found: to a sink, or to a function of a
// covered package that may turn out to be a wrapper.
type ref struct {
	pkg   string
	at    place
	where string // the enclosing function, for the message
	in    string // the enclosing function's full name, "" at package level
	sink  string // the sink as package.Name; "" for a function of a covered package
	what  string
	value bool   // a function value rather than a call
	fn    string // the referenced function's full name, for a covered package's function
	fnPkg string
}

// mark is one //pathguard: comment.
type mark struct {
	pkg    string
	at     place // the comment's own line
	covers place // the line it allows
	reason string
	used   bool
}

// contains is one //pathguard:contains marker.
type contains struct {
	pkg    string
	at     place
	reason string
	used   bool
}

// wrapper is a function of a covered package that reaches a file by path
// through one of its parameters.
type wrapper struct{ name string }

// analysis collects references and comments across packages, then reports.
type analysis struct {
	fset     *token.FileSet
	first    bool
	rel      func(string) string
	sources  map[string][]string
	refs     []ref
	marks    map[place][]*mark
	wrappers map[string]wrapper
	checked  map[string]bool
	// malformed holds the comments that spell the marker with a space.
	malformed []finding
	// contained holds the //pathguard:contains markers.
	contained []*contains
}

func newAnalysis(fset *token.FileSet, first bool, rel func(string) string) *analysis {
	return &analysis{
		fset:     fset,
		first:    first,
		rel:      rel,
		sources:  map[string][]string{},
		marks:    map[place][]*mark{},
		wrappers: map[string]wrapper{},
		checked:  map[string]bool{},
	}
}

// parseFiles parses, with comments, the files the compiler selected for a
// package.
func (a *analysis) parseFiles(p guardpackages.Package) ([]*ast.File, error) {
	files := make([]*ast.File, 0, len(p.CompiledGoFiles))
	for _, name := range p.CompiledGoFiles {
		path := name
		if !filepath.IsAbs(path) {
			path = filepath.Join(p.Dir, name)
		}
		f, err := a.parse(path, nil)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, nil
}

// parse parses one file from src, or from disk when src is nil.
func (a *analysis) parse(path string, src []byte) (*ast.File, error) {
	const mode = parser.ParseComments | parser.SkipObjectResolution
	if src == nil {
		return parser.ParseFile(a.fset, path, nil, mode)
	}
	a.sources[path] = strings.Split(string(src), "\n")
	return parser.ParseFile(a.fset, path, src, mode)
}

// plantFile returns a file in package pkg, in directory dir, that opens a file
// by path, for the self-test to prove the package is checked.
func (a *analysis) plantFile(dir, pkg string) (*ast.File, string, error) {
	name := filepath.Join(dir, "zz_pathguard_plant.go")
	src := "package " + pkg + "\n\nimport pathguardplantos \"os\"\n\nfunc _() { _, _ = pathguardplantos.Open(\"pathguard-plant\") }\n"
	f, err := a.parse(name, []byte(src))
	return f, name, err
}

// line returns one line of a file, read from disk the first time.
func (a *analysis) line(file string, n int) (string, error) {
	lines, ok := a.sources[file]
	if !ok {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		lines = strings.Split(string(data), "\n")
		a.sources[file] = lines
	}
	if n < 1 || n > len(lines) {
		return "", fmt.Errorf("%s: no line %d", file, n)
	}
	return lines[n-1], nil
}

// addPackage type-checks one package and records its references, its
// comments and its wrappers.
func (a *analysis) addPackage(imp types.Importer, path string, files []*ast.File) error {
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Uses:  map[*ast.Ident]types.Object{},
		Defs:  map[*ast.Ident]types.Object{},
	}
	var typeErrs []error
	conf := types.Config{Importer: imp, Error: func(err error) { typeErrs = append(typeErrs, err) }}
	if _, err := conf.Check(path, a.fset, files, info); err != nil && len(typeErrs) > 0 {
		return typeErrs[0]
	}
	a.checked[path] = true
	for _, f := range files {
		name := a.fset.Position(f.Pos()).Filename
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		if err := a.addFile(info, path, f); err != nil {
			return err
		}
	}
	return nil
}

// addFile records one file's comments, then its references.
func (a *analysis) addFile(info *types.Info, pkg string, f *ast.File) error {
	local := map[place]*mark{}
	// helpers maps each function whose doc comment holds a contains marker
	// to that marker.
	helpers := map[*ast.FuncDecl]*contains{}
	inDoc := map[*ast.Comment]bool{}
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Doc == nil {
			continue
		}
		for _, c := range fd.Doc.List {
			if reason, ok := containsReason(c.Text); ok {
				pos := a.fset.Position(c.Slash)
				m := &contains{pkg: pkg, at: place{a.rel(pos.Filename), pos.Line}, reason: reason}
				a.contained = append(a.contained, m)
				helpers[fd] = m
				inDoc[c] = true
			}
		}
	}
	for _, g := range f.Comments {
		for _, c := range g.List {
			pos := a.fset.Position(c.Slash)
			if _, ok := containsReason(c.Text); ok {
				if !inDoc[c] {
					a.malformed = append(a.malformed, finding{pkg: pkg, text: fmt.Sprintf("%s:%d: %s belongs in the doc comment of the function it describes", a.rel(pos.Filename), pos.Line, containsMarker)})
				}
				continue
			}
			if strings.HasPrefix(c.Text, "// pathguard:") {
				// gofmt turns the marker into this inside a doc comment.
				a.malformed = append(a.malformed, finding{pkg: pkg, text: fmt.Sprintf("%s:%d: write the comment as %s with no space, on the line of the call when it would join a doc comment", a.rel(pos.Filename), pos.Line, marker)})
				continue
			}
			text, ok := strings.CutPrefix(c.Text, marker)
			if !ok {
				continue
			}
			src, err := a.line(pos.Filename, pos.Line)
			if err != nil {
				return err
			}
			at := place{a.rel(pos.Filename), pos.Line}
			covers := at
			if pos.Column <= len(src) && strings.TrimSpace(src[:pos.Column-1]) == "" {
				covers.line++
			}
			m := &mark{pkg: pkg, at: at, covers: covers, reason: strings.TrimSpace(text)}
			a.marks[covers] = append(a.marks[covers], m)
			if m.reason != "" {
				local[covers] = m
			}
		}
	}

	calls := map[*ast.Ident]*ast.CallExpr{}
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if id := calleeIdent(call.Fun); id != nil {
				calls[id] = call
			}
		}
		return true
	})
	var decls []*ast.FuncDecl
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Body != nil {
			decls = append(decls, fd)
		}
	}
	enclosing := func(pos token.Pos) *ast.FuncDecl {
		for _, fd := range decls {
			if fd.Pos() <= pos && pos < fd.End() {
				return fd
			}
		}
		return nil
	}

	ast.Inspect(f, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		obj := info.Uses[id]
		if obj == nil {
			return true
		}
		call := calls[id]
		var (
			s     sink
			sname string
			found bool
		)
		switch obj := obj.(type) {
		case *types.Func:
			fn := obj.Origin()
			sig, _ := fn.Type().(*types.Signature)
			if fn.Pkg() == nil || sig == nil {
				return true
			}
			if sig.Recv() == nil {
				s, found = sinks[fn.Pkg().Path()][fn.Name()]
				sname = fn.Pkg().Name() + "." + fn.Name()
			}
			if !found {
				if covered(fn.Pkg().Path()) && sig.Params().Len() > 0 {
					r := a.newRef(pkg, id, enclosing(id.Pos()), info)
					r.fn, r.fnPkg, r.value = fn.FullName(), fn.Pkg().Path(), call == nil
					a.refs = append(a.refs, r)
				}
				return true
			}
		case *types.TypeName:
			if call == nil || obj.Pkg() == nil || obj.Pkg().Path() != "net/http" || obj.Name() != "Dir" {
				return true
			}
			s, sname = httpDir, "http.Dir"
		default:
			return true
		}
		fd := enclosing(id.Pos())
		r := a.newRef(pkg, id, fd, info)
		r.sink, r.what, r.value = sname, s.what, call == nil
		a.refs = append(a.refs, r)
		// An allowed call that takes one of its function's parameters as the
		// path makes the function a wrapper.
		if call != nil && fd != nil && takesParam(info, fd, call, s.pathArgs) {
			if h := helpers[fd]; h != nil {
				h.used = true
				return true
			}
			if local[r.at] == nil {
				return true
			}
			if def, ok := info.Defs[fd.Name].(*types.Func); ok {
				a.wrappers[def.FullName()] = wrapper{name: displayName(def)}
			}
		}
		return true
	})
	return nil
}

// containsReason reports whether a comment is a contains marker, and returns
// its reason.
func containsReason(text string) (string, bool) {
	rest, ok := strings.CutPrefix(text, containsMarker)
	if !ok || (rest != "" && rest[0] != ' ' && rest[0] != '\t') {
		return "", false
	}
	return strings.TrimSpace(rest), true
}

// newRef starts a reference at an identifier.
func (a *analysis) newRef(pkg string, id *ast.Ident, fd *ast.FuncDecl, info *types.Info) ref {
	pos := a.fset.Position(id.Pos())
	r := ref{pkg: pkg, at: place{a.rel(pos.Filename), pos.Line}, where: "at package level"}
	if fd != nil {
		r.where = "in " + fd.Name.Name
		if def, ok := info.Defs[fd.Name].(*types.Func); ok {
			r.in = def.FullName()
		}
	}
	return r
}

// calleeIdent returns the identifier naming what a call calls, or nil.
func calleeIdent(fun ast.Expr) *ast.Ident {
	fun = ast.Unparen(fun)
	switch x := fun.(type) {
	case *ast.IndexExpr:
		fun = x.X
	case *ast.IndexListExpr:
		fun = x.X
	}
	switch x := fun.(type) {
	case *ast.Ident:
		return x
	case *ast.SelectorExpr:
		return x.Sel
	}
	return nil
}

// takesParam reports whether one of a call's path arguments is a parameter of
// the function declaration, possibly converted to a string type.
func takesParam(info *types.Info, fd *ast.FuncDecl, call *ast.CallExpr, args []int) bool {
	params := map[types.Object]bool{}
	for _, field := range fd.Type.Params.List {
		for _, name := range field.Names {
			if obj := info.Defs[name]; obj != nil {
				params[obj] = true
			}
		}
	}
	for _, i := range args {
		if i >= len(call.Args) {
			continue
		}
		arg := ast.Unparen(call.Args[i])
		if conv, ok := arg.(*ast.CallExpr); ok && len(conv.Args) == 1 && info.Types[conv.Fun].IsType() {
			arg = ast.Unparen(conv.Args[0])
		}
		if id, ok := arg.(*ast.Ident); ok && params[info.Uses[id]] {
			return true
		}
	}
	return false
}

// displayName names a function as package.Name, or package.Type.Method.
func displayName(fn *types.Func) string {
	name := fn.Pkg().Name() + "."
	if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil {
		t := sig.Recv().Type()
		if p, ok := t.(*types.Pointer); ok {
			t = p.Elem()
		}
		if n, ok := t.(*types.Named); ok {
			name += n.Obj().Name() + "."
		}
	}
	return name + fn.Name()
}

// findings reports each reference no comment allows, each comment with no
// reason, and each comment on a line with no reported reference.
func (a *analysis) findings() []finding {
	found := slices.Clone(a.malformed)
	// uncertain holds the lines that refer to a function of a package this run
	// did not check, which may be a wrapper; a later configuration cannot tell
	// that a comment there is stale.
	uncertain := map[place]bool{}
	for _, r := range a.refs {
		text := ""
		switch {
		case r.sink != "" && r.value:
			text = r.sink + ", taken as a value, " + r.what
		case r.sink != "":
			text = r.sink + " " + r.what
		default:
			if !a.checked[r.fnPkg] {
				uncertain[r.at] = true
			}
			w, ok := a.wrappers[r.fn]
			if !ok || r.fn == r.in {
				continue
			}
			text = "reaches a file by path through " + w.name
			if r.value {
				text = w.name + ", taken as a value, reaches a file by path"
			}
		}
		if a.allow(r.at) {
			continue
		}
		found = append(found, finding{pkg: r.pkg, text: fmt.Sprintf("%s:%d: %s (%s)", r.at.file, r.at.line, text, r.where)})
	}
	for _, ms := range a.marks {
		for _, m := range ms {
			switch {
			case m.reason == "":
				found = append(found, finding{pkg: m.pkg, text: fmt.Sprintf("%s:%d: %s comment gives no reason", m.at.file, m.at.line, marker)})
			case !m.used && (a.first || !uncertain[m.covers]):
				found = append(found, finding{pkg: m.pkg, text: fmt.Sprintf("%s:%d: %s comment allows line %d, which reaches no file by path; remove it", m.at.file, m.at.line, marker, m.covers.line)})
			}
		}
	}
	for _, m := range a.contained {
		switch {
		case m.reason == "":
			found = append(found, finding{pkg: m.pkg, text: fmt.Sprintf("%s:%d: %s marker gives no reason", m.at.file, m.at.line, containsMarker)})
		case !m.used:
			found = append(found, finding{pkg: m.pkg, text: fmt.Sprintf("%s:%d: %s marker on a function that reaches no file by a path parameter; remove it", m.at.file, m.at.line, containsMarker)})
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].text < found[j].text })
	return found
}

// allow reports whether a comment with a reason covers a line, and marks the
// comments that do as used.
func (a *analysis) allow(at place) bool {
	ok := false
	for _, m := range a.marks[at] {
		if m.reason != "" {
			m.used, ok = true, true
		}
	}
	return ok
}
