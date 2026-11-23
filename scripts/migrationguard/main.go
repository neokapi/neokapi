// Command migrationguard fails a change whose migrations disagree with
// origin/main's: a list in the working tree and the same list on origin/main
// carrying one version under two names, or a version the committed migration
// ledger files (core/storage, bowrain/storage) list on origin/main whose name
// or identity differs here. The ledger files are written by the runners
// themselves (make migration-ledger), so they cover every list however it is
// built, a spliced range included, and pin each shipped migration's identity:
// an edit to a shipped migration fails here before it merges. Two branches that each append "the
// next version" pass every test on their own and collide once both merge; the
// database records whichever ran first and skips the other for good.
//
// It reads every Go file that declares a []storage.Migration literal, at the
// working tree and at origin/main (fetched shallowly when the checkout lacks
// it), parses each list with go/parser and compares the lists by name.
//
// It cannot see a collision that reaches main after it ran: a change checked
// against an older main can still merge into one that took the same number
// since. The migration runner is what protects a database. It records each
// migration's name and identity and refuses to start when the code
// disagrees (core/storage.CheckLedger), so a deploy that would skip a
// migration stops before it serves.
package main

import (
	"bytes"
	"encoding/json"
	"context"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

const mainRef = "origin/main"

func main() {
	selfTest := flag.Bool("self-test", false, "prove the comparison on fixtures and exit")
	flag.Parse()
	if *selfTest {
		if err := runSelfTest(); err != nil {
			fmt.Fprintln(os.Stderr, "migrationguard self-test:", err)
			os.Exit(1)
		}
		fmt.Println("migrationguard self-test: ok")
		return
	}
	if top, err := git("rev-parse", "--show-toplevel"); err == nil {
		if err := os.Chdir(strings.TrimSpace(top)); err != nil {
			fmt.Fprintln(os.Stderr, "migrationguard:", err)
			os.Exit(1)
		}
	}
	problems, err := check()
	if err != nil {
		fmt.Fprintln(os.Stderr, "migrationguard:", err)
		os.Exit(1)
	}
	if len(problems) > 0 {
		fmt.Fprintln(os.Stderr, "migration versions collide with "+mainRef+":")
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, "  "+p)
		}
		fmt.Fprintln(os.Stderr, "Merge "+mainRef+" and give your migrations the next free versions.")
		os.Exit(1)
	}
	fmt.Println("migrationguard: no migration version collides with " + mainRef)
}

// check compares every migration list in the working tree with origin/main's.
func check() ([]string, error) {
	if err := ensureMain(); err != nil {
		return nil, err
	}
	files, err := migrationFiles()
	if err != nil {
		return nil, err
	}
	var problems []string
	for _, f := range files {
		here, err := os.ReadFile(f)
		if err != nil {
			continue // removed in the working tree: nothing of it can collide
		}
		there, err := git("show", mainRef+":"+f)
		if err != nil {
			continue // new in this change: nothing on main to collide with
		}
		hereLists, err := parseLists(f, here)
		if err != nil {
			return nil, err
		}
		thereLists, err := parseLists(f, []byte(there))
		if err != nil {
			return nil, err
		}
		problems = append(problems, compare(f, hereLists, thereLists)...)
	}
	for _, f := range ledgerFiles {
		here, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		there, err := git("show", mainRef+":"+f)
		if err != nil {
			continue // not on main yet
		}
		p, err := compareLedgers(f, here, []byte(there))
		if err != nil {
			return nil, err
		}
		problems = append(problems, p...)
	}
	return problems, nil
}

// ledgerFiles are the committed migration ledger files.
var ledgerFiles = []string{"core/storage/migration_ledger.json", "bowrain/storage/migration_ledger.json"}

type shipped struct {
	Description string `json:"description"`
	Identity    string `json:"identity"`
}

// compareLedgers reports each version origin/main's ledger file lists that
// this change drops, renames or gives another identity.
func compareLedgers(file string, here, there []byte) ([]string, error) {
	var h, m map[string]map[string]shipped
	if err := json.Unmarshal(here, &h); err != nil {
		return nil, fmt.Errorf("read %s: %w", file, err)
	}
	if err := json.Unmarshal(there, &m); err != nil {
		return nil, fmt.Errorf("read %s on %s: %w", file, mainRef, err)
	}
	var problems []string
	ledgers := make([]string, 0, len(m))
	for l := range m {
		ledgers = append(ledgers, l)
	}
	sort.Strings(ledgers)
	for _, l := range ledgers {
		versions := make([]string, 0, len(m[l]))
		for v := range m[l] {
			versions = append(versions, v)
		}
		sort.Slice(versions, func(i, j int) bool { a, _ := strconv.Atoi(versions[i]); b, _ := strconv.Atoi(versions[j]); return a < b })
		for _, v := range versions {
			want := m[l][v]
			got, ok := h[l][v]
			switch {
			case !ok:
				problems = append(problems, fmt.Sprintf("%s: %s version %s (%q) is on %s and not here", file, l, v, want.Description, mainRef))
			case got.Description != want.Description:
				problems = append(problems, fmt.Sprintf("%s: %s version %s is %q here and %q on %s", file, l, v, got.Description, want.Description, mainRef))
			case got.Identity != want.Identity:
				problems = append(problems, fmt.Sprintf("%s: %s version %s (%q) has identity %s here and shipped with %s on %s: a shipped migration was edited", file, l, v, want.Description, got.Identity, want.Identity, mainRef))
			}
		}
	}
	return problems, nil
}

// ensureMain makes origin/main readable; a shallow CI checkout lacks it.
func ensureMain() error {
	if _, err := git("rev-parse", "--verify", "--quiet", mainRef); err == nil {
		return nil
	}
	if _, err := git("fetch", "--no-tags", "--depth=1", "origin", "+refs/heads/main:refs/remotes/origin/main"); err != nil {
		return fmt.Errorf("fetch %s: %w", mainRef, err)
	}
	return nil
}

// migrationFiles names every Go file, outside tests, that declares a
// migration list in the working tree or on origin/main.
func migrationFiles() ([]string, error) {
	seen := map[string]bool{}
	for _, args := range [][]string{
		{"grep", "-l", "--", "storage.Migration{", "--", "*.go"},
		{"grep", "-l", "--", "storage.Migration{", mainRef, "--", "*.go"},
	} {
		out, err := git(args...)
		if err != nil && out == "" {
			continue // no match is exit status 1
		}
		for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
			line = strings.TrimPrefix(line, mainRef+":")
			if line != "" && !strings.HasSuffix(line, "_test.go") {
				seen[line] = true
			}
		}
	}
	files := make([]string, 0, len(seen))
	for f := range seen {
		files = append(files, f)
	}
	sort.Strings(files)
	return files, nil
}

func git(args ...string) (string, error) {
	cmd := exec.CommandContext(context.Background(), "git", args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// entry is one migration of a list: its name, or that its version is retired.
type entry struct {
	description string
	retired     bool
}

// parseLists reads every []storage.Migration (or []Migration) literal in a
// Go file, keyed by the variable or function that holds it, and the order
// within that holder.
func parseLists(name string, src []byte) (map[string]map[int]entry, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}
	lists := map[string]map[int]entry{}
	counts := map[string]int{}
	for _, decl := range file.Decls {
		holder := ""
		switch d := decl.(type) {
		case *ast.FuncDecl:
			holder = "func " + d.Name.Name
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok && len(vs.Names) > 0 {
					holder = "var " + vs.Names[0].Name
				}
			}
		}
		ast.Inspect(decl, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok || !isMigrationSlice(lit.Type) {
				return true
			}
			key := fmt.Sprintf("%s#%d", holder, counts[holder])
			counts[holder]++
			lists[key] = readList(lit)
			return false
		})
	}
	return lists, nil
}

func isMigrationSlice(t ast.Expr) bool {
	arr, ok := t.(*ast.ArrayType)
	if !ok || arr.Len != nil {
		return false
	}
	switch e := arr.Elt.(type) {
	case *ast.SelectorExpr:
		return e.Sel.Name == "Migration"
	case *ast.Ident:
		return e.Name == "Migration"
	}
	return false
}

func readList(lit *ast.CompositeLit) map[int]entry {
	out := map[int]entry{}
	for _, elt := range lit.Elts {
		m, ok := elt.(*ast.CompositeLit)
		if !ok {
			continue
		}
		version := -1
		var e entry
		for _, f := range m.Elts {
			kv, ok := f.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, _ := kv.Key.(*ast.Ident)
			if key == nil {
				continue
			}
			switch key.Name {
			case "Version":
				if b, ok := kv.Value.(*ast.BasicLit); ok && b.Kind == token.INT {
					version, _ = strconv.Atoi(b.Value)
				}
			case "Description":
				e.description = stringValue(kv.Value)
			case "Retired":
				if id, ok := kv.Value.(*ast.Ident); ok && id.Name == "true" {
					e.retired = true
				}
			}
		}
		if version >= 0 {
			out[version] = e
		}
	}
	return out
}

// stringValue is a string literal's value, or a concatenation of literals.
func stringValue(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.BasicLit:
		if s, err := strconv.Unquote(v.Value); err == nil {
			return s
		}
		return v.Value
	case *ast.BinaryExpr:
		return stringValue(v.X) + stringValue(v.Y)
	}
	return fmt.Sprintf("%T", e)
}

// compare reports each version a list carries here and on main under two
// names. A version main retired and this change still applies (or the other
// way round) is a collision too.
func compare(file string, here, there map[string]map[int]entry) []string {
	var problems []string
	keys := make([]string, 0, len(here))
	for k := range here {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		mainList, ok := there[k]
		if !ok {
			continue
		}
		versions := make([]int, 0, len(here[k]))
		for v := range here[k] {
			versions = append(versions, v)
		}
		sort.Ints(versions)
		for _, v := range versions {
			h := here[k][v]
			m, ok := mainList[v]
			if !ok || h == m {
				continue
			}
			problems = append(problems, fmt.Sprintf("%s (%s): version %d is %s here and %s on %s", file, k, v, describe(h), describe(m), mainRef))
		}
	}
	return problems
}

func describe(e entry) string {
	if e.retired {
		return "retired"
	}
	return strconv.Quote(e.description)
}

func runSelfTest() error {
	const mainSrc = `package p
var Migrations = []storage.Migration{
	{Version: 1, Description: "create a", SQL: "x"},
	{Version: 2, Description: "context session holds keyed by the item's parts"},
}`
	const branchSrc = `package p
var Migrations = []storage.Migration{
	{Version: 1, Description: "create a", SQL: "x"},
	{Version: 2, Description: "the agent host a review decision was recorded from"},
}`
	const appendedSrc = `package p
var Migrations = []storage.Migration{
	{Version: 1, Description: "create a", SQL: "x"},
	{Version: 2, Description: "context session holds keyed by the item's parts"},
	{Version: 3, Description: "the agent host " + "a review decision was recorded from"},
}`
	there, err := parseLists("main.go", []byte(mainSrc))
	if err != nil {
		return err
	}
	collided, err := parseLists("branch.go", []byte(branchSrc))
	if err != nil {
		return err
	}
	appended, err := parseLists("appended.go", []byte(appendedSrc))
	if err != nil {
		return err
	}
	if got := compare("f.go", collided, there); len(got) != 1 || !strings.Contains(got[0], "version 2 is \"the agent host a review decision was recorded from\" here and \"context session holds keyed by the item's parts\"") {
		return fmt.Errorf("a reused version was not reported: %q", got)
	}
	if got := compare("f.go", appended, there); len(got) != 0 {
		return fmt.Errorf("a migration appended after main's was reported: %q", got)
	}
	if appended["var Migrations#0"][3].description != "the agent host a review decision was recorded from" {
		return fmt.Errorf("a concatenated description was misread: %q", appended["var Migrations#0"][3].description)
	}

	mainLedger := []byte(`{"store_schema_migrations": {"63": {"description": "the reasons", "identity": "aaa"}, "64": {"description": "holds", "identity": "bbb"}}}`)
	for name, tc := range map[string]struct {
		here string
		want string
	}{
		"an edited migration": {`{"store_schema_migrations": {"63": {"description": "the reasons", "identity": "zzz"}, "64": {"description": "holds", "identity": "bbb"}}}`, "version 63 (\"the reasons\") has identity zzz here and shipped with aaa"},
		"a reused version":    {`{"store_schema_migrations": {"63": {"description": "the reasons", "identity": "aaa"}, "64": {"description": "the agent host", "identity": "ccc"}}}`, "version 64 is \"the agent host\" here and \"holds\""},
		"a dropped version":   {`{"store_schema_migrations": {"63": {"description": "the reasons", "identity": "aaa"}}}`, "version 64 (\"holds\") is on origin/main and not here"},
	} {
		got, err := compareLedgers("l.json", []byte(tc.here), mainLedger)
		if err != nil {
			return err
		}
		if len(got) != 1 || !strings.Contains(got[0], tc.want) {
			return fmt.Errorf("%s was not reported: %q", name, got)
		}
	}
	appendedLedger := `{"store_schema_migrations": {"63": {"description": "the reasons", "identity": "aaa"}, "64": {"description": "holds", "identity": "bbb"}, "65": {"description": "new", "identity": "ddd"}}}`
	if got, err := compareLedgers("l.json", []byte(appendedLedger), mainLedger); err != nil || len(got) != 0 {
		return fmt.Errorf("an appended version was reported: %q: %w", got, err)
	}
	return nil
}
