// Command migrationledger writes the committed migration ledger files:
// core/storage/migration_ledger.json for kapi's stores and
// bowrain/storage/migration_ledger.json for the platform's. Each holds, per
// ledger table and version, the identity the migration ships with and every
// name the version has carried.
//
// The identities come from the runners themselves: `make migration-ledger`
// runs the test suites of the packages that own a migration list with
// NEOKAPI_MIGRATION_LEDGER_RECORD naming a directory, and every runner writes
// what it migrates there (core/storage.RecordLedger). This reads that
// directory.
//
// The names come from git history. For every file that declares a migration
// list, it reads each commit's version of the file (following renames) and
// tracks each version of each list from the commit that added it. A version
// keeps the names it carried only across commits that left its statements as
// they were, compared as the database executes them. A change of name and
// statements together is another migration under a reused number, and its
// earlier names are not this migration's.
//
// A recorded ledger whose versions no migration file declares (a list a test
// writes for itself) is left out.
package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/storage"
)

func main() {
	record := flag.String("record", "", "the directory the runners recorded into")
	names := flag.Bool("names", false, "print the names read from history and exit")
	flag.Parse()
	if err := os.Chdir(topLevel()); err != nil {
		fail(err)
	}
	history, renamed, err := readHistory()
	if err != nil {
		fail(err)
	}
	if *names {
		for _, r := range renamed {
			fmt.Println(r)
		}
		return
	}
	if *record == "" {
		fail(errors.New("-record names the directory make migration-ledger collected into"))
	}
	ledgers, err := readRecords(*record, history)
	if err != nil {
		fail(err)
	}
	for side, path := range map[string]string{"core": "core/storage/migration_ledger.json", "bowrain": "bowrain/storage/migration_ledger.json"} {
		b, err := json.MarshalIndent(ledgers[side], "", "  ")
		if err != nil {
			fail(err)
		}
		if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
			fail(err)
		}
		fmt.Printf("migrationledger: wrote %s (%d ledgers)\n", path, len(ledgers[side]))
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "migrationledger:", err)
	os.Exit(1)
}

func topLevel() string {
	out, err := exec.CommandContext(context.Background(), "git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		fail(fmt.Errorf("find the repository: %w", err))
	}
	return strings.TrimSpace(string(out))
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

// sideOf is the ledger file a migration list's names belong to.
func sideOf(path string) string {
	if strings.HasPrefix(path, "bowrain/") {
		return "bowrain"
	}
	return "core"
}

// historyKey identifies a version by its number and current name, which is
// how a recorded ledger joins the history.
func historyKey(version int, current string) string {
	return strconv.Itoa(version) + "\x00" + current
}

// readHistory reads every migration list's history. It returns, per side, the
// names each version has carried keyed by historyKey, and a line per version
// whose name changed, for the pull request's description.
func readHistory() (map[string]map[string][]string, []string, error) {
	out, err := git("grep", "-l", "--", "storage.Migration{", "--", "*.go")
	if err != nil && out == "" {
		return nil, nil, err
	}
	history := map[string]map[string][]string{"core": {}, "bowrain": {}}
	var renamed []string
	for file := range strings.FieldsSeq(out) {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		lists, err := fileHistory(file)
		if err != nil {
			return nil, nil, err
		}
		for key, versions := range lists {
			for v, t := range versions {
				history[sideOf(file)][historyKey(v, t.current)] = t.names
				if len(t.names) > 1 {
					renamed = append(renamed, fmt.Sprintf("reworded: %s (%s) version %d: %s", file, key, v, strings.Join(quoteAll(t.names), " -> ")))
				}
				if len(t.earlier) > 0 {
					renamed = append(renamed, fmt.Sprintf("reused number: %s (%s) version %d is %q; it carried %s before", file, key, v, t.current, strings.Join(quoteAll(t.earlier), ", ")))
				}
			}
		}
	}
	slices.Sort(renamed)
	return history, renamed, nil
}

func quoteAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = strconv.Quote(s)
	}
	return out
}

// track is one version of one list through history.
type track struct {
	current    string
	statements string
	names      []string
	// earlier are the names this number carried before the current
	// migration took it, for the report; they are not this migration's.
	earlier []string
}

// fileHistory walks a file's commits oldest first, following renames, and
// tracks each version of each list in it.
func fileHistory(file string) (map[string]map[int]*track, error) {
	// git mishandles --reverse together with --follow, so the commits are
	// read newest first and reversed here. --follow can take a near-copy for
	// a rename; at that commit the statements differ, so no name crosses it.
	log, err := git("log", "--follow", "--format=commit %H", "--name-only", "--", file)
	if err != nil {
		return nil, err
	}
	type rev struct{ commit, path string }
	var revs []rev
	var commit string
	sc := bufio.NewScanner(strings.NewReader(log))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "commit "):
			commit = strings.TrimPrefix(line, "commit ")
		case line != "" && commit != "":
			revs = append(revs, rev{commit, line})
			commit = ""
		}
	}
	slices.Reverse(revs)
	current, err := currentStatements(file)
	if err != nil {
		return nil, err
	}
	tracks := map[string]map[int]*track{}
	for _, r := range revs {
		src, err := git("show", r.commit+":"+r.path)
		if err != nil {
			continue // removed in this commit
		}
		lists, err := parseLists(r.path, []byte(src))
		if err != nil {
			continue // a commit whose file does not parse names nothing
		}
		for key, versions := range lists {
			if tracks[key] == nil {
				tracks[key] = map[int]*track{}
			}
			for v, e := range versions {
				t := tracks[key][v]
				switch {
				case t == nil:
					tracks[key][v] = &track{current: e.description, statements: e.statements, names: []string{e.description}}
				case e.description != t.current && changedStatements(t.statements, e.statements, current[key][v]):
					// A new migration under a reused number.
					*t = track{current: e.description, statements: e.statements, names: []string{e.description}, earlier: append(t.earlier, t.names...)}
				case e.description != t.current:
					t.current = e.description
					if !slices.Contains(t.names, e.description) {
						t.names = append(t.names, e.description)
					}
				default:
					t.statements = e.statements
				}
			}
		}
	}
	// Only the lists and versions the file declares now.
	src, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	now, err := parseLists(file, src)
	if err != nil {
		return nil, err
	}
	out := map[string]map[int]*track{}
	for key, versions := range now {
		for v, e := range versions {
			t := tracks[key][v]
			if t == nil || t.current != e.description {
				t = &track{current: e.description, names: []string{e.description}}
			}
			if out[key] == nil {
				out[key] = map[int]*track{}
			}
			out[key][v] = t
		}
	}
	return out, nil
}

type entry struct {
	description string
	statements  string
	retired     bool
}

// parseLists reads every migration list literal in a Go file, keyed by its
// order in the file. A variable holding a list is renamed now and then (brand
// became voice), and the order survives that.
func parseLists(name string, src []byte) (map[string]map[int]entry, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, 0)
	if err != nil {
		return nil, err
	}
	lists := map[string]map[int]entry{}
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok || !isMigrationSlice(lit.Type) {
			return true
		}
		lists[fmt.Sprintf("list %d", len(lists)+1)] = readList(fset, lit)
		return false
	})
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

func readList(fset *token.FileSet, lit *ast.CompositeLit) map[int]entry {
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
			case "SQL":
				e.statements = statements(fset, kv.Value)
			case "Retired":
				if id, ok := kv.Value.(*ast.Ident); ok && id.Name == "true" {
					e.retired = true
				}
			}
		}
		if version >= 0 && !e.retired {
			out[version] = e
		}
	}
	return out
}

// statements is a migration's SQL as the database executes it, for a
// literal, or "expr:" and the expression's source for anything else (a
// constant, a rendered call), whose statements this cannot read.
func statements(fset *token.FileSet, e ast.Expr) string {
	if b, ok := e.(*ast.BasicLit); ok && b.Kind == token.STRING {
		if s, err := strconv.Unquote(b.Value); err == nil {
			return storage.NormalizeSQL(s)
		}
	}
	var buf bytes.Buffer
	_ = printer.Fprint(&buf, fset, e)
	return "expr:" + buf.String()
}

// changedStatements reports whether a commit changed what a version
// executes. Two literals compare directly. When one side is a constant or a
// rendered call, the literal is compared with what the version executes now
// (now, its frozen SQL): a literal moved into a renderer whose output is the
// same text keeps the version's names, and any other literal is another
// migration. Two non-literals (a renderer edited for another version) keep
// them: the renderers are golden-tested to render shipped versions
// unchanged.
func changedStatements(before, after, now string) bool {
	beforeExpr, afterExpr := strings.HasPrefix(before, "expr:"), strings.HasPrefix(after, "expr:")
	switch {
	case !beforeExpr && !afterExpr:
		return before != after
	case beforeExpr && afterExpr:
		return false
	case beforeExpr:
		return !sameAsNow(after, now)
	default:
		return !sameAsNow(before, now)
	}
}

// sameAsNow reports whether a literal's statements are the version's frozen
// SQL, with the tokenizer placeholder read as either build's tokenizer.
func sameAsNow(literal, now string) bool {
	if now == "" || strings.HasPrefix(now, "expr:") {
		return false
	}
	for _, tok := range []string{storage.FTSWordTokenizerPlaceholder, "icu", "unicode61"} {
		if literal == storage.NormalizeSQL(strings.ReplaceAll(now, storage.FTSWordTokenizerPlaceholder, tok)) {
			return true
		}
	}
	return false
}

// currentStatements is what each version of each list in file executes now,
// with a constant resolved from the package's other files.
func currentStatements(file string) (map[string]map[int]string, error) {
	consts, err := packageConsts(filepath.Dir(file))
	if err != nil {
		return nil, err
	}
	src, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	lists, err := parseLists(file, src)
	if err != nil {
		return nil, err
	}
	out := map[string]map[int]string{}
	for key, versions := range lists {
		out[key] = map[int]string{}
		for v, e := range versions {
			sql := e.statements
			if name, ok := strings.CutPrefix(sql, "expr:"); ok {
				if c, ok := consts[name]; ok {
					sql = storage.NormalizeSQL(c)
				}
			}
			out[key][v] = sql
		}
	}
	return out, nil
}

// packageConsts reads the string constants declared in a package's files.
func packageConsts(dir string) (map[string]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), f, nil, 0)
		if err != nil {
			return nil, err
		}
		for _, decl := range parsed.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, name := range vs.Names {
					if i < len(vs.Values) {
						if b, ok := vs.Values[i].(*ast.BasicLit); ok && b.Kind == token.STRING {
							if v, err := strconv.Unquote(b.Value); err == nil {
								out[name.Name] = v
							}
						}
					}
				}
			}
		}
	}
	return out, nil
}

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

// readRecords joins what the runners recorded with the names from history.
func readRecords(dir string, history map[string]map[string][]string) (map[string]storage.ShippedLedgers, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("nothing was recorded in %s", dir)
	}
	out := map[string]storage.ShippedLedgers{"core": {}, "bowrain": {}}
	for _, f := range files {
		side, _, _ := strings.Cut(filepath.Base(f), "-")
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		for line := range strings.SplitSeq(strings.TrimSpace(string(b)), "\n") {
			var r struct {
				Ledger      string `json:"ledger"`
				Version     int    `json:"version"`
				Description string `json:"description"`
				Identity    string `json:"identity"`
			}
			if err := json.Unmarshal([]byte(line), &r); err != nil {
				return nil, fmt.Errorf("%s: %w", f, err)
			}
			names, ok := history[side][historyKey(r.Version, r.Description)]
			if !ok {
				continue // a list a test writes for itself
			}
			if out[side][r.Ledger] == nil {
				out[side][r.Ledger] = map[int]storage.ShippedMigration{}
			}
			if prev, seen := out[side][r.Ledger][r.Version]; seen && prev.Identity != r.Identity {
				return nil, fmt.Errorf("%s version %d was recorded with two identities, %s and %s: a migration differs between builds or runs", r.Ledger, r.Version, prev.Identity, r.Identity)
			}
			out[side][r.Ledger][r.Version] = storage.ShippedMigration{Description: r.Description, Identity: r.Identity, Names: names}
		}
	}
	return out, nil
}
