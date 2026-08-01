package host

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// isolateTrust points the record at a throwaway config dir and clears the
// environment grant, so a test never reads or writes the developer's real
// decisions. ConfigDir honours KAPI_CONFIG_DIR, which is the same switch the
// in-repo isolation contract sets.
func isolateTrust(t *testing.T) {
	t.Helper()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("KAPI_TRUST_EXEC", "")
}

const execStepRecipe = `version: v1
flows:
  default:
    steps:
      - tool: external-command
        config:
          command: /bin/echo
`

func readerOf(s string) io.Reader { return strings.NewReader(s) }

func execProject(t *testing.T) *project.KapiProject {
	t.Helper()
	var p project.KapiProject
	require.NoError(t, yaml.Unmarshal([]byte(execStepRecipe), &p))
	return &p
}

// TestCheckExecToolAllowed_IgnoresOrdinaryTools: the backstop sits on the path
// every flow step takes, so it must cost nothing for tools that run no code.
func TestCheckExecToolAllowed_IgnoresOrdinaryTools(t *testing.T) {
	isolateTrust(t)
	a := &App{}
	for _, name := range []string{"translate", "term-check", "redact", "pseudo-translate"} {
		assert.NoError(t, a.checkExecToolAllowed(name), name)
	}
}

// TestCheckExecToolAllowed_RefusesWithoutADecision: a handful of internal paths
// load a recipe directly rather than through LoadProjectInteractive. Tool
// construction is the chokepoint they all share, so an exec-class tool built
// with no decision behind it is refused there rather than run.
func TestCheckExecToolAllowed_RefusesWithoutADecision(t *testing.T) {
	isolateTrust(t)
	a := &App{}
	for _, name := range []string{"external-command", "script"} {
		err := a.checkExecToolAllowed(name)
		require.Error(t, err, name)
		assert.ErrorIs(t, err, ErrExecNotTrusted)
	}
}

// TestCheckExecToolAllowed_HonoursTheProcessGrant: once the gate has run and
// the user has answered, the steps of that run build normally.
func TestCheckExecToolAllowed_HonoursTheProcessGrant(t *testing.T) {
	isolateTrust(t)
	a := &App{}
	require.NoError(t, a.ensureExecTrust(
		filepath.Join(t.TempDir(), project.RecipeFileName),
		execProject(t),
		LoadProjectInteractiveOptions{
			IsTTYFn: func() bool { return true },
			In:      readerOf("y\n"),
			Out:     io.Discard,
		}))
	assert.NoError(t, a.checkExecToolAllowed("external-command"))
}

// TestCheckExecToolAllowed_ReadsTheRecordForPathsThatSkipThePrompt: a project
// already approved keeps working on the internal paths that never reach the
// prompt, which is what stops the backstop from being a regression.
func TestCheckExecToolAllowed_ReadsTheRecordForPathsThatSkipThePrompt(t *testing.T) {
	isolateTrust(t)

	dir := t.TempDir()
	recipePath := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipePath, []byte(execStepRecipe), 0o644))
	proj := execProject(t)

	// A fresh App that never ran the gate, but with the project bound.
	a := &App{ProjectContext: project.NewProjectContext(proj, recipePath)}
	require.Error(t, a.checkExecToolAllowed("external-command"), "no record yet")

	require.NoError(t, recordExecTrust(recipePath,
		project.ExecSurfaceDigest(project.ExecSurface(proj)), execTrustAllow))

	b := &App{ProjectContext: project.NewProjectContext(proj, recipePath)}
	assert.NoError(t, b.checkExecToolAllowed("external-command"))
}

// TestExecTrustEnvGranted_NeedsAnAffirmativeValue: KAPI_NO_PROJECT and
// KAPI_PLUGINS_DIR_ONLY treat any non-empty value as set. That convention is
// wrong for a switch that grants code execution — reading "0" as yes is the
// wrong way to be wrong — so this one requires a real yes.
func TestExecTrustEnvGranted_NeedsAnAffirmativeValue(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"1", true}, {"true", true}, {"TRUE", true}, {"yes", true},
		// Surrounding whitespace survives an env var set from a shell script.
		{"  yes  ", true},
		{"", false}, {"0", false}, {"false", false}, {"no", false}, {"maybe", false},
	}
	for _, tc := range cases {
		t.Run("value="+tc.value, func(t *testing.T) {
			t.Setenv(execTrustEnvVar, tc.value)
			assert.Equal(t, tc.want, execTrustEnvGranted())
		})
	}
}

// TestExecTrustRecordIsOwnerOnly: the file records who may run code on this
// machine. Another local account must not be able to read it, and above all
// must not be able to add an entry.
func TestExecTrustRecordIsOwnerOnly(t *testing.T) {
	isolateTrust(t)
	require.NoError(t, recordExecTrust("/some/project/kapi.yaml", "digest", execTrustAllow))

	info, err := os.Stat(ExecTrustPath())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// TestExecTrustRecordSurvivesACorruptFile: the record is not authoritative
// state — losing it costs one prompt. Refusing to start because it is
// unparseable would be worse than re-asking.
func TestExecTrustRecordSurvivesACorruptFile(t *testing.T) {
	isolateTrust(t)
	require.NoError(t, os.MkdirAll(ConfigDir(), 0o700))
	require.NoError(t, os.WriteFile(ExecTrustPath(), []byte("{not json"), 0o600))

	_, found := lookupExecTrust("/some/project/kapi.yaml", "digest")
	assert.False(t, found)
	require.NoError(t, recordExecTrust("/some/project/kapi.yaml", "digest", execTrustAllow))
	decision, found := lookupExecTrust("/some/project/kapi.yaml", "digest")
	assert.True(t, found)
	assert.Equal(t, execTrustAllow, decision)
}
