//go:build !js

package host

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// `kapi check --target` and MCP check_file with a target name both files, so a
// format with no reader fails them with the plugin to install.
func TestNamedTargetCheckWithNoReaderNamesThePlugin(t *testing.T) {
	root := reviewUnreadProject(t, true)
	t.Chdir(root)

	cmd := executionCommand(t)
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", filepath.Join(root, "kapi.yaml")))
	cmd.Flags().String("target", filepath.Join("pkg", "doc.fr.idml"), "")
	cmd.Flags().String("target-lang", "fr", "")
	_, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, []string{filepath.Join("pkg", "doc.idml")})
	requireInstallHint(t, err, "okapi-bridge")

	_, _, err = (&App{SourceLang: "en", mcpRecipePath: filepath.Join(root, "kapi.yaml")}).checkFileMCP(t.Context(), checkFileInput{
		File: filepath.Join(root, "pkg", "doc.idml"), Target: filepath.Join(root, "pkg", "doc.fr.idml"), TargetLang: string(model.LocaleID("fr")),
	})
	requireInstallHint(t, err, "okapi-bridge")
}
