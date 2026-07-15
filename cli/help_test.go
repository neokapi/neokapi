package cli

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// TestStyledHelpDoesNotMutateGlobalColorMode guards issue #1248: rendering
// styled help resolves --color and passes it explicitly, so it must not leak
// that decision into output's process-global color mode. An embedded/in-process
// caller (Kapi Desktop, the wasm CLI, tests) that renders help and then keeps
// producing output through Print would otherwise inherit help's color choice.
func TestStyledHelpDoesNotMutateGlobalColorMode(t *testing.T) {
	output.SetColorMode(output.ColorNever)
	t.Cleanup(func() { output.SetColorMode(output.ColorAuto) })

	cmd := &cobra.Command{Use: "demo", Short: "demo"}
	cmd.Flags().String("color", "always", "colorize output")

	var buf bytes.Buffer
	require.NoError(t, renderStyledUsage(cmd, &buf))

	// The explicit mode is honored: --color=always produced ANSI.
	require.Contains(t, buf.String(), "\x1b[", "help should honor --color=always")

	// The global is untouched: a fresh global-mode Theme stays plain, proving
	// help did not flip the process-global to always.
	require.NotContains(t, output.Theme(&bytes.Buffer{}).Accent.Render("x"), "\x1b[",
		"help rendering leaked its --color decision into the global color mode")
}
