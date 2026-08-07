package cli

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestContentMemoryFlagAliases pins the content-memory opt-out flags on the
// public CLI surface: `--help` teaches the current name, and the retired
// spelling still parses so existing invocations keep working.
func TestContentMemoryFlagAliases(t *testing.T) {
	tests := []struct {
		name    string
		build   func(*App) *cobra.Command
		current string
		retired string
	}{
		{
			name:    "extract",
			build:   func(a *App) *cobra.Command { return NewExtractCmd(a, ExtractCmdOptions{}) },
			current: "no-memory",
			retired: "no-tm",
		},
		{
			name:    "merge",
			build:   func(a *App) *cobra.Command { return NewMergeCmd(a, MergeCmdOptions{}) },
			current: "no-memory-update",
			retired: "no-tm-update",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.build(&App{})

			current := cmd.Flags().Lookup(tt.current)
			require.NotNil(t, current, "--%s must exist", tt.current)
			assert.False(t, current.Hidden, "--%s is the taught name", tt.current)

			retired := cmd.Flags().Lookup(tt.retired)
			require.NotNil(t, retired, "--%s must stay accepted", tt.retired)
			assert.True(t, retired.Hidden, "--%s must not appear in help", tt.retired)

			for _, flag := range []string{tt.current, tt.retired} {
				cmd := tt.build(&App{})
				require.NoError(t, cmd.Flags().Parse([]string{"--" + flag}))
				assert.True(t, BoolFlagAny(cmd, tt.current, tt.retired),
					"--%s must opt out of the content memory", flag)
			}
		})
	}
}
