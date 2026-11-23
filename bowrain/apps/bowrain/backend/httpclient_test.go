package backend

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// hangingServer answers no request until the test ends. It reads the request
// body first: the server notices a closed client connection, and cancels the
// request's context, only once the body is read, and closing the server
// waits for this handler.
func hangingServer(t *testing.T) string {
	t.Helper()
	release := make(chan struct{})
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})
	return srv.URL
}

// A request to a server that never answers ends at the client's bound, so a
// save the page has given up on is not left in flight.
func TestFlowRequest_EndsAtTheTimeout(t *testing.T) {
	url := hangingServer(t)
	a := newTestApp(t)
	a.remote = newRemoteClient(50 * time.Millisecond)

	start := time.Now()
	err := a.flowDo(http.MethodPut, url+"/flows/f", "token", map[string]string{"name": "x"}, nil)
	require.Error(t, err)
	require.Less(t, time.Since(start), 5*time.Second)
}

// Shutting the app down cancels a request still in flight.
func TestFlowRequest_CanceledOnShutdown(t *testing.T) {
	url := hangingServer(t)
	a := newTestApp(t)
	a.remote = newRemoteClient(time.Minute)

	done := make(chan error, 1)
	go func() { done <- a.flowDo(http.MethodGet, url+"/flows", "token", nil, nil) }()
	time.Sleep(50 * time.Millisecond)
	require.NoError(t, a.ServiceShutdown())
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("the request outlived the app")
	}
}
