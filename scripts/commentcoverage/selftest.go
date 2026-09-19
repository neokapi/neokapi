package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// fixtureRecipe claims the Go files under core/ and the Markdown files at the
// root for their comments. It excludes testdata, and recipes, which the content
// resolver never claims. extra is appended to its exclude list.
func fixtureRecipe(extra string) string {
	return `version: v1
name: fixture
defaults:
  source_language: en
  exclude:
    - "**/testdata/**"
    - "**/kapi.yaml"
` + extra + `collections:
  - name: go
    source_only: true
    content:
      - path: "core/**/*.go"
        comments:
          only: true
  - name: markup
    source_only: true
    content:
      - path: "*.md"
        comments:
          only: true
`
}

// selfTestCase is one fixture repository and the status the guard must return.
// untrackedRecipe leaves the recipe out of git, so a case can track no file of
// any family.
type selfTestCase struct {
	name            string
	files           map[string]string
	exclude         string
	untrackedRecipe bool
	want            int
	names           string
}

var selfTestCases = []selfTestCase{
	{
		name:  "claimed and excluded files pass",
		files: map[string]string{"core/a.go": "package core\n", "core/testdata/b.go": "package testdata\n", "README.md": "# Fixture\n"},
		want:  0,
	},
	{
		name:  "must fail: a tracked .go file in an unlisted directory",
		files: map[string]string{"core/a.go": "package core\n", "newpkg/c.go": "package newpkg\n"},
		want:  1,
		names: "newpkg/c.go",
	},
	{
		name:    "an exclude covers the same file",
		files:   map[string]string{"core/a.go": "package core\n", "newpkg/c.go": "package newpkg\n"},
		exclude: "    - \"newpkg/**\"\n",
		want:    0,
	},
	{
		name:            "must fail: no tracked file in any family",
		files:           map[string]string{"notes.txt": "nothing to check\n"},
		untrackedRecipe: true,
		want:            1,
		names:           "no tracked Go",
	},
}

// runSelfTest runs each case in a fresh git repository and reports whether the
// guard returned the status the case requires.
func runSelfTest(w io.Writer) int {
	status := 0
	for _, c := range selfTestCases {
		got, out, err := runCase(c)
		switch {
		case err != nil:
			fmt.Fprintf(w, "✖ self-test: %s: %v\n", c.name, err)
			status = 1
		case got != c.want:
			fmt.Fprintf(w, "✖ self-test: %s: expected exit %d, got %d:\n%s", c.name, c.want, got, out)
			status = 1
		case c.names != "" && !strings.Contains(out, c.names):
			fmt.Fprintf(w, "✖ self-test: %s: output does not name %s:\n%s", c.name, c.names, out)
			status = 1
		default:
			fmt.Fprintf(w, "✓ self-test: %s\n", c.name)
		}
	}
	return status
}

func runCase(c selfTestCase) (int, string, error) {
	dir, err := os.MkdirTemp("", "commentcoverage-")
	if err != nil {
		return 0, "", err
	}
	defer os.RemoveAll(dir)
	files := map[string]string{"kapi.yaml": fixtureRecipe(c.exclude)}
	for rel, body := range c.files {
		files[rel] = body
	}
	for rel, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return 0, "", err
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return 0, "", err
		}
	}
	steps := [][]string{{"init", "-q"}, {"add", "-A"}}
	if c.untrackedRecipe {
		steps = append(steps, []string{"rm", "--cached", "-q", "kapi.yaml"})
	}
	for _, args := range steps {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			return 0, "", fmt.Errorf("git %s: %v: %s", args[0], err, out)
		}
	}
	var out bytes.Buffer
	return run(&out, dir, "kapi.yaml"), out.String(), nil
}
