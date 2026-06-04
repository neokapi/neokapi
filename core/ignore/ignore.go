// Package ignore provides .kapiignore file parsing and matching,
// following the same syntax as .gitignore.
//
// Default rules (always applied, even without a .kapiignore file):
//   - *.kapi        (project files are infrastructure, not content)
//   - .git/         (version control)
//   - .DS_Store     (macOS metadata)
//
// Additional patterns can be supplied via the KAPI_IGNORE environment
// variable (comma-separated list of patterns).
package ignore

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// defaultPatterns are always ignored, even without a .kapiignore file.
var defaultPatterns = []rule{
	{pattern: "*.kapi", dirOnly: false, negated: false},
	{pattern: ".git", dirOnly: false, negated: false},
	{pattern: ".DS_Store", dirOnly: false, negated: false},
}

// rule is a single parsed ignore rule.
type rule struct {
	pattern string
	dirOnly bool // trailing / in the original pattern
	negated bool // leading ! in the original pattern
}

// Matcher tests file paths against a set of ignore rules.
type Matcher struct {
	rules []rule
}

// New creates a Matcher with default rules only.
func New() *Matcher {
	m := &Matcher{}
	m.rules = append(m.rules, defaultPatterns...)
	return m
}

// LoadFile parses a .kapiignore file and adds its rules.
// Returns no error if the file doesn't exist.
func (m *Matcher) LoadFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		m.AddPattern(scanner.Text())
	}
	return scanner.Err()
}

// LoadEnv adds patterns from the KAPI_IGNORE environment variable
// (comma-separated).
func (m *Matcher) LoadEnv() {
	val := os.Getenv("KAPI_IGNORE")
	if val == "" {
		return
	}
	for p := range strings.SplitSeq(val, ",") {
		m.AddPattern(p)
	}
}

// AddPattern parses and adds a single pattern line.
func (m *Matcher) AddPattern(line string) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return
	}

	r := rule{}

	if strings.HasPrefix(line, "!") {
		r.negated = true
		line = line[1:]
	}

	if strings.HasSuffix(line, "/") {
		r.dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}

	r.pattern = line
	m.rules = append(m.rules, r)
}

// Match returns true if the path should be ignored.
// relPath must use forward slashes and be relative to the project root.
// isDir indicates whether the path is a directory.
func (m *Matcher) Match(relPath string, isDir bool) bool {
	// Normalize to forward slashes for matching.
	relPath = filepath.ToSlash(relPath)

	ignored := false
	for _, r := range m.rules {
		if r.dirOnly && !isDir {
			continue
		}
		if matchRule(r.pattern, relPath) {
			ignored = !r.negated
		}
	}
	return ignored
}

// matchRule checks whether a pattern matches a relative path.
// Supports:
//   - Simple globs: *.json, temp.*
//   - Directory patterns: build, node_modules
//   - Path patterns with /: src/generated
//   - ** for recursive matching: docs/**, **/*.tmp
func matchRule(pattern, relPath string) bool {
	// If the pattern contains no slash, match against every path component
	// as well as the full path (gitignore behaviour).
	if !strings.Contains(pattern, "/") {
		// Try matching the basename.
		base := relPath
		if i := strings.LastIndex(relPath, "/"); i >= 0 {
			base = relPath[i+1:]
		}
		if matchGlob(pattern, base) {
			return true
		}
		// Also try the full relative path for patterns like "foo".
		return matchGlob(pattern, relPath)
	}

	// Pattern contains a slash — match against the full relative path.
	return matchGlob(pattern, relPath)
}

// matchGlob matches a pattern against a string, supporting * and **.
func matchGlob(pattern, name string) bool {
	// Handle ** (matches zero or more path segments).
	if strings.Contains(pattern, "**") {
		return matchDoublestar(pattern, name)
	}

	// Use filepath.Match for simple globs (*, ?).
	matched, _ := filepath.Match(pattern, name)
	return matched
}

// matchDoublestar handles patterns containing **.
func matchDoublestar(pattern, name string) bool {
	// Split pattern on ** and try all possible segment combinations.
	prefix, suffix, _ := strings.Cut(pattern, "**")

	// Remove leading/trailing slashes from ** boundaries.
	prefix = strings.TrimSuffix(prefix, "/")
	suffix = strings.TrimPrefix(suffix, "/")

	if prefix == "" && suffix == "" {
		// Pattern is just "**" — matches everything.
		return true
	}

	if prefix == "" {
		// Pattern like "**/foo" or "**/*.json".
		// Match suffix against every possible tail of the path.
		segments := strings.Split(name, "/")
		for i := range segments {
			tail := strings.Join(segments[i:], "/")
			if matchGlob(suffix, tail) {
				return true
			}
		}
		return false
	}

	if suffix == "" {
		// Pattern like "foo/**".
		// Match if name starts with the prefix (or equals it).
		return name == prefix || strings.HasPrefix(name, prefix+"/")
	}

	// Pattern like "foo/**/bar" — prefix must match start,
	// suffix must match end of some sub-path.
	if !strings.HasPrefix(name, prefix+"/") && name != prefix {
		return false
	}
	rest := strings.TrimPrefix(name, prefix+"/")
	// Try matching suffix against every possible tail.
	segments := strings.Split(rest, "/")
	for i := range segments {
		tail := strings.Join(segments[i:], "/")
		if matchGlob(suffix, tail) {
			return true
		}
	}
	return false
}

// ForProjectDir creates a fully loaded Matcher for a project directory:
// default rules + .kapiignore + KAPI_IGNORE env var.
func ForProjectDir(dir string) *Matcher {
	m := New()
	_ = m.LoadFile(filepath.Join(dir, ".kapiignore"))
	m.LoadEnv()
	return m
}
