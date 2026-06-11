// Package skills embeds the canonical kapi Agent Skill into the binary. This is
// the single source of truth: the `kapi skills` command installs it into a
// project or user .claude/skills directory, and `make plugin-bundle` exports the
// same files into the Claude Code plugin bundle — so all distribution paths stay
// byte-identical. Each skill is a directory under data/ containing a SKILL.md
// (the router) plus progressive-disclosure reference files.
package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed all:data
var dataFS embed.FS

// Skill is one embedded Agent Skill.
type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"-"` // SKILL.md bytes (the router)
}

// List returns all embedded skills, sorted by name.
func List() ([]Skill, error) {
	entries, err := fs.ReadDir(dataFS, "data")
	if err != nil {
		return nil, fmt.Errorf("read embedded skills: %w", err)
	}
	var skills []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		s, err := Get(e.Name())
		if err != nil {
			return nil, err
		}
		skills = append(skills, s)
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, nil
}

// Get returns an embedded skill by directory name. Name comes from the SKILL.md
// frontmatter, falling back to the directory name.
func Get(name string) (Skill, error) {
	data, err := dataFS.ReadFile(path("data", name, "SKILL.md"))
	if err != nil {
		return Skill{}, fmt.Errorf("skill %q not found: %w", name, err)
	}
	content := string(data)
	fm := parseFrontmatter(content)
	skillName := fm["name"]
	if skillName == "" {
		skillName = name
	}
	return Skill{Name: skillName, Description: fm["description"], Content: content}, nil
}

// InstallTo copies the named skills (or all when names is empty) into baseDir,
// preserving each skill's full directory tree (SKILL.md plus reference files),
// and returns the file paths written. Files are byte-identical to the embedded
// source. names match the embedded directory name (e.g. "kapi").
func InstallTo(baseDir string, names []string) ([]string, error) {
	entries, err := fs.ReadDir(dataFS, "data")
	if err != nil {
		return nil, fmt.Errorf("read embedded skills: %w", err)
	}
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}

	var written []string
	matched := false
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := e.Name()
		if len(names) > 0 && !want[dir] {
			continue
		}
		matched = true
		w, err := copyTree(path("data", dir), filepath.Join(baseDir, dir))
		if err != nil {
			return nil, err
		}
		written = append(written, w...)
	}
	if len(names) > 0 && !matched {
		return nil, fmt.Errorf("no matching skills for %v", names)
	}
	return written, nil
}

// copyTree copies an embedded directory subtree to dst, returning the files
// written. Subdirectories (e.g. references/) are recreated.
func copyTree(srcDir, dst string) ([]string, error) {
	var written []string
	err := fs.WalkDir(dataFS, srcDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, srcDir), "/")
		target := filepath.Join(dst, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := dataFS.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
		written = append(written, target)
		return nil
	})
	return written, err
}

// parseFrontmatter extracts simple `key: value` pairs from a leading YAML
// frontmatter block delimited by `---` lines. Sufficient for name/description.
func parseFrontmatter(content string) map[string]string {
	out := map[string]string{}
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return out
	}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		before, after, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key := strings.TrimSpace(before)
		val := strings.TrimSpace(after)
		out[key] = val
	}
	return out
}

// path joins with forward slashes for embed.FS (which always uses "/").
func path(parts ...string) string {
	return strings.Join(parts, "/")
}
