package golang

import (
	"go/ast"
	"go/token"
	"path"
	"strconv"
)

// subject is what a comment sits on: the structural path of the declaration,
// and whether the comment is that declaration's documentation.
type subject struct {
	path string
	doc  bool
}

// subjectMap resolves a comment group to its subject.
type subjectMap struct {
	attached map[*ast.CommentGroup]subject
	// decls lists each top-level declaration's range, in file order, so a
	// comment inside a body resolves to the declaration around it.
	decls []declRange
}

type declRange struct {
	pos, end token.Pos
	path     string
}

// of returns the subject of g: the declaration it is attached to, else the
// declaration it sits inside, else a bare comment.
func (m subjectMap) of(g *ast.CommentGroup) subject {
	if s, ok := m.attached[g]; ok {
		return s
	}
	for _, d := range m.decls {
		if g.Pos() >= d.pos && g.End() <= d.end {
			return subject{path: d.path + "/comment"}
		}
	}
	return subject{path: "comment"}
}

// subjectsOf walks the file's declarations and records the subject of every
// comment group the parser attached to one.
func subjectsOf(f *ast.File) subjectMap {
	m := subjectMap{attached: map[*ast.CommentGroup]subject{}}
	attach := func(g *ast.CommentGroup, p string, doc bool) {
		if g != nil {
			m.attached[g] = subject{path: p, doc: doc}
		}
	}
	attach(f.Doc, "package", true)

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			p := "func/" + d.Name.Name
			if d.Recv != nil && len(d.Recv.List) > 0 {
				p = "func/" + receiverName(d.Recv.List[0].Type) + "." + d.Name.Name
			}
			attach(d.Doc, p, true)
			m.decls = append(m.decls, declRange{pos: d.Pos(), end: d.End(), path: p})
		case *ast.GenDecl:
			tok := d.Tok.String()
			p := tok
			if len(d.Specs) > 0 {
				p = tok + "/" + specName(d.Specs[0])
			}
			attach(d.Doc, p, true)
			for _, spec := range d.Specs {
				sp := tok + "/" + specName(spec)
				switch s := spec.(type) {
				case *ast.ValueSpec:
					attach(s.Doc, sp, true)
					attach(s.Comment, sp, false)
				case *ast.TypeSpec:
					attach(s.Doc, sp, true)
					attach(s.Comment, sp, false)
					fields(s.Type, sp, attach)
				case *ast.ImportSpec:
					attach(s.Doc, sp, true)
					attach(s.Comment, sp, false)
				}
			}
			m.decls = append(m.decls, declRange{pos: d.Pos(), end: d.End(), path: p})
		}
	}
	return m
}

// fields attaches the documentation of struct fields and interface methods,
// descending into struct types declared inline.
func fields(expr ast.Expr, parent string, attach func(*ast.CommentGroup, string, bool)) {
	var list *ast.FieldList
	switch t := expr.(type) {
	case *ast.StructType:
		list = t.Fields
	case *ast.InterfaceType:
		list = t.Methods
	default:
		return
	}
	if list == nil {
		return
	}
	for _, field := range list.List {
		name := typeName(field.Type)
		if len(field.Names) > 0 {
			name = field.Names[0].Name
		}
		p := parent + "/" + name
		attach(field.Doc, p, true)
		attach(field.Comment, p, false)
		fields(field.Type, p, attach)
	}
}

// specName names a declaration spec: the first name it declares, or an import's
// local name.
func specName(spec ast.Spec) string {
	switch s := spec.(type) {
	case *ast.ValueSpec:
		if len(s.Names) > 0 {
			return s.Names[0].Name
		}
	case *ast.TypeSpec:
		return s.Name.Name
	case *ast.ImportSpec:
		if s.Name != nil {
			return s.Name.Name
		}
		if p, err := strconv.Unquote(s.Path.Value); err == nil {
			return path.Base(p)
		}
	}
	return ""
}

// receiverName is a method receiver's type name, without a pointer or type
// parameters.
func receiverName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return receiverName(t.X)
	case *ast.IndexExpr:
		return receiverName(t.X)
	case *ast.IndexListExpr:
		return receiverName(t.X)
	case *ast.ParenExpr:
		return receiverName(t.X)
	}
	return typeName(expr)
}

// typeName names an embedded field by the identifier it embeds.
func typeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.StarExpr:
		return typeName(t.X)
	case *ast.IndexExpr:
		return typeName(t.X)
	case *ast.IndexListExpr:
		return typeName(t.X)
	}
	return "embedded"
}
