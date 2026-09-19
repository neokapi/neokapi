// Command commentcoverage checks that the recipe reaches every tracked file in
// the families whose comments it checks. A tracked file must be claimed by a
// kapi.yaml collection or match a defaults.exclude pattern; any other file has
// comments no check reads, so the guard fails and names it. The exclude list is
// the only allowlist.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
)

// family is one kind of file whose comments the recipe checks.
type family struct {
	name string
	exts []string
}

var families = []family{
	{"Go", []string{".go"}},
	{"TypeScript, JavaScript and CSS", []string{".ts", ".tsx", ".js", ".mjs", ".cjs", ".css"}},
	{"YAML", []string{".yaml", ".yml"}},
	{"Markdown, MDX and HTML", []string{".md", ".mdx", ".html", ".htm"}},
}

func (f family) has(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, e := range f.exts {
		if ext == e {
			return true
		}
	}
	return false
}

// report is what one check found: the tracked files checked in each family,
// and those no collection claims and no exclude covers.
type report struct {
	checked   map[string]int
	unclaimed map[string][]string
}

// check holds the tracked files, relative to the recipe's directory, to what
// the recipe claims and excludes.
func check(recipe string, tracked []string) (report, error) {
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return report{}, fmt.Errorf("load %s: %w", recipe, err)
	}
	// A claim does not depend on a file's format, so no reader is registered.
	resolved, err := project.NewProjectContext(proj, recipe).ResolveContent(registry.NewFormatRegistry())
	if err != nil {
		return report{}, fmt.Errorf("resolve %s: %w", recipe, err)
	}
	claimed := make(map[string]bool, len(resolved))
	for _, rf := range resolved {
		claimed[filepath.ToSlash(rf.Relative)] = true
	}
	r := report{checked: map[string]int{}, unclaimed: map[string][]string{}}
	for _, path := range tracked {
		for _, f := range families {
			if !f.has(path) {
				continue
			}
			r.checked[f.name]++
			if !claimed[path] && !excluded(path, proj.Defaults.Exclude) {
				r.unclaimed[f.name] = append(r.unclaimed[f.name], path)
			}
		}
	}
	return r, nil
}

func excluded(path string, patterns []string) bool {
	for _, p := range patterns {
		if project.MatchGlob(p, path) {
			return true
		}
	}
	return false
}

// write prints the report and returns the exit status: 1 when a file is
// unclaimed, or when no file was checked at all.
func (r report) write(w io.Writer) int {
	total := 0
	var counts []string
	for _, f := range families {
		total += r.checked[f.name]
		counts = append(counts, fmt.Sprintf("%s %d", f.name, r.checked[f.name]))
	}
	if total == 0 {
		fmt.Fprintln(w, "✖ comment coverage: no tracked Go, code, YAML or markup file was checked")
		return 1
	}
	status := 0
	for _, f := range families {
		files := r.unclaimed[f.name]
		if len(files) == 0 {
			continue
		}
		status = 1
		sort.Strings(files)
		fmt.Fprintf(w, "✖ comment coverage: %d tracked %s file(s) belong to no kapi.yaml collection and match no defaults.exclude pattern:\n", len(files), f.name)
		for _, p := range files {
			fmt.Fprintf(w, "    %s\n", p)
		}
	}
	if status != 0 {
		fmt.Fprintln(w, "  Add the directory to a comment collection in kapi.yaml, or exclude it under defaults.exclude with a comment saying why.")
		return status
	}
	fmt.Fprintf(w, "✓ comment coverage: every tracked file is claimed or excluded (%s)\n", strings.Join(counts, ", "))
	return 0
}

// trackedFiles lists the files git tracks under root that exist in the working
// tree.
func trackedFiles(root string) ([]string, error) {
	out, err := exec.Command("git", "-C", root, "ls-files", "-z").Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var files []string
	for _, rel := range bytes.Split(out, []byte{0}) {
		if len(rel) == 0 {
			continue
		}
		if _, err := os.Lstat(filepath.Join(root, string(rel))); err == nil {
			files = append(files, string(rel))
		}
	}
	return files, nil
}

func run(w io.Writer, root, recipe string) int {
	abs, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(w, "✖ comment coverage:", err)
		return 1
	}
	tracked, err := trackedFiles(abs)
	if err != nil {
		fmt.Fprintln(w, "✖ comment coverage:", err)
		return 1
	}
	r, err := check(filepath.Join(abs, recipe), tracked)
	if err != nil {
		fmt.Fprintln(w, "✖ comment coverage:", err)
		return 1
	}
	return r.write(w)
}

func main() {
	root := flag.String("root", ".", "repository root")
	recipe := flag.String("recipe", "kapi.yaml", "recipe path, relative to the root")
	selfTest := flag.Bool("self-test", false, "prove the guard passes and fails on fixture repositories")
	flag.Parse()
	if *selfTest {
		os.Exit(runSelfTest(os.Stdout))
	}
	os.Exit(run(os.Stdout, *root, *recipe))
}
