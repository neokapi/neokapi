package cli

import (
	"context"
	"errors"
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ExitCode(cmd, tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
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
