package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
)

// caskSource is a Homebrew cask with a doubled word in its description.
const caskSource = "cask \"kapi\" do\n  desc \"Desktop workbench for the the content\"\n  caveats \"Open a project to begin.\"\nend\n"

// sourcecodeFormatCheck checks a project whose one collection reads file with
// the sourcecode format under config. It runs from the project's directory
// with a relative project path, as `kapi check -p kapi.yaml` does, so the
// reader receives the file by its project path rather than an absolute one.
func sourcecodeFormatCheck(t *testing.T, file, config string) (check.Report, error) {
	t.Helper()
	a := sourcecodeApp(t, nil)
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, filepath.Dir(file)), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, file), []byte(caskSource), 0o644))
	recipe := "version: v1\nname: cask\ndefaults:\n  source_language: en\ncollections:\n  - name: cask\n    source_only: true\n    content:\n      - path: \"" + file + "\"\n        format: { name: sourcecode, config: " + config + " }\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	t.Chdir(root)
	cmd := executionCommand(t)
	cmd.Flags().String(projectFlagName, "kapi.yaml", "")
	return a.ComputeCheck(cmd, nil)
}

func TestSourcecodeFormatFindsTheGrammarFromTheFileName(t *testing.T) {
	t.Run("a cask recipe without a language reads and checks", func(t *testing.T) {
		report, err := sourcecodeFormatCheck(t, "Casks/kapi.rb", "{ nodePathPatterns: [desc, caveats] }")
		require.NoError(t, err)
		assert.NotEqual(t, check.VerdictDidNotRun, report.Verdict, report.DidNotRun)
		assert.Positive(t, report.Target.Blocks)
		var doubled []string
		for _, f := range report.Findings {
			if f.Rule == "hygiene.doubled-word" {
				doubled = append(doubled, f.Location.Block)
			}
		}
		assert.Equal(t, []string{"desc"}, doubled, "the description's doubled word is found")
	})

	t.Run("an extension no grammar reads fails naming the file and the languages", func(t *testing.T) {
		_, err := sourcecodeFormatCheck(t, "Casks/kapi.cask", "{ nodePathPatterns: [desc, caveats] }")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Casks/kapi.cask")
		assert.Contains(t, err.Error(), "ruby")
	})

	t.Run("a declared language reads a file whose extension names none", func(t *testing.T) {
		report, err := sourcecodeFormatCheck(t, "Casks/kapi.cask", "{ language: ruby, nodePathPatterns: [desc, caveats] }")
		require.NoError(t, err)
		assert.Positive(t, report.Target.Blocks)
	})

	t.Run("a declared language wins over the extension", func(t *testing.T) {
		_, err := sourcecodeFormatCheck(t, "Casks/kapi.rb", "{ language: python, nodePathPatterns: [desc, caveats] }")
		require.Error(t, err, "the declaration is read, though the extension names a grammar")
		assert.Contains(t, err.Error(), `"python"`)
		assert.Contains(t, err.Error(), "Casks/kapi.rb")
		assert.Contains(t, err.Error(), "ruby")
	})
}
