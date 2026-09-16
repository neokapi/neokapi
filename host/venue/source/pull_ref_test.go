package source

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/ref"
	bowrainconn "github.com/neokapi/neokapi/core/venue/connector"
)

// A pull asks the venue for its ref and the venue answers from the project's
// own rows, so what comes back is definite about them: an empty component there
// is the venue holding nothing, not the venue declining to say.
//
// Merging that answer instead of recording it leaves a component this project
// observed while the venue held something, and the venue now holds nothing. The
// next push asserts the old value, the venue refuses it, and the pull the
// refusal asks for cannot clear it, so every later push fails until someone
// deletes the ref cache by hand.

func TestPull_RecordsAnEmptiedDecisionsComponent(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{Content: 42, Context: "ctx-1", Terms: "trm-1", Decisions: "dec-1"})
	conn := newRefConnector(t, srv.Server, "proj1")
	defer conn.Close()

	_, err := conn.Pull(context.Background(), bowrainconn.PullOptions{})
	require.NoError(t, err)
	require.Equal(t, "dec-1", loadRefs(t, conn).Ref("main").Decisions, "the first pull records what the venue holds")

	// The venue's ledger is emptied, which is what a data reset that keeps the
	// project id looks like from here.
	srv.published.Decisions = ""
	_, err = conn.Pull(context.Background(), bowrainconn.PullOptions{})
	require.NoError(t, err)

	assert.Empty(t, loadRefs(t, conn).Ref("main").Decisions,
		"a venue holding no decisions says so, and this project records it")
}

func TestPull_RecordsAnEmptiedContextComponent(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{Content: 42, Context: "ctx-1", Terms: "trm-1", Decisions: "dec-1"})
	conn := newRefConnector(t, srv.Server, "proj1")
	defer conn.Close()

	_, err := conn.Pull(context.Background(), bowrainconn.PullOptions{})
	require.NoError(t, err)
	require.Equal(t, "ctx-1", loadRefs(t, conn).Ref("main").Context)

	srv.published.Context = ""
	_, err = conn.Pull(context.Background(), bowrainconn.PullOptions{})
	require.NoError(t, err)

	assert.Empty(t, loadRefs(t, conn).Ref("main").Context,
		"the same rule for the context the venue declares")
}

// Terminology is the exception, and it is deliberate: a content pull computes
// no terminology, so the empty terms component its ref carries says nothing.
// The value a terminology pull observed stands until a terminology pull moves
// it.
func TestPull_LeavesTerminologyToTheTerminologyPath(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{Content: 42, Context: "ctx-1", Terms: "trm-1", Decisions: "dec-1"})
	conn := newRefConnector(t, srv.Server, "proj1")
	defer conn.Close()

	_, err := conn.Pull(context.Background(), bowrainconn.PullOptions{})
	require.NoError(t, err)
	require.Equal(t, "trm-1", loadRefs(t, conn).Ref("main").Terms)

	srv.published.Terms = ""
	_, err = conn.Pull(context.Background(), bowrainconn.PullOptions{})
	require.NoError(t, err)

	assert.Equal(t, "trm-1", loadRefs(t, conn).Ref("main").Terms,
		"a content pull's empty terms component is silence, not an answer")
}
