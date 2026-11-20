package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmailConversion(t *testing.T) {
	source := filepath.Join("..", "core", "formats", "eml", "testdata", "plain.eml")
	for _, tc := range []struct{ extension, target, heading string }{
		{"md", "md", "# Project update"},
		{"dclg.xml", "", `<heading level="1">Project update</heading>`},
		{"html", "html", "<h1>Project update</h1>"},
	} {
		t.Run(tc.extension, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "result."+tc.extension)
			cmd := BusyboxRoot(newToolboxApp(t), "kconv")
			args := []string{source, "-o", out}
			if tc.target != "" {
				args = append(args, "--to", tc.target)
			}
			cmd.SetArgs(args)
			require.NoError(t, cmd.ExecuteContext(t.Context()))
			data, err := os.ReadFile(out)
			require.NoError(t, err)
			require.Contains(t, string(data), tc.heading)
			require.Contains(t, string(data), "The draft is ready.")
			require.NotContains(t, string(data), "Subject:")
		})
	}
}
