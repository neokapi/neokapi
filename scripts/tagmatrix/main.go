// Command tagmatrix runs a Go analyzer over every file of the modules it is
// given, whatever build constraint gates the file.
//
// One run of golangci-lint or go vet compiles the files of one build
// configuration: one GOOS, one CGO_ENABLED and one tag set. A file behind
// `//go:build e2e`, `onnx`, `js && wasm` or `windows` is analyzed only by a
// run that selects it. tagmatrix runs the analyzer once per configuration in
// scripts/internal/buildconfigs. The first run covers each module whole. Each
// later run covers only the package directories that still hold a file no
// earlier run compiled. A Go file that no configuration compiles fails the
// run, so a new build tag cannot leave its files unanalyzed.
//
//	go run ./scripts/tagmatrix -tool lint . host cli kapi
//	go run ./scripts/tagmatrix -tool vet plugins/sat
//	go run ./scripts/tagmatrix -config windows -tool lint . host
//	go run ./scripts/tagmatrix -plan .
//
// The -config flag narrows the runs to the named configurations. The earlier
// ones are still listed to find what is left to analyze. CI runs one
// configuration per job.
//
// The -warm flag compiles the export data the analyzer reads for those runs
// and analyzes nothing. The warm job uses it to fill the build cache a lint
// job restores.
//
// The -affected-since flag prints true when a change since a git ref can
// change what those runs report, and false otherwise.
//
// A module that go.work does not list runs with GOWORK=off. The analyzer's
// exit status is collected per run, and every run happens before the command
// exits, so one failing configuration does not hide the next.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/build/constraint"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/scripts/internal/buildconfigs"
)

// listedPackage is the part of `go list -json` tagmatrix reads.
type listedPackage struct {
	Dir            string
	GoFiles        []string
	CgoFiles       []string
	TestGoFiles    []string
	XTestGoFiles   []string
	IgnoredGoFiles []string
}

// run is one analyzer invocation: a configuration and the packages it covers.
type run struct {
	cfg      buildconfigs.Config
	patterns []string
}

// alwaysAffected match the paths whose change can alter any configuration's
// result: the analyzer and its configuration, the module files, the build
// definitions and the CI workflow.
var alwaysAffected = []string{
	".golangci.yml", "go.work", "go.work.sum", "Makefile", "bowrain/Makefile",
	".github/workflows/ci.yml", ".github/actions/", "scripts/tagmatrix/",
	"scripts/internal/", "scripts/lib/", "scripts/pdfium-pkgconfig.sh", "scripts/fetch-pdfium.sh",
}

func main() {
	tool := flag.String("tool", "lint", "analyzer to run: lint (golangci-lint) or vet (go vet)")
	plan := flag.Bool("plan", false, "print the runs and exit without analyzing")
	only := flag.String("config", "", "comma-separated configurations to run (default: all)")
	warm := flag.Bool("warm", false, "compile the export data the runs read, and analyze nothing")
	since := flag.String("affected-since", "", "print whether a change since this git ref affects the runs")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: go run ./scripts/tagmatrix [-tool lint|vet] [-config names] [-plan|-warm|-affected-since ref] module-dir...")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}
	if *tool != "lint" && *tool != "vet" {
		fail(fmt.Errorf("unknown -tool %q: want lint or vet", *tool))
	}
	selected, err := buildconfigs.Named(*only)
	if err != nil {
		fail(err)
	}
	want := map[string]bool{}
	for _, c := range selected {
		want[c.Name] = true
	}
	root, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	work, err := workModules(root)
	if err != nil {
		fail(err)
	}
	var changed []string
	if *since != "" {
		changed, err = changedFiles(root, *since)
		if err != nil {
			fail(err)
		}
		if p := triggering(changed); p != "" {
			fmt.Fprintf(os.Stderr, "tagmatrix: %s changed, so every configuration is affected\n", p)
			fmt.Println("true")
			return
		}
	}

	var failed []string
	for _, dir := range flag.Args() {
		dir = filepath.Clean(dir)
		modDir := filepath.Join(root, dir)
		modEnv := []string{}
		if !work[dir] {
			modEnv = append(modEnv, "GOWORK=off")
		}
		runs, uncovered, err := planModule(modDir, modEnv)
		if err != nil {
			fail(fmt.Errorf("%s: %w", dir, err))
		}
		if len(uncovered) > 0 && *since == "" {
			fmt.Fprintf(os.Stderr, "tagmatrix: %s: no build configuration compiles these files; add one to scripts/internal/buildconfigs:\n", dir)
			for _, f := range uncovered {
				fmt.Fprintln(os.Stderr, "  "+filepath.Join(dir, f))
			}
			failed = append(failed, dir+": uncovered files")
		}
		for _, r := range runs {
			if !want[r.cfg.Name] {
				continue
			}
			env := append(slices.Clone(modEnv), r.cfg.Env...)
			switch {
			case *since != "":
				hit, err := affected(root, modDir, env, r, changed)
				if err != nil {
					fail(fmt.Errorf("%s [%s]: %w", dir, r.cfg.Name, err))
				}
				if hit != "" {
					fmt.Fprintf(os.Stderr, "tagmatrix: %s changed, which %s [%s] compiles\n", hit, dir, r.cfg.Name)
					fmt.Println("true")
					return
				}
			case *plan:
				fmt.Printf("tagmatrix: %s [%s] tags=%s %s\n", dir, r.cfg.Name, r.cfg.Tags, summarize(r.patterns))
			case *warm:
				fmt.Printf("tagmatrix: warm %s [%s] %s\n", dir, r.cfg.Name, summarize(r.patterns))
				if err := compileExports(modDir, env, r.cfg.Tags, r.patterns); err != nil {
					failed = append(failed, fmt.Sprintf("%s [%s]", dir, r.cfg.Name))
				}
			default:
				fmt.Printf("tagmatrix: %s [%s] tags=%s %s\n", dir, r.cfg.Name, r.cfg.Tags, summarize(r.patterns))
				first := buildconfigs.Index(r.cfg.Name) == 0
				if err := analyze(*tool, modDir, env, r.cfg.Tags, first, r.patterns); err != nil {
					failed = append(failed, fmt.Sprintf("%s [%s]", dir, r.cfg.Name))
				}
			}
		}
	}
	if *since != "" {
		fmt.Fprintln(os.Stderr, "tagmatrix: no change reaches a package these configurations compile")
		fmt.Println("false")
		return
	}
	if len(failed) > 0 {
		fmt.Fprintln(os.Stderr, "tagmatrix: failed:")
		for _, f := range failed {
			fmt.Fprintln(os.Stderr, "  "+f)
		}
		os.Exit(1)
	}
}

// summarize prints a run's packages, shortened when there are many.
func summarize(patterns []string) string {
	if len(patterns) <= 6 {
		return strings.Join(patterns, " ")
	}
	return fmt.Sprintf("%s ... (%d packages)", strings.Join(patterns[:6], " "), len(patterns))
}

// changedFiles returns the repository paths that differ between ref and HEAD.
func changedFiles(root, ref string) ([]string, error) {
	cmd := exec.CommandContext(context.Background(), "git", "diff", "--name-only", ref, "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only %s HEAD: %w", ref, err)
	}
	return strings.Fields(string(out)), nil
}

// triggering returns the first changed path that affects every
// configuration, or "".
func triggering(changed []string) string {
	for _, p := range changed {
		base := filepath.Base(p)
		if base == "go.mod" || base == "go.sum" {
			return p
		}
		for _, a := range alwaysAffected {
			if p == a || strings.HasSuffix(a, "/") && strings.HasPrefix(p, a) {
				return p
			}
		}
	}
	return ""
}

// affected returns a changed path in a directory the run compiles, its own
// packages or any package they import, tests included; or "" when no
// changed path is in one. Every file in such a directory counts, Go or not,
// since a package can embed any of them.
func affected(root, modDir string, env []string, r run, changed []string) (string, error) {
	args := append([]string{"list", "-e", "-deps", "-test", "-tags", r.cfg.Tags, "-f", "{{.Dir}}"}, r.patterns...)
	cmd := exec.CommandContext(context.Background(), "go", args...)
	cmd.Dir = modDir
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go list -deps: %w\n%s", err, stderr.String())
	}
	dirs := map[string]bool{}
	for d := range strings.FieldsSeq(string(out)) {
		if rel, err := filepath.Rel(root, d); err == nil && !strings.HasPrefix(rel, "..") {
			dirs[filepath.ToSlash(rel)] = true
		}
	}
	for _, p := range changed {
		if dirs[filepath.ToSlash(filepath.Dir(p))] {
			return p, nil
		}
	}
	return "", nil
}

// compileExports compiles the export data golangci-lint reads for patterns
// and their dependencies, tests included, into the build cache. It links
// nothing.
func compileExports(modDir string, env []string, tags string, patterns []string) error {
	args := append([]string{"list", "-e", "-export", "-deps", "-test", "-tags", tags, "-f", "{{if .Error}}{{.ImportPath}}: {{.Error}}{{end}}"}, patterns...)
	cmd := exec.CommandContext(context.Background(), "go", args...)
	cmd.Dir = modDir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// workModules returns the module directories go.work lists, relative to root.
func workModules(root string) (map[string]bool, error) {
	cmd := exec.CommandContext(context.Background(), "go", "work", "edit", "-json")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go work edit -json: %w", err)
	}
	var w struct{ Use []struct{ DiskPath string } }
	if err := json.Unmarshal(out, &w); err != nil {
		return nil, fmt.Errorf("go work edit -json: %w", err)
	}
	mods := map[string]bool{}
	for _, u := range w.Use {
		mods[filepath.Clean(u.DiskPath)] = true
	}
	return mods, nil
}

// planModule lists the module under every configuration and returns the runs
// that compile each of its Go files once, and the files no configuration
// compiles.
func planModule(modDir string, modEnv []string) ([]run, []string, error) {
	covered := map[string]bool{}
	all := map[string]bool{}
	var runs []run
	for i, cfg := range buildconfigs.All {
		pkgs, err := list(modDir, append(slices.Clone(modEnv), cfg.Env...), cfg.Tags)
		if err != nil {
			return nil, nil, fmt.Errorf("[%s]: %w", cfg.Name, err)
		}
		var dirs []string
		for _, p := range pkgs {
			rel, err := filepath.Rel(modDir, p.Dir)
			if err != nil {
				return nil, nil, err
			}
			fresh := false
			for _, group := range [][]string{p.GoFiles, p.CgoFiles, p.TestGoFiles, p.XTestGoFiles} {
				for _, f := range group {
					key := filepath.Join(rel, f)
					all[key] = true
					if !covered[key] {
						covered[key] = true
						fresh = true
					}
				}
			}
			for _, f := range p.IgnoredGoFiles {
				ignored, err := ignoreTagged(filepath.Join(p.Dir, f))
				if err != nil {
					return nil, nil, err
				}
				if !ignored {
					all[filepath.Join(rel, f)] = true
				}
			}
			if fresh {
				dirs = append(dirs, "./"+filepath.ToSlash(rel))
			}
		}
		switch {
		case i == 0:
			runs = append(runs, run{cfg: cfg, patterns: []string{"./..."}})
		case len(dirs) > 0:
			sort.Strings(dirs)
			runs = append(runs, run{cfg: cfg, patterns: dirs})
		}
	}
	var uncovered []string
	for f := range all {
		if !covered[f] {
			uncovered = append(uncovered, f)
		}
	}
	sort.Strings(uncovered)
	return runs, uncovered, nil
}

// list runs `go list -e -json` over the module under one configuration.
func list(modDir string, env []string, tags string) ([]listedPackage, error) {
	cmd := exec.CommandContext(context.Background(), "go", "list", "-e", "-tags", tags,
		"-json=Dir,GoFiles,CgoFiles,TestGoFiles,XTestGoFiles,IgnoredGoFiles", "./...")
	cmd.Dir = modDir
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w\n%s", err, stderr.String())
	}
	var pkgs []listedPackage
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p listedPackage
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("go list output: %w", err)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

// ignoreTagged reports whether a file's build constraint names the `ignore`
// tag, the convention for a file no build compiles (a `go run` generator).
func ignoreTagged(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "//") && !constraint.IsGoBuild(line) {
			continue
		}
		if !constraint.IsGoBuild(line) {
			return false, nil
		}
		expr, err := constraint.Parse(line)
		if err != nil {
			return false, fmt.Errorf("%s: %w", path, err)
		}
		return names(expr, "ignore"), nil
	}
	return false, sc.Err()
}

// names reports whether a build constraint mentions tag anywhere.
func names(expr constraint.Expr, tag string) bool {
	switch e := expr.(type) {
	case *constraint.TagExpr:
		return e.Tag == tag
	case *constraint.NotExpr:
		return names(e.X, tag)
	case *constraint.AndExpr:
		return names(e.X, tag) || names(e.Y, tag)
	case *constraint.OrExpr:
		return names(e.X, tag) || names(e.Y, tag)
	}
	return false
}

// analyze runs the analyzer over patterns in modDir under one configuration.
// golangci-lint is looked up on PATH, and every finding is printed rather
// than the first few of each kind.
//
// The unused linter runs in the first configuration only. A later run compiles
// part of a package, and a symbol whose only use sits in a file that run
// leaves out would be reported as unused. The first run covers every package
// of the module, so unused still judges each symbol the default build holds.
func analyze(tool, modDir string, env []string, tags string, first bool, patterns []string) error {
	var cmd *exec.Cmd
	switch tool {
	case "lint":
		args := []string{"run", "--build-tags", tags, "--max-issues-per-linter=0", "--max-same-issues=0"}
		if !first {
			args = append(args, "--disable", "unused")
		}
		cmd = exec.CommandContext(context.Background(), "golangci-lint", append(args, patterns...)...)
	case "vet":
		cmd = exec.CommandContext(context.Background(), "go", append([]string{"vet", "-tags", tags}, patterns...)...)
	}
	cmd.Dir = modDir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "tagmatrix:", err)
	os.Exit(1)
}
