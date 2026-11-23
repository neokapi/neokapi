package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/scripts/internal/guardpackages"
)

// fixtures are packages the check must report, or must pass, each with the
// number of findings it expects.
var fixtures = []struct {
	name string
	want int
	src  string
}{
	{"a joined map index", 1, `package f
func f(m map[string]int, a, b string) int { return m[a+"\x00"+b] }`},
	{"the delimiter anywhere in a chain", 2, `package f
func f(m map[string]bool, a, b, c string) {
	m["x"+a+"\x00"+b] = true
	m[a+b+c+"\x00"] = true
}`},
	{"a named delimiter constant", 1, `package f
const sep = "\x1f"
func f(m map[string]bool, a, b string) bool { return m[a+sep+b] }`},
	{"strings.Join with a delimiter", 1, `package f
import "strings"
func f(m map[string]bool, parts []string) { m[strings.Join(parts, "\x00")] = true }`},
	{"fmt.Sprintf with a delimiter", 1, `package f
import "fmt"
func f(m map[string]bool, a, b string) { m[fmt.Sprintf("%s\x00%s", a, b)] = true }`},
	{"a map literal key", 1, `package f
func f(a, b string) map[string]int { return map[string]int{a + "\x00" + b: 1} }`},
	{"delete with a joined key", 1, `package f
func f(m map[string]int, a, b string) { delete(m, a+"\x00"+b) }`},
	{"a set lookup through a local variable", 1, `package f
func f(seen map[string]struct{}, a, b string) bool {
	k := a + "\x00" + b
	if _, ok := seen[k]; ok {
		return true
	}
	seen[k] = struct{}{}
	return false
}`},
	{"a variable declared with var", 1, `package f
func f(m map[string]int, a, b string) int {
	var k = a + "\x00" + b
	return m[k]
}`},
	{"a key function", 1, `package f
func key(a, b string) string { return a + "\x00" + b }
func f(m map[string]int, a, b string) int { return m[key(a, b)] }`},
	{"a key method", 1, `package f
type ref struct{ a, b string }
func (r ref) key() string { return r.a + "\x00" + r.b }
func f(m map[string]int, r ref) int { return m[r.key()] }`},
	{"a key function literal", 1, `package f
func f(m map[string]int, a, b string) int {
	key := func(x, y string) string { return x + "\x00" + y }
	return m[key(a, b)]
}`},
	{"a key function returning a variable", 1, `package f
func key(a, b string) string {
	k := a + "\x00" + b
	return k
}
func f(m map[string]int) int { return m[key("a", "b")] }`},
	{"a struct field map", 1, `package f
type s struct{ m map[string]int }
func (x *s) f(a, b string) { x.m[a+"\x00"+b]++ }`},
	{"a named string key type", 1, `package f
type id string
func f(m map[id]int, a, b string) int { return m[id(a+"\x00"+b)] }`},
	{"a key function in another package", 1, `package f
import "fixture/dep"
func f(m map[string]int, r dep.Ref) int { return m[r.Key()] }`},
	{"a key among several results", 1, `package f
func at(a, b string) (string, int, error) { return a + "\x00" + b, 1, nil }
func f(m map[string]int, a, b string) int {
	k, _, _ := at(a, b)
	return m[k]
}`},
	{"a key built from a key function", 1, `package f
func inner(a, b string) string { return a + "\x00" + b }
func outer(a, b string) string { return inner(a, b) }
func f(m map[string]int) int { return m[outer("a", "b")] }`},
	{"a sync.Map key", 1, `package f
import "sync"
func f(m *sync.Map, a, b string) { m.LoadOrStore(a+"\x00"+b, true) }`},
	{"a delimiter inside a longer literal", 1, `package f
func f(m map[string]int, a, b string) int { return m[a+"\x00kind\x00"+b] }`},
	{"a lowered join in a chain", 1, `package f
import "strings"
func f(m map[string]int, keep []string, a string) int { return m[strings.ToLower(strings.Join(keep, "\x00"))+"|"+a] }`},
	{"a key appended to with +=", 1, `package f
func f(m map[string]int, a, b string) int {
	k := a
	k += "\x00" + b
	return m[k]
}`},
	{"a struct field assigned a join", 2, `package f
type ref struct{ key, name string }
func f(m map[string]int, a, b string) int {
	r := ref{key: a + "\x00" + b}
	var s ref
	s.name = b + "\x00" + a
	return m[r.key] + m[s.name]
}`},
	{"a field of a struct in another package", 1, `package f
import "fixture/dep"
func f(m map[string]int, a, b string) int { return m[dep.Of(a, b).ID] }`},
	{"a log line", 0, `package f
import "log"
func f(a, b string) { log.Printf("%s", a+"\x00"+b) }`},
	{"a hash input written to a hasher", 0, `package f
import (
	"crypto/sha256"
	"hash"
)
func f(h hash.Hash, a, b string) [32]byte {
	h.Write([]byte(a + "\x00" + b))
	seed := a + "\x00" + b
	return sha256.Sum256([]byte(seed))
}`},
	{"a hashed key", 0, `package f
import (
	"crypto/sha256"
	"encoding/hex"
)
func hashKey(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }
func f(m map[string]bool, a, b string) { m[hashKey(a+"\x00"+b)] = true }`},
	{"a path, a qualified name and a newline join", 0, `package f
import "strings"
func f(m map[string]bool, a, b string, lines []string) {
	m[a+"/"+b] = true
	m[a+":"+b] = true
	m[strings.Join(lines, "\n")] = true
}`},
	{"a pipe between two parts", 3, `package f
import (
	"fmt"
	"strings"
)
func f(m map[string]bool, a, b string, parts []string, n int) {
	m[a+"|"+b] = true
	m[strings.Join(parts, "|")] = true
	m[fmt.Sprintf("%s|%d", a, n)] = true
}`},
	{"a pipe on one value", 0, `package f
func f(m map[string]bool, a string) { m["rule|"+a] = true }`},
	{"fmt.Sprint with a delimiter", 1, `package f
import "fmt"
func f(m map[string]bool, a, b string) { m[fmt.Sprint(a, "\x00", b)] = true }`},
	{"a builder a delimiter is written to", 1, `package f
import "strings"
func key(a, b string) string {
	var sb strings.Builder
	sb.WriteString(a)
	sb.WriteByte(0)
	sb.WriteString(b)
	return sb.String()
}
func f(m map[string]bool, a, b string) { m[key(a, b)] = true }`},
	{"a decoder writing a control character in a loop", 0, `package f
import "strings"
func decode(in string) string {
	var sb strings.Builder
	for _, r := range in {
		if r == 'b' {
			sb.WriteByte('\b')
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}
func f(m map[string]bool, a string) { m[decode(a)] = true }`},
	{"a parameter used as a key", 1, `package f
type cache struct{ m map[string]int }
func (c *cache) get(key string) int { return c.m[key] }
func f(c *cache, a, b string) int { return c.get(a + "\x00" + b) }`},
	{"a singleflight key", 1, `package f
import "golang.org/x/sync/singleflight"
func f(g *singleflight.Group, a, b string) {
	_, _, _ = g.Do(a+"\x00"+b, func() (any, error) { return nil, nil })
}`},
	{"a joined field of a struct key", 1, `package f
type key struct{ ref, locale string }
func f(m map[key]int, a, b, l string) int { return m[key{ref: a + "\x00" + b, locale: l}] }`},
	{"a struct or array key", 0, `package f
func f(m map[struct{ a, b string }]int, n map[[2]string]int, a, b string) int {
	return m[struct{ a, b string }{a, b}] + n[[2]string{a, b}]
}`},
	{"a slice index", 0, `package f
func f(s []string, i int, a string) string { s[i] = a + "\x00"; return s[i] }`},
	{"a constant prefix on one value", 0, `package f
import "strconv"
func f(m map[string]int, k int, a string) int { return m["\x00"+strconv.Itoa(k)] + m["kind\x00"+a] }`},
	{"an escape sequence in a key", 0, `package f
func f(m map[string]int, a string) int { return m["\x1b[1m"+a] }`},
	{"a persisted identity with a directive", 0, `package f
func f(m map[string]int, a, b string) int {
	//keyguard:persisted the documents table's config_key column
	k := a + "\x00" + b
	return m[k] + m[b+"\x00"+a] //keyguard:persisted the cache index
}`},
	{"a directive without a reason", 2, `package f
func f(m map[string]int, a, b string) int {
	//keyguard:persisted
	return m[a+"\x00"+b]
}`},
	{"a directive on no join", 1, `package f
//keyguard:persisted nothing here
func f(m map[string]int, a string) int { return m[a] }`},
}

// fixtureDeps holds, by fixture name, a package checked first as fixture/dep.
var fixtureDeps = map[string]string{
	"a field of a struct in another package": `package dep
type Ref struct{ ID string }
func Of(a, b string) Ref { return Ref{ID: a + "\x00" + b} }`,
	"a key function in another package": `package dep
type Ref struct{ A, B string }
func (r Ref) Key() string { return r.A + "\x00" + r.B }`,
}

// runSelfTest checks every fixture and fails when one reports other than it
// expects.
func runSelfTest() error {
	pkgs, err := guardpackages.List("strings", "strconv", "fmt", "log", "hash", "crypto/sha256", "encoding/hex", "sync", "golang.org/x/sync/singleflight")
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	std := pkgs.Importer(fset)
	var failures []string
	for _, fx := range fixtures {
		st := newState(fset, func(pos token.Pos) string { return filepath.Base(fset.Position(pos).Filename) })
		imp := fixtureImporter{std: std, pkgs: map[string]*types.Package{}}
		if dep, ok := fixtureDeps[fx.name]; ok {
			d, err := parser.ParseFile(fset, "dep.go", dep, parser.ParseComments)
			if err != nil {
				return fmt.Errorf("%s: %w", fx.name, err)
			}
			pkg, err := st.checkPackage(imp, "fixture/dep", []*ast.File{d})
			if err != nil {
				return fmt.Errorf("%s: %w", fx.name, err)
			}
			imp.pkgs["fixture/dep"] = pkg
		}
		f, err := parser.ParseFile(fset, "fixture.go", fx.src, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("%s: %w", fx.name, err)
		}
		if _, err := st.checkPackage(imp, "fixture", []*ast.File{f}); err != nil {
			return fmt.Errorf("%s: %w", fx.name, err)
		}
		found := st.found
		if len(found) != fx.want {
			failures = append(failures, fmt.Sprintf("%s: want %d findings, got %d: %s", fx.name, fx.want, len(found), strings.Join(found, "; ")))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("self-test failed:\n  %s", strings.Join(failures, "\n  "))
	}
	fmt.Println("keyguard: self-test passed")
	return nil
}

// fixtureImporter resolves a fixture's own dependency package from source and
// every other import from export data.
type fixtureImporter struct {
	std  types.Importer
	pkgs map[string]*types.Package
}

func (i fixtureImporter) Import(path string) (*types.Package, error) {
	if p, ok := i.pkgs[path]; ok {
		return p, nil
	}
	return i.std.Import(path)
}
