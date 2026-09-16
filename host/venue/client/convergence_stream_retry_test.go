package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/convergence"
)

// A run outlives its connection, and the stream in front of it is a long quiet
// one. The connection drops, this client resumes from the last event id, and
// the gateway answers that resume with a status of its own while the run is
// still going. A 504 from a gateway says nothing about the run, so failing on
// it fails work the server is completing: the nightly of 2026-09-16 lost a run
// that way, four minutes after its last event.
func TestStreamConvergenceRunEvents_ResumesThroughATransientGatewayStatus(t *testing.T) {
	var mu sync.Mutex
	attempts := 0
	var resumeSeen []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		n := attempts
		resumeSeen = append(resumeSeen, r.Header.Get("Last-Event-ID"))
		mu.Unlock()

		switch n {
		case 1:
			// One event, then the connection closes with no terminal frame.
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, "id: 1\ndata: {\"type\":\"pass_start\",\"pass\":1}\n\n")
		case 2:
			// The gateway, not the server, answers the resume.
			w.WriteHeader(http.StatusGatewayTimeout)
			_, _ = fmt.Fprint(w, "<html><body>504 Gateway Timeout</body></html>")
		default:
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, "id: 2\ndata: {\"type\":\"done\",\"state\":\"converged\"}\n\n")
		}
	}))
	defer srv.Close()

	c := NewProjectBearerClient(srv.URL, "proj1", "tok")
	var seen []convergence.EventType
	err := c.StreamConvergenceRunEvents(context.Background(), "run1", func(ev convergence.Event) {
		seen = append(seen, ev.Type)
	})

	require.NoError(t, err, "a gateway's status on a resume is not the run's answer")
	assert.Contains(t, seen, convergence.EventDone, "the run's terminal frame still arrives")

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 3, attempts, "the drop and the gateway status are both resumed")
	require.Len(t, resumeSeen, 3)
	assert.Equal(t, "1", resumeSeen[2], "the retry resumes from the last event this client saw")
}

// A status about the run itself is the run's answer, and it stands.
func TestStreamConvergenceRunEvents_StopsWhenTheRunIsGone(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"error":"not_found","message":"run not found"}`)
	}))
	defer srv.Close()

	c := NewProjectBearerClient(srv.URL, "proj1", "tok")
	err := c.StreamConvergenceRunEvents(context.Background(), "run1", nil)

	require.Error(t, err)
	var status *StatusError
	require.ErrorAs(t, err, &status)
	assert.Equal(t, http.StatusNotFound, status.StatusCode)
	assert.Equal(t, 1, attempts, "a run that is gone is not asked for again")
}
