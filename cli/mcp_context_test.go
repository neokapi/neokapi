package cli

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewMCPCmd_RunsWithoutACobraContext is the regression test for the crash
// the kapi-bowrain plugin's `mcp-server` hit on every start: it builds this
// command and calls RunE directly, so the command never went through cobra's
// Execute and carries no context. server.Run dereferences the context on its
// first line, so the plugin's whole MCP surface died with a segfault before
// serving anything, which is why nothing had ever reached it.
//
// The command serves on os.Stdin, so the test hands it a closed pipe: the
// transport reads EOF and Run returns.
func TestNewMCPCmd_RunsWithoutACobraContext(t *testing.T) {
	app := &App{}
	app.InitRegistries()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	require.NoError(t, w.Close())
	realStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = realStdin
		_ = r.Close()
	})

	cmd := NewMCPCmd(app, "kapi-test")
	require.Nil(t, cmd.Context(), "the command was built, never executed")

	// The goroutine reports what happened rather than what it returned: the
	// regression is a panic, and RunE's own error is beside the point.
	panicked := make(chan any, 1)
	go func() {
		defer func() { panicked <- recover() }()
		_ = cmd.RunE(cmd, nil)
	}()

	select {
	case p := <-panicked:
		assert.Nil(t, p, "serving with no cobra context panicked")
	case <-time.After(30 * time.Second):
		t.Fatal("the MCP command did not return on stdin EOF")
	}
}
