// Package skills embeds the canonical kapi/bowrain Agent Skill definitions
// (SKILL.md files) into the binary. This is the single source of truth: the
// `kapi skills` command installs them into a project or user .claude/skills
// directory, and `make plugin-bundle` exports the same files into the Claude
// Code plugin bundle — so all distribution paths stay byte-identical.
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
	Family      string `json:"family"` // "kapi" or "bowrain"
	Content     string `json:"-"`      // full SKILL.md bytes
}

// List returns all embedded skills, sorted by name.
func List() ([]Skill, error) {
	var skills []Skill
	entries, err := fs.ReadDir(dataFS, "data")
	if err != nil {
		return nil, fmt.Errorf("read embedded skills: %w", err)
	}
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

// Get returns a single embedded skill by directory name.
func Get(name string) (Skill, error) {
	data, err := dataFS.ReadFile(path("data", name, "SKILL.md"))
	if err != nil {
		return Skill{}, fmt.Errorf("skill %q not found: %w", name, err)
	}
	content := string(data)
	fm := parseFrontmatter(content)
	family := "kapi"
	if strings.HasPrefix(name, "bowrain-") {
		family = "bowrain"
	}
	skillName := fm["name"]
	if skillName == "" {
		skillName = name
	}
	return Skill{
		Name:        skillName,
		Description: fm["description"],
		Family:      family,
		Content:     content,
	}, nil
}

// InstallTo writes the named skills (or all when names is empty) into
// baseDir/<name>/SKILL.md, returning the relative paths written. Files are
// written byte-for-byte from the embedded source.
func InstallTo(baseDir string, names []string) ([]string, error) {
	all, err := List()
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}

	var written []string
	for _, s := range all {
		dir := dirName(s)
		if len(names) > 0 && !want[s.Name] && !want[dir] {
			continue
		}
		outDir := filepath.Join(baseDir, dir)
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return nil, fmt.Errorf("create %s: %w", outDir, err)
		}
		outPath := filepath.Join(outDir, "SKILL.md")
		if err := os.WriteFile(outPath, []byte(s.Content), 0o644); err != nil {
			return nil, fmt.Errorf("write %s: %w", outPath, err)
		}
		written = append(written, outPath)
	}
	if len(names) > 0 && len(written) == 0 {
		return nil, fmt.Errorf("no matching skills for %v", names)
	}
	return written, nil
}

// dirName returns the install directory for a skill. The embedded directory
// name equals the skill's `name` frontmatter by convention.
func dirName(s Skill) string {
	return s.Name
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
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		out[key] = val
	}
	return out
}

// path joins with forward slashes for embed.FS (which always uses "/").
func path(parts ...string) string {
	return strings.Join(parts, "/")
}
