// Command fieldguard counts the uses of the block fields that the peer-edition
// flip removes from the content model.
//
// A block stores its content in three fields: Source holds the edition the
// block was read in, SourceStatus holds that edition's status, and Targets
// holds every other edition as a *model.Target. WP14 of the edit model
// (docs/internals/edit-model.md, section 6.4) replaces them with peer editions.
// Every other package reaches content through the accessors in
// core/model/edition.go (Edition, SetEdition, SetEditionStatus, RemoveEdition,
// Editions, EachEdition, Authoritative), so the flip can change core/model
// alone. This check finds what still reaches past them.
//
// It type-checks every package, with its tests, of every module in go.work and
// of every other module in the tree (the plugins under plugins/ build on their
// own), and reports each identifier that resolves to:
//
//   - the field Block.Source, Block.Targets or Block.SourceStatus, whether in a
//     selector (b.Source), a composite literal's key (model.Block{Source: r})
//     or a selector that reaches the field through an embedded Block;
//   - the type model.Target, which the flip replaces with model.Edition; or
//   - a function or method of core/model whose parameters or results carry a
//     model.Target (model.NewTarget, Block.Target, Block.SetTarget and the
//     variant forms).
//
// Matching is by the object an identifier resolves to, so a field of another
// type that shares a name (a proto message's Source, an AltTranslation's
// Source) is not reported. core/model itself is exempt, and so is each package
// listed in allowed, with the reason it keeps the fields.
//
// Packages load under each build configuration in configs in turn. The first
// covers the host with every test tag; each later one (the model tags,
// js/wasm, windows, linux without cgo) loads only the directories that still
// hold a file no earlier configuration built, and a Go file no configuration
// builds is listed as unchecked.
//
// The output counts the uses per package, non-test and test separately,
// grouped by module, with the totals and a count per kind. The check fails
// while a use remains outside core/model and the allowed packages. -report
// prints the same inventory and exits 0, which is how make fieldguard runs it
// until the flip turns the gate on.
//
// Run from the repository root:
//
//	go run ./scripts/fieldguard            # fail while a use remains
//	go run ./scripts/fieldguard -report    # print the inventory and exit 0
//	go run ./scripts/fieldguard -v         # list every use as well
//	go run ./scripts/fieldguard -format markdown
//	go run ./scripts/fieldguard -self-test # prove the check on fixtures
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/token"
	"go/types"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// modelPath is the package that declares the fields.
const modelPath = "github.com/neokapi/neokapi/core/model"

// exemptDir is the directory of modelPath, whose own code and tests are where
// the fields live.
const exemptDir = "core/model"

// fields are the Block fields the flip removes.
var fields = map[string]bool{"Source": true, "Targets": true, "SourceStatus": true}

// allowed lists the packages, by directory, that keep using the fields, each
// with the reason.
var allowed = map[string]string{
	"core/plugin/protoconvert": "the plugin wire: BlockMessage keeps its source and targets field numbers, and the mapping reads source as the first native edition and targets as the rest (edit-model 6.4)",
}

// config is one build configuration packages load under.
type config struct {
	name string
	env  []string
	tags string
}

// testTags are the build tags that hold tests: the parity, integration,
// acceptance and end-to-end suites.
const testTags = "fts5,parity,integration,acceptance,e2e"

// configs are the build configurations, in order. The first loads every
// package; each later one loads only the directories that still hold a file
// no earlier one built.
var configs = []config{
	{name: "host", tags: testTags},
	{name: "host with the model tags", tags: "fts5,onnx,satmodel,pdfium_experimental,storage_oneconn,notelemetry"},
	{name: "js/wasm", env: []string{"GOOS=js", "GOARCH=wasm"}, tags: testTags},
	{name: "windows", env: []string{"GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0"}, tags: testTags},
	{name: "linux without cgo", env: []string{"GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0"}, tags: testTags},
}

func main() {
	selfTest := flag.Bool("self-test", false, "prove the check on fixtures")
	report := flag.Bool("report", false, "print the inventory and exit 0 while uses remain")
	verbose := flag.Bool("v", false, "list every use")
	format := flag.String("format", "text", "output format: text, markdown or json")
	flag.Parse()

	root, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.work")); err != nil {
		fail(errors.New("no go.work here: run from the repository root"))
	}
	if *selfTest {
		if err := runSelfTest(root); err != nil {
			fail(err)
		}
		return
	}
	inv, err := check(root)
	if err != nil {
		fail(err)
	}
	switch *format {
	case "text":
		inv.writeText(os.Stdout, *verbose)
	case "markdown":
		inv.writeMarkdown(os.Stdout, *verbose)
	case "json":
		if err := inv.writeJSON(os.Stdout); err != nil {
			fail(err)
		}
	default:
		fail(fmt.Errorf("unknown format %q: use text, markdown or json", *format))
	}
	for _, w := range inv.warnings {
		fmt.Fprintln(os.Stderr, "fieldguard: warning:", w)
	}
	if n := inv.remaining(); n > 0 && !*report {
		fmt.Fprintf(os.Stderr, "fieldguard: %d uses of Block.Source, Block.Targets, Block.SourceStatus and model.Target remain outside core/model and the allowed packages.\n", n)
		fmt.Fprintln(os.Stderr, "Read and write a block's editions through the accessors in core/model/edition.go, or list the package in scripts/fieldguard with the reason it keeps the fields.")
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "fieldguard:", err)
	os.Exit(2)
}

// module is one Go module the check loads.
type module struct {
	Path string `json:"path"`
	Dir  string `json:"dir"`  // relative to the repository root, slash-separated
	Work bool   `json:"work"` // listed in go.work
}

// modules returns the modules listed in go.work, in its order, then every
// other module in the tree.
func modules(root string) ([]module, error) {
	cmd := exec.CommandContext(context.Background(), "go", "work", "edit", "-json")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go work edit -json: %w", err)
	}
	var work struct{ Use []struct{ DiskPath string } }
	if err := json.Unmarshal(out, &work); err != nil {
		return nil, fmt.Errorf("go work edit -json: %w", err)
	}
	var mods []module
	seen := map[string]bool{}
	for _, u := range work.Use {
		dir := filepath.ToSlash(filepath.Clean(u.DiskPath))
		p, err := modulePath(filepath.Join(root, dir, "go.mod"))
		if err != nil {
			return nil, err
		}
		mods = append(mods, module{Path: p, Dir: dir, Work: true})
		seen[dir] = true
	}
	var extra []module
	err = filepath.WalkDir(root, func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if file != root && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "go.mod" {
			return nil
		}
		rel, err := filepath.Rel(root, filepath.Dir(file))
		if err != nil {
			return err
		}
		dir := filepath.ToSlash(rel)
		if seen[dir] {
			return nil
		}
		p, err := modulePath(file)
		if err != nil {
			return err
		}
		extra = append(extra, module{Path: p, Dir: dir})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(extra, func(i, j int) bool { return extra[i].Dir < extra[j].Dir })
	return append(mods, extra...), nil
}

// modulePath reads the module path from a go.mod file.
func modulePath(file string) (string, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	for line := range strings.Lines(string(b)) {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`), nil
		}
	}
	return "", fmt.Errorf("%s names no module", file)
}

// moduleOf returns the module whose directory holds dir: the one with the
// longest directory that is dir or a parent of it.
func moduleOf(mods []module, dir string) module {
	var best module
	bestLen := -1
	for _, m := range mods {
		l := len(m.Dir)
		if m.Dir == "." {
			l = 0
		} else if dir != m.Dir && !strings.HasPrefix(dir, m.Dir+"/") {
			continue
		}
		if l > bestLen {
			best, bestLen = m, l
		}
	}
	return best
}

// load type-checks the packages patterns name in module m, with their tests,
// under cfg.
func load(root string, m module, cfg config, patterns []string) ([]*packages.Package, error) {
	env := append(os.Environ(), cfg.env...)
	if !m.Work {
		env = append(env, "GOWORK=off")
	}
	pc := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Context:    context.Background(),
		Dir:        filepath.Join(root, m.Dir),
		Env:        env,
		BuildFlags: []string{"-tags", cfg.tags},
		Tests:      true,
	}
	return packages.Load(pc, patterns...)
}

// benign reports a package error that says only that a configuration builds
// none of the package's files, which a later configuration covers.
func benign(e packages.Error) bool {
	return strings.Contains(e.Msg, "build constraints exclude all Go files")
}

// embedOnly reports a package error about a //go:embed pattern that matches
// no file: build output such as a frontend bundle that this checkout has not
// produced. The package still type-checks, so it is a warning.
func embedOnly(e packages.Error) bool {
	return e.Kind == packages.ListError && strings.Contains(e.Msg, "no matching files found")
}

// check loads every module under every configuration and returns the
// inventory of uses.
func check(root string) (*inventory, error) {
	mods, err := modules(root)
	if err != nil {
		return nil, err
	}
	// all is every Go file in the tree; a file no configuration builds is
	// reported as unchecked, whether a build constraint excludes it everywhere
	// or no pattern reaches its directory.
	all, err := goFiles(root)
	if err != nil {
		return nil, err
	}
	inv := newInventory(root, mods)
	built := map[string]bool{} // absolute .go files a configuration type-checked
	var fatal []string
	for ci, cfg := range configs {
		// A later configuration loads whole packages to reach the files no
		// earlier one built; a problem in a file already built was reported
		// then, or is one this configuration has no reason to build.
		before := maps.Clone(built)
		for _, m := range mods {
			patterns := []string{"./..."}
			if ci > 0 {
				patterns = pending(root, mods, m, all, built)
				if len(patterns) == 0 {
					continue
				}
			}
			// A problem in the first configuration of a go.work module fails
			// the check: a package that does not type-check can hide a use.
			// Anywhere else it is a warning.
			report := func(msg string, warnOnly bool) {
				if ci == 0 && m.Work && !warnOnly {
					fatal = append(fatal, msg)
				} else {
					inv.warnings = append(inv.warnings, msg)
				}
			}
			pkgs, err := load(root, m, cfg, patterns)
			if err != nil {
				msg := fmt.Sprintf("%s (%s): %v", m.Dir, cfg.name, err)
				if ci == 0 && m.Work {
					return nil, errors.New(msg)
				}
				inv.warnings = append(inv.warnings, msg)
				continue
			}
			for _, p := range pkgs {
				// fresh holds the repository files this configuration builds
				// first; a test main's generated file sits outside it.
				fresh := map[string]bool{}
				for _, f := range p.GoFiles {
					if !before[f] && within(root, f) {
						fresh[f] = true
					}
					built[f] = true
				}
				for _, e := range p.Errors {
					if benign(e) {
						continue
					}
					if file, _, _ := strings.Cut(e.Pos, ":"); ci > 0 && !fresh[file] {
						continue
					}
					report(fmt.Sprintf("%s (%s): %s", p.PkgPath, cfg.name, e), embedOnly(e))
				}
				if p.IllTyped && len(p.Errors) == 0 && len(fresh) > 0 {
					report(fmt.Sprintf("%s (%s): a package it imports did not build, so a use in it could be missed", p.PkgPath, cfg.name), false)
				}
				if p.TypesInfo != nil {
					inv.packages++
					scan(p.Fset, p.TypesInfo, inv.add)
				}
			}
		}
		added := 0
		for f := range built {
			if !before[f] && within(root, f) {
				added++
			}
		}
		inv.configs = append(inv.configs, configCount{Name: cfg.name, Files: added})
	}
	if len(fatal) > 0 {
		slices.Sort(fatal)
		fatal = slices.Compact(fatal)
		return nil, fmt.Errorf("these packages did not type-check, so a use in them could be missed:\n  %s", strings.Join(fatal, "\n  "))
	}
	for _, f := range all {
		if built[f] {
			continue
		}
		rel, err := filepath.Rel(root, f)
		if err != nil {
			continue
		}
		inv.unchecked = append(inv.unchecked, filepath.ToSlash(rel))
	}
	sort.Strings(inv.unchecked)
	sort.Strings(inv.warnings)
	inv.warnings = slices.Compact(inv.warnings)
	return inv, nil
}

// skipDir reports a directory the go command leaves out of a ./... pattern,
// or one that holds no module's code.
func skipDir(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") ||
		name == "node_modules" || name == "testdata" || name == "vendor"
}

// goFiles returns every .go file in the tree outside the directories skipDir
// names, as absolute paths.
func goFiles(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if file != root && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(file, ".go") {
			out = append(out, file)
		}
		return nil
	})
	return out, err
}

// pending returns the directory patterns, relative to module m, that still
// hold a file no configuration has built.
func pending(root string, mods []module, m module, all []string, built map[string]bool) []string {
	dirs := map[string]bool{}
	for _, f := range all {
		if built[f] {
			continue
		}
		rel, err := filepath.Rel(root, filepath.Dir(f))
		if err != nil {
			continue
		}
		dir := filepath.ToSlash(rel)
		if moduleOf(mods, dir).Dir != m.Dir {
			continue
		}
		inMod := dir
		if m.Dir != "." {
			inMod = strings.TrimPrefix(strings.TrimPrefix(dir, m.Dir), "/")
		}
		if inMod == "" || inMod == "." {
			dirs["."] = true
		} else {
			dirs["./"+inMod] = true
		}
	}
	out := make([]string, 0, len(dirs))
	for d := range dirs {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// scan reports every identifier in info that resolves to one of the things
// the flip removes, with its position and kind.
func scan(fset *token.FileSet, info *types.Info, add func(token.Position, string)) {
	for id, obj := range info.Uses {
		if k := kindOf(obj); k != "" {
			add(fset.Position(id.Pos()), k)
		}
	}
}

// kindOf names what obj is when it is a Block field the flip removes, the
// type model.Target, or a core/model function whose signature carries one.
// Anything else is "".
func kindOf(obj types.Object) string {
	if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != modelPath {
		return ""
	}
	switch o := obj.(type) {
	case *types.Var:
		if o.IsField() && fields[o.Name()] && blockField(o.Pkg(), o.Name()) == o {
			return "Block." + o.Name()
		}
	case *types.TypeName:
		if o.Name() == "Target" && !o.IsAlias() {
			return "model.Target"
		}
	case *types.Func:
		sig := o.Signature()
		if carriesTarget(sig.Params()) || carriesTarget(sig.Results()) {
			if recv := sig.Recv(); recv != nil {
				return recvName(recv.Type()) + "." + o.Name()
			}
			return "model." + o.Name()
		}
	}
	return ""
}

// blockFields caches the fields of each loaded copy of core/model's Block.
var blockFields = map[*types.Package]map[string]*types.Var{}

// blockField returns the Block field of that name in pkg, a copy of
// core/model.
func blockField(pkg *types.Package, name string) *types.Var {
	byName, ok := blockFields[pkg]
	if !ok {
		byName = map[string]*types.Var{}
		if tn, ok := pkg.Scope().Lookup("Block").(*types.TypeName); ok {
			if st, ok := tn.Type().Underlying().(*types.Struct); ok {
				for f := range st.Fields() {
					if fields[f.Name()] {
						byName[f.Name()] = f
					}
				}
			}
		}
		blockFields[pkg] = byName
	}
	return byName[name]
}

// carriesTarget reports whether a parameter or result list carries a
// model.Target.
func carriesTarget(t *types.Tuple) bool {
	for v := range t.Variables() {
		if hasTarget(v.Type()) {
			return true
		}
	}
	return false
}

// hasTarget reports whether t is model.Target or is built from it: a pointer,
// slice, array, map, channel or function over it. It does not look inside
// another named type, so a *model.Block, whose Targets hold Target values,
// does not count.
func hasTarget(t types.Type) bool {
	switch t := t.(type) {
	case *types.Named:
		o := t.Obj()
		return o.Pkg() != nil && o.Pkg().Path() == modelPath && o.Name() == "Target"
	case *types.Alias:
		return hasTarget(types.Unalias(t))
	case *types.Pointer:
		return hasTarget(t.Elem())
	case *types.Slice:
		return hasTarget(t.Elem())
	case *types.Array:
		return hasTarget(t.Elem())
	case *types.Map:
		return hasTarget(t.Key()) || hasTarget(t.Elem())
	case *types.Chan:
		return hasTarget(t.Elem())
	case *types.Signature:
		return carriesTarget(t.Params()) || carriesTarget(t.Results())
	}
	return false
}

// recvName is a method receiver's type name without the pointer.
func recvName(t types.Type) string {
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	if n, ok := t.(*types.Named); ok {
		return n.Obj().Name()
	}
	return types.TypeString(t, nil)
}

// use is one identifier that resolves to something the flip removes.
type use struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Col     int    `json:"col"`
	Kind    string `json:"kind"`
	Test    bool   `json:"test"`
	Module  string `json:"module"`
	Package string `json:"package"` // the directory, relative to the root
	Allowed bool   `json:"allowed,omitempty"`
}

// configCount is a build configuration and the number of repository files it
// type-checked that no earlier configuration had.
type configCount struct {
	Name  string `json:"name"`
	Files int    `json:"files"`
}

// configLine names each configuration with the files it added.
func (inv *inventory) configLine() string {
	parts := make([]string, 0, len(inv.configs))
	for i, c := range inv.configs {
		if i == 0 {
			parts = append(parts, fmt.Sprintf("%s (%d files)", c.Name, c.Files))
		} else {
			parts = append(parts, fmt.Sprintf("%s (%d more)", c.Name, c.Files))
		}
	}
	return strings.Join(parts, ", ")
}

// within reports whether file sits inside the repository root.
func within(root, file string) bool {
	rel, err := filepath.Rel(root, file)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// inventory collects the uses found across every configuration.
type inventory struct {
	root      string
	mods      []module
	seen      map[string]bool
	uses      []use
	packages  int // package variants type-checked
	configs   []configCount
	unchecked []string
	warnings  []string
}

func newInventory(root string, mods []module) *inventory {
	return &inventory{root: root, mods: mods, seen: map[string]bool{}}
}

// add records a use at pos, once per position, unless it sits outside the
// repository or in core/model.
func (inv *inventory) add(pos token.Position, kind string) {
	file := pos.Filename
	if filepath.IsAbs(file) {
		if !within(inv.root, file) {
			return
		}
		rel, err := filepath.Rel(inv.root, file)
		if err != nil {
			return
		}
		file = rel
	}
	file = filepath.ToSlash(file)
	key := fmt.Sprintf("%s:%d:%d", file, pos.Line, pos.Column)
	if inv.seen[key] {
		return
	}
	inv.seen[key] = true
	dir := path.Dir(file)
	if dir == exemptDir {
		return
	}
	_, ok := allowed[dir]
	inv.uses = append(inv.uses, use{
		File:    file,
		Line:    pos.Line,
		Col:     pos.Column,
		Kind:    kind,
		Test:    strings.HasSuffix(file, "_test.go"),
		Module:  moduleOf(inv.mods, dir).Path,
		Package: dir,
		Allowed: ok,
	})
}

// remaining is the number of uses outside the allowed packages.
func (inv *inventory) remaining() int {
	n := 0
	for _, u := range inv.uses {
		if !u.Allowed {
			n++
		}
	}
	return n
}

// count is the uses of one package, or of a total, split into non-test and
// test.
type count struct {
	NonTest      int            `json:"non_test"`
	Test         int            `json:"test"`
	NonTestFiles int            `json:"non_test_files"`
	TestFiles    int            `json:"test_files"`
	Kinds        map[string]int `json:"kinds,omitempty"` // "Block.Source" and "Block.Source (test)"
	files        map[string]bool
}

func (c *count) add(u use) {
	if c.files == nil {
		c.files = map[string]bool{}
		c.Kinds = map[string]int{}
	}
	kind := u.Kind
	if u.Test {
		c.Test++
		kind += " (test)"
	} else {
		c.NonTest++
	}
	c.Kinds[kind]++
	if !c.files[u.File] {
		c.files[u.File] = true
		if u.Test {
			c.TestFiles++
		} else {
			c.NonTestFiles++
		}
	}
}

// pkgCount is one package's uses.
type pkgCount struct {
	Module  string `json:"module"`
	Package string `json:"package"`
	Allowed string `json:"allowed,omitempty"` // the reason, for an allowed package
	count
}

// summary aggregates the uses per package and in total.
type summary struct {
	Packages []*pkgCount `json:"packages"`
	Total    count       `json:"total"`
	Allowed  count       `json:"allowed"`
	// Non-test and test package counts in the total.
	NonTestPackages int `json:"non_test_packages"`
	TestPackages    int `json:"test_packages"`
}

func (inv *inventory) summarize() summary {
	byPkg := map[string]*pkgCount{}
	for _, u := range inv.uses {
		pc := byPkg[u.Package]
		if pc == nil {
			pc = &pkgCount{Module: u.Module, Package: u.Package, Allowed: allowed[u.Package]}
			byPkg[u.Package] = pc
		}
		pc.add(u)
	}
	for dir, reason := range allowed {
		if byPkg[dir] == nil {
			byPkg[dir] = &pkgCount{Module: moduleOf(inv.mods, dir).Path, Package: dir, Allowed: reason}
		}
	}
	var s summary
	for _, pc := range byPkg {
		s.Packages = append(s.Packages, pc)
	}
	order := map[string]int{}
	for i, m := range inv.mods {
		order[m.Path] = i
	}
	sort.Slice(s.Packages, func(i, j int) bool {
		a, b := s.Packages[i], s.Packages[j]
		if order[a.Module] != order[b.Module] {
			return order[a.Module] < order[b.Module]
		}
		return a.Package < b.Package
	})
	for _, u := range inv.uses {
		if u.Allowed {
			s.Allowed.add(u)
		} else {
			s.Total.add(u)
		}
	}
	for _, pc := range s.Packages {
		if pc.Allowed != "" {
			continue
		}
		if pc.NonTest > 0 {
			s.NonTestPackages++
		}
		if pc.Test > 0 {
			s.TestPackages++
		}
	}
	return s
}

// kinds lists the kinds in c, in a stable order: the fields, the type, then
// the functions.
func kinds(c count) []string {
	set := map[string]bool{}
	for k := range c.Kinds {
		set[strings.TrimSuffix(k, " (test)")] = true
	}
	rank := func(k string) int {
		switch k {
		case "Block.Source":
			return 0
		case "Block.Targets":
			return 1
		case "Block.SourceStatus":
			return 2
		case "model.Target":
			return 3
		}
		return 4
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if rank(out[i]) != rank(out[j]) {
			return rank(out[i]) < rank(out[j])
		}
		return out[i] < out[j]
	})
	return out
}

func (inv *inventory) sortedUses() []use {
	out := slices.Clone(inv.uses)
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Col < out[j].Col
	})
	return out
}

func (inv *inventory) writeText(w io.Writer, verbose bool) {
	s := inv.summarize()
	width := len("package")
	for _, pc := range s.Packages {
		width = max(width, len(pc.Package)+2)
	}
	fmt.Fprintf(w, "fieldguard: type-checked %d package variants in %d modules under %s\n",
		inv.packages, len(inv.mods), inv.configLine())
	fmt.Fprintln(w, "Uses of Block.Source, Block.Targets, Block.SourceStatus and model.Target outside core/model:")
	fmt.Fprintf(w, "%-*s %9s %9s\n", width, "", "non-test", "test")
	module := ""
	var modTotal count
	flush := func() {
		if module != "" {
			fmt.Fprintf(w, "%-*s %9d %9d\n", width, "  module total", modTotal.NonTest, modTotal.Test)
		}
	}
	for _, pc := range s.Packages {
		if pc.Allowed != "" {
			continue
		}
		if pc.Module != module {
			flush()
			module, modTotal = pc.Module, count{}
			fmt.Fprintln(w, module)
		}
		modTotal.NonTest += pc.NonTest
		modTotal.Test += pc.Test
		fmt.Fprintf(w, "%-*s %9d %9d\n", width, "  "+pc.Package, pc.NonTest, pc.Test)
	}
	flush()
	fmt.Fprintln(w, "allowed")
	for _, pc := range s.Packages {
		if pc.Allowed != "" {
			fmt.Fprintf(w, "%-*s %9d %9d   %s\n", width, "  "+pc.Package, pc.NonTest, pc.Test, pc.Allowed)
		}
	}
	fmt.Fprintln(w, "total outside core/model and the allowed packages:")
	fmt.Fprintf(w, "  non-test: %d uses in %d files across %d packages\n", s.Total.NonTest, s.Total.NonTestFiles, s.NonTestPackages)
	fmt.Fprintf(w, "  test:     %d uses in %d files across %d packages\n", s.Total.Test, s.Total.TestFiles, s.TestPackages)
	fmt.Fprintf(w, "by kind:%*s %9s %9s\n", width-len("by kind:"), "", "non-test", "test")
	for _, k := range kinds(s.Total) {
		fmt.Fprintf(w, "%-*s %9d %9d\n", width, "  "+k, s.Total.Kinds[k], s.Total.Kinds[k+" (test)"])
	}
	if len(inv.unchecked) > 0 {
		fmt.Fprintf(w, "unchecked: %d files no configuration builds\n", len(inv.unchecked))
		for _, f := range inv.unchecked {
			fmt.Fprintln(w, "  "+f)
		}
	}
	if verbose {
		fmt.Fprintln(w, "uses:")
		for _, u := range inv.sortedUses() {
			note := ""
			if u.Allowed {
				note = " (allowed)"
			}
			fmt.Fprintf(w, "  %s:%d:%d: %s%s\n", u.File, u.Line, u.Col, u.Kind, note)
		}
	}
}

func (inv *inventory) writeMarkdown(w io.Writer, verbose bool) {
	s := inv.summarize()
	fmt.Fprintln(w, "# Block field inventory")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Uses of `Block.Source`, `Block.Targets`, `Block.SourceStatus` and `model.Target` outside `core/model`, from `scripts/fieldguard`: %d package variants type-checked in %d modules under %s.\n",
		inv.packages, len(inv.mods), inv.configLine())
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Totals")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| | uses | files | packages |")
	fmt.Fprintln(w, "| --- | ---: | ---: | ---: |")
	fmt.Fprintf(w, "| non-test | %d | %d | %d |\n", s.Total.NonTest, s.Total.NonTestFiles, s.NonTestPackages)
	fmt.Fprintf(w, "| test | %d | %d | %d |\n", s.Total.Test, s.Total.TestFiles, s.TestPackages)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| kind | non-test | test |")
	fmt.Fprintln(w, "| --- | ---: | ---: |")
	for _, k := range kinds(s.Total) {
		fmt.Fprintf(w, "| `%s` | %d | %d |\n", k, s.Total.Kinds[k], s.Total.Kinds[k+" (test)"])
	}
	module := ""
	var modTotal count
	flush := func() {
		if module != "" {
			fmt.Fprintf(w, "| **module total** | **%d** | **%d** |\n", modTotal.NonTest, modTotal.Test)
		}
	}
	for _, pc := range s.Packages {
		if pc.Allowed != "" {
			continue
		}
		if pc.Module != module {
			flush()
			module, modTotal = pc.Module, count{}
			fmt.Fprintln(w)
			fmt.Fprintf(w, "## `%s`\n\n", module)
			fmt.Fprintln(w, "| package | non-test | test |")
			fmt.Fprintln(w, "| --- | ---: | ---: |")
		}
		modTotal.NonTest += pc.NonTest
		modTotal.Test += pc.Test
		fmt.Fprintf(w, "| `%s` | %d | %d |\n", pc.Package, pc.NonTest, pc.Test)
	}
	flush()
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Allowed")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| package | non-test | test | reason |")
	fmt.Fprintln(w, "| --- | ---: | ---: | --- |")
	for _, pc := range s.Packages {
		if pc.Allowed != "" {
			fmt.Fprintf(w, "| `%s` | %d | %d | %s |\n", pc.Package, pc.NonTest, pc.Test, pc.Allowed)
		}
	}
	if len(inv.unchecked) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "## Unchecked\n\n%d files no configuration builds:\n\n", len(inv.unchecked))
		for _, f := range inv.unchecked {
			fmt.Fprintf(w, "- `%s`\n", f)
		}
	}
	if verbose {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "## Uses")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "```")
		for _, u := range inv.sortedUses() {
			fmt.Fprintf(w, "%s:%d:%d: %s\n", u.File, u.Line, u.Col, u.Kind)
		}
		fmt.Fprintln(w, "```")
	}
}

func (inv *inventory) writeJSON(w io.Writer) error {
	s := inv.summarize()
	out := struct {
		Modules         []module      `json:"modules"`
		Configs         []configCount `json:"configs"`
		Packages        []*pkgCount   `json:"packages"`
		Total           count         `json:"total"`
		NonTestPackages int           `json:"non_test_packages"`
		TestPackages    int           `json:"test_packages"`
		Allowed         count         `json:"allowed"`
		Unchecked       []string      `json:"unchecked"`
		Warnings        []string      `json:"warnings"`
		Uses            []use         `json:"uses"`
	}{inv.mods, inv.configs, s.Packages, s.Total, s.NonTestPackages, s.TestPackages, s.Allowed, inv.unchecked, inv.warnings, inv.sortedUses()}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}
