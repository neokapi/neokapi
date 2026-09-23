package cli

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestContentMemoryFlags pins the content-memory opt-out flags on the public
// CLI surface: `--help` teaches the current name, and the retired `tm`
// spelling is gone rather than hidden (CLAUDE.md: a `tm` identifier is a
// leftover).
func TestContentMemoryFlags(t *testing.T) {
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
			require.NoError(t, cmd.Flags().Parse([]string{"--" + tt.current}))
			assert.True(t, BoolFlag(cmd, tt.current), "--%s must opt out of the content memory", tt.current)

			assert.Nil(t, cmd.Flags().Lookup(tt.retired), "--%s is retired", tt.retired)
		})
	}
}
