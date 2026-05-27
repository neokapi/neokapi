package cli

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExitCode(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "test"}

	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil error returns ExitOK", nil, ExitOK},
		{"generic error returns ExitError", errors.New("something went wrong"), ExitError},
		{"context.Canceled returns ExitSignal", context.Canceled, ExitSignal},
		{"wrapped context.Canceled returns ExitSignal", errors.Join(errors.New("wrapper"), context.Canceled), ExitSignal},
		{"ErrQualityGate returns ExitGate", ErrQualityGate, ExitGate},
		{"ErrSilentExit returns ExitError", ErrSilentExit, ExitError},
		{"WithExitCode(ExitUsage) returns ExitUsage", WithExitCode(ExitUsage, errors.New("bad pattern")), ExitUsage},
		{"wrapped WithExitCode returns its code", fmt.Errorf("context: %w", WithExitCode(ExitUsage, errors.New("x"))), ExitUsage},
		{"WithExitCode(nil) is nil → ExitOK", WithExitCode(ExitUsage, nil), ExitOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ExitCode(cmd, tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestMapToolboxErr verifies the grep-style exit-code contract the toolbox
// utilities (kgrep/ksed/kcat) use: 0 on match, 1 on no-match, 130 on
// interrupt, and 2 on operational trouble — with the trouble message preserved.
func TestMapToolboxErr(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{Use: "kgrep"}

	// Match → nil → ExitOK.
	require.NoError(t, mapToolboxErr(nil))

	// No-match → ErrSilentExit passes through → ExitError (1), message suppressed.
	noMatch := mapToolboxErr(ErrSilentExit)
	require.ErrorIs(t, noMatch, ErrSilentExit)
	assert.Equal(t, ExitError, ExitCode(cmd, noMatch))

	// Interrupt → context.Canceled passes through → ExitSignal (130).
	cancelled := mapToolboxErr(context.Canceled)
	require.ErrorIs(t, cancelled, context.Canceled)
	assert.Equal(t, ExitSignal, ExitCode(cmd, cancelled))

	// Operational trouble → ExitUsage (2), underlying message preserved.
	trouble := mapToolboxErr(errors.New("invalid regexp"))
	assert.Equal(t, ExitUsage, ExitCode(cmd, trouble))
	assert.Equal(t, "invalid regexp", trouble.Error())
}

func TestSignalContext(t *testing.T) {
	t.Parallel()

	ctx, stop := SignalContext(context.Background())
	defer stop()

	// Context should not be cancelled initially.
	require.NoError(t, ctx.Err())

	// Calling stop should release signal notification resources without error.
	stop()
}
