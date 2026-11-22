// Command tagmatrix runs a Go analyzer over every file of the modules it is
// given, whatever build constraint gates the file.
//
// One run of golangci-lint or go vet compiles the files of one build
// configuration: one GOOS, one CGO_ENABLED and one tag set. A file behind
// `//go:build e2e`, `onnx`, `js && wasm` or `windows` is analyzed only by a
// run that selects it. tagmatrix runs the analyzer once per configuration in
// configs. The first run covers each module whole. Each later run covers only
// the package directories that still hold a file no earlier run compiled. A
// Go file that no configuration compiles fails the run, so a new build tag
// cannot leave its files unanalyzed.
//
//	go run ./scripts/tagmatrix -tool lint . host cli kapi
//	go run ./scripts/tagmatrix -tool vet plugins/sat
//	go run ./scripts/tagmatrix -plan .
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
)

// config is one build configuration the analyzer runs under.
type config struct {
	name string
	env  []string
	tags string
}

// testTags select the integration, acceptance and end-to-end suites. They
// combine with each other and with fts5.
const testTags = "fts5,integration,acceptance,e2e"

// configs are the build configurations, in order. The second holds the tags
// whose negation gates other files (`!onnx`, `!storage_oneconn`,
// `!notelemetry`), so they cannot join the first. The GOOS runs cover the
// files behind an operating system constraint the host does not satisfy, and
// the runs with CGO_ENABLED=0 cover the `!cgo` files. On a Linux runner the
// darwin run picks up the _darwin.go files; on a Mac the linux run picks up
// the _linux.go files. scripts/fieldguard type-checks under the same list.
var configs = []config{
	{name: "host", tags: testTags},
	{name: "host with the model tags", tags: testTags + ",onnx,satmodel,pdfium_experimental,storage_oneconn,notelemetry"},
	{name: "js/wasm", env: []string{"GOOS=js", "GOARCH=wasm"}, tags: testTags},
	{name: "windows", env: []string{"GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0"}, tags: testTags},
	{name: "linux without cgo", env: []string{"GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0"}, tags: testTags},
	{name: "darwin without cgo", env: []string{"GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=0"}, tags: testTags},
}

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
	cfg      config
	patterns []string
}

func main() {
	tool := flag.String("tool", "lint", "analyzer to run: lint (golangci-lint) or vet (go vet)")
	plan := flag.Bool("plan", false, "print the runs and exit without analyzing")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: go run ./scripts/tagmatrix [-tool lint|vet] [-plan] module-dir...")
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
	root, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	work, err := workModules(root)
	if err != nil {
		fail(err)
	}

	var failed []string
	for _, dir := range flag.Args() {
		dir = filepath.Clean(dir)
		modEnv := []string{}
		if !work[dir] {
			modEnv = append(modEnv, "GOWORK=off")
		}
		runs, uncovered, err := planModule(filepath.Join(root, dir), modEnv)
		if err != nil {
			fail(fmt.Errorf("%s: %w", dir, err))
		}
		if len(uncovered) > 0 {
			fmt.Fprintf(os.Stderr, "tagmatrix: %s: no build configuration compiles these files; add one to configs in scripts/tagmatrix/main.go:\n", dir)
			for _, f := range uncovered {
				fmt.Fprintln(os.Stderr, "  "+filepath.Join(dir, f))
			}
			failed = append(failed, dir+": uncovered files")
		}
		for i, r := range runs {
			fmt.Printf("tagmatrix: %s [%s] tags=%s %s\n", dir, r.cfg.name, r.cfg.tags, summarize(r.patterns))
			if *plan {
				continue
			}
			if err := analyze(*tool, filepath.Join(root, dir), append(slices.Clone(modEnv), r.cfg.env...), r.cfg.tags, i == 0, r.patterns); err != nil {
				failed = append(failed, fmt.Sprintf("%s [%s]", dir, r.cfg.name))
			}
		}
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
	for i, cfg := range configs {
		pkgs, err := list(modDir, append(slices.Clone(modEnv), cfg.env...), cfg.tags)
		if err != nil {
			return nil, nil, fmt.Errorf("[%s]: %w", cfg.name, err)
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
