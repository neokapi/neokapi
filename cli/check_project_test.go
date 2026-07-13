package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheck_BareInsideProjectChecksTheProject: `kapi check` with no positional
// files, run inside a project, checks the project's declared content — the way
// every other project-aware command works on the whole project when you give it
// nothing to narrow it to. (It used to demand a file and exit 1, while the docs
// promised project mode.)
func TestCheck_BareInsideProjectChecksTheProject(t *testing.T) {
	root, _ := writeVerifyProject(t)
	// A second source file, so "the project" is demonstrably more than one file
	// and carries a finding of its own.
	second := filepath.Join(root, "locales", "en", "extra.json")
	require.NoError(t, os.WriteFile(second, []byte(`{"tagline": "Ship  faster"}`), 0o644))
	t.Chdir(root)

	a := &App{}
	cmd := NewCheckCmd(a)

	report, err := a.ComputeCheck(cmd, nil)
	require.NoError(t, err, "a bare check inside a project must not demand a file")

	// Both declared source files were checked, not just one.
	files := map[string]bool{}
	for _, f := range report.Findings {
		files[f.Location.File] = true
	}
	assert.Contains(t, files, filepath.Join("locales", "en", "extra.json"),
		"the project's content is checked, and findings name files as the user would type them (relative)")
	assert.Positive(t, report.Summary.Findings)
	assert.Equal(t, "2 files", report.Target.File, "the report says what it checked")
}

// TestCheck_NamedFilesStillNarrow: passing files still checks exactly those —
// project expansion is the no-argument default, not an override.
func TestCheck_NamedFilesStillNarrow(t *testing.T) {
	root, _ := writeVerifyProject(t)
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "locales", "en", "extra.json"),
		[]byte(`{"tagline": "Ship  faster"}`), 0o644))
	t.Chdir(root)

	a := &App{}
	cmd := NewCheckCmd(a)

	named := filepath.Join("locales", "en", "extra.json")
	report, err := a.ComputeCheck(cmd, []string{named})
	require.NoError(t, err)

	assert.Equal(t, named, report.Target.File, "only the named file was checked")
	for _, f := range report.Findings {
		assert.Equal(t, named, f.Location.File)
	}
}

// TestCheck_BareOutsideProjectStillRequiresAFile: with no project to expand to,
// the file requirement stands — and the error says how to satisfy it.
func TestCheck_BareOutsideProjectStillRequiresAFile(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1") // no upward walk into a real recipe
	t.Chdir(t.TempDir())

	a := &App{}
	cmd := NewCheckCmd(a)

	_, err := a.ComputeCheck(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one file is required")
	assert.Contains(t, err.Error(), ".kapi project")
}

// TestCheck_BareWithTargetStillRequiresOneSource: --target is bilingual mode —
// one source paired with one translated target — so there is nothing to expand
// the project into, and it says so rather than silently checking every file.
func TestCheck_BareWithTargetStillRequiresOneSource(t *testing.T) {
	root, targetFile := writeVerifyProject(t)
	t.Chdir(root)

	a := &App{}
	cmd := NewCheckCmd(a)
	require.NoError(t, cmd.Flags().Set("target", targetFile))

	_, err := a.ComputeCheck(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one positional file")
}
