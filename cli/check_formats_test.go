package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFormatBoundProject creates a project whose recipe declares BOTH halves of
// a format binding the gate has to honour: a per-format reader config
// (defaults.formats.yaml.config.keyPathPatterns, which selects the only keys
// that hold prose) and a per-item format override (.md read as mdx). It binds a
// voice profile with one prohibited pattern so a block that should never have
// been read announces itself as a finding.
func writeFormatBoundProject(t *testing.T) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "demos"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))

	recipe := `version: v1
name: formats
defaults:
  source_language: en
  voice:
    profile_file: voice.yaml
  formats:
    yaml:
      config:
        keyPathPatterns:
          - title
collections:
  - name: demos
    content:
      - path: "demos/*.yaml"
        format:
          name: yaml
  - name: docs
    content:
      - path: "docs/*.md"
        format:
          name: mdx
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))

	profile := `name: formats
description: One pattern, so an unwanted block is visible as a finding.
tone:
  personality: [plain]
  formality: formal
style:
  prohibited_patterns:
    - regex: '\bseamless\b'
      description: Marketing superlative
      severity: critical
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "voice.yaml"), []byte(profile), 0o644))

	// `title` is prose the recipe selects; `command` is a shell line it does not.
	// Only the command carries the prohibited word.
	demo := `title: Northsea governance
command: ksed -i 's/our seamless integration/our unified integration/' index.html
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "demos", "demo.yaml"), []byte(demo), 0o644))

	// An MDX import line is prose to the markdown reader and structure to the
	// mdx one. The recipe binds mdx, so the import must not be read as content.
	doc := `# Title

import { Preview } from '@site/src/seamless';

A plain paragraph.
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "page.md"), []byte(doc), 0o644))
	return root
}

// TestCheck_ReadsProjectDeclaredFormatConfig: the gate reads each file under the
// reader config the project declared for it, so it judges the content the
// project declares as content and nothing else. Read under reader defaults, the
// YAML's `command:` line is a block like any other and the prohibited pattern in
// its sed expression fails the gate — text no convergence run ever touches.
func TestCheck_ReadsProjectDeclaredFormatConfig(t *testing.T) {
	root := writeFormatBoundProject(t)
	t.Chdir(root)

	a := &App{}
	report, err := a.ComputeCheck(NewCheckCmd(a), nil)
	require.NoError(t, err)

	for _, f := range report.Findings {
		assert.NotEqual(t, "voice.style", f.Rule,
			"the recipe's keyPathPatterns exclude `command:`, so its text is not checked: %+v", f.Location)
	}
	assert.Zero(t, report.Summary.Critical)
	assert.True(t, report.Pass)
}

// TestCheck_ReadsProjectDeclaredFormat: a per-item `format:` override decides
// which reader the gate uses. The markdown reader treats an MDX `import` line as
// a paragraph of prose; the recipe binds mdx, so the import is structure and
// never reaches the checkset.
func TestCheck_ReadsProjectDeclaredFormat(t *testing.T) {
	root := writeFormatBoundProject(t)
	t.Chdir(root)

	a := &App{}
	report, err := a.ComputeCheck(NewCheckCmd(a), []string{filepath.Join("docs", "page.md")})
	require.NoError(t, err)

	for _, f := range report.Findings {
		assert.NotContains(t, f.Location.Snippet, "import {",
			"the recipe binds the mdx reader, so an import line is structure, not prose")
	}
}

// TestCheck_FormatFlagStillOverridesTheProject: --format names one format for
// everything the caller passed, and it still wins over the recipe's binding.
func TestCheck_FormatFlagStillOverridesTheProject(t *testing.T) {
	root := writeFormatBoundProject(t)
	t.Chdir(root)

	a := &App{}
	cmd := NewCheckCmd(a)
	a.FormatFlag = "markdown"

	report, err := a.ComputeCheck(cmd, []string{filepath.Join("docs", "page.md")})
	require.NoError(t, err)
	assert.Positive(t, report.Target.Blocks, "the named override read the file")
}

// TestCheck_NamedFileSurvivesAnUnexpandablePattern: a recipe pattern that will
// not expand is a fault, and the whole-project check fails on it — but naming a
// file is the caller saying which file to check, and an unrelated broken pattern
// elsewhere in the recipe must not stop them.
func TestCheck_NamedFileSurvivesAnUnexpandablePattern(t *testing.T) {
	root := writeFormatBoundProject(t)
	recipe, err := os.ReadFile(filepath.Join(root, "kapi.yaml"))
	require.NoError(t, err)
	// An unclosed `[` is path.ErrBadPattern to doublestar.
	broken := string(recipe) + `  - name: broken
    content:
      - path: "docs/[unclosed*.md"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(broken), 0o644))
	t.Chdir(root)

	a := &App{}
	report, err := a.ComputeCheck(NewCheckCmd(a), []string{filepath.Join("docs", "page.md")})
	require.NoError(t, err, "a named file stays checkable when another pattern is broken")
	assert.Positive(t, report.Target.Blocks)

	// The whole-project check still refuses a content set it cannot account for.
	_, bareErr := a.ComputeCheck(NewCheckCmd(a), nil)
	require.Error(t, bareErr)
	assert.Contains(t, bareErr.Error(), "cannot be expanded")
}
