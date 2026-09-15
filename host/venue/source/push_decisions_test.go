package source

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/ref"
	bowrainconn "github.com/neokapi/neokapi/core/venue/connector"
	"github.com/neokapi/neokapi/host"
)

// A push sends the project's committed decision record when that record has
// changed since this client last sent it, and not otherwise. The decisions
// component a pull records is the venue's fold, which moves as a server run
// drafts; compared against it, the record read as changed on every push, so
// every push resent it and asserted a component the server had since moved.
func TestPush_SendsTheCommittedRecordOnlyWhenItChanged(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{Content: 99, Context: "ctx-server", Decisions: "dec-server"})
	conn := newRefConnector(t, srv.Server, "proj1")

	a := &host.App{}
	defer a.Shutdown()
	st, err := a.OpenProjectState(t.Context(), conn.project.Root)
	require.NoError(t, err)
	require.NoError(t, st.Put(t.Context(), approvedUnit("greeting", "fr")))
	require.NoError(t, st.Commit(t.Context()))

	_, err = conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, srv.decisionsSent, "the first push sends the committed record")

	_, err = conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, srv.decisionsSent, "an unchanged record is not sent again")

	require.NoError(t, st.Put(t.Context(), approvedUnit("farewell", "fr")))
	require.NoError(t, st.Commit(t.Context()))
	_, err = conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	require.Equal(t, 3, srv.decisionsSent, "a changed record is sent, whole")
}
