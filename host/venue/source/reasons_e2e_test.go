//go:build e2e

package source

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	bowrainconn "github.com/neokapi/neokapi/core/venue/connector"
)

// A reviewer rejects a server draft with reasons and a note. A checkout pulls
// the decision and keeps the reasons in its record, under the identity the
// server gives the decision. The push after the pull leaves the reasons on
// the server, and the push after that sends no decisions. A client that
// predates the field is covered by TestReviewDecision_AnEchoWithoutReasonsKeepsThem
// in bowrain/server and by the released plugin's run against this server.
func TestRevisionE2E_RejectionReasonsSurviveAPullAndAPush(t *testing.T) {
	commits := recordCommits(t)
	s := newE2ESession(t)
	c := s.checkout(t)
	up(t, c)
	s.push(t, c)

	resp := s.call(t, http.MethodPost, "/api/v1/"+s.ws+"/"+s.pid+"/actions/main/pseudo-translate?item="+url.QueryEscape(arbSource),
		`{"target_locale":"fr"}`)
	require.Equal(t, http.StatusOK, resp.code, resp.body)

	pr, err := c.client.Pull(context.Background(), 0, nil, 0)
	require.NoError(t, err)
	var bid, unit string
	for _, sb := range pr.Blocks {
		if sb.ItemName == arbSource && sb.Name == "greeting" {
			bid, unit = sb.ID, sb.Unit
		}
	}
	require.NotEmpty(t, bid)
	drafted := s.grade(t, c, "greeting")
	require.Equal(t, model.TargetStatusDraft, drafted.rung)

	set := fmt.Sprintf(`{"schema":"kapi.change/v1","note":"Too formal for a greeting.","ops":[`+
		`{"op":"decide","at":{"doc":%q,"block":%q,"edition":"fr"},"if_match":%q,"outcome":"reject","reasons":["tone","meaning"]}]}`,
		arbSource, bid, drafted.translation)
	resp = s.call(t, http.MethodPost, "/api/v1/"+s.ws+"/projects/"+s.pid+"/streams/main/changes", set)
	require.Equal(t, http.StatusOK, resp.code, resp.body)
	var res struct {
		Ops []struct {
			Status string `json:"status"`
		} `json:"ops"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.body), &res))
	require.Len(t, res.Ops, 1)
	require.Equal(t, "applied", res.Ops[0].Status, resp.body)

	rejected := s.grade(t, c, "greeting").record
	require.Equal(t, venue.ReviewStateRejected, rejected.ReviewState)
	require.Equal(t, []string{"meaning", "tone"}, rejected.Reasons, "the server holds the reasons, sorted")
	require.Equal(t, "Too formal for a greeting.", rejected.Note)

	// The checkout pulls the rejection, and its record holds the reasons.
	_, err = c.Pull(context.Background(), bowrainconn.PullOptions{})
	require.NoError(t, err)
	local, err := c.projectDecisions(context.Background())
	require.NoError(t, err)
	var mine *venue.UnitDecision
	for i := range local {
		// The checkout files the unit under its own key, which a push maps
		// back to the server's.
		if local[i].Variant == "fr" && local[i].ReviewState == venue.ReviewStateRejected {
			mine = &local[i]
		}
	}
	require.NotNil(t, mine, "the checkout's record holds the pulled rejection: %+v (server unit %s)", local, unit)
	assert.Equal(t, []string{"meaning", "tone"}, mine.Reasons)
	assert.Equal(t, "Too formal for a greeting.", mine.Note)
	assert.Equal(t, venue.DecisionIdentity(rejected), venue.DecisionIdentity(*mine),
		"the checkout and the server give the decision one identity")

	// The push after the pull sends what the pull wrote into the checkout's
	// file (a rejection without reasons does the same) and leaves the reasons
	// in place; the push after it has nothing to send.
	s.push(t, c)
	assert.Equal(t, []string{"meaning", "tone"}, s.grade(t, c, "greeting").record.Reasons)
	before := commits.withDecisions()
	s.push(t, c)
	assert.Equal(t, before, commits.withDecisions(), "the record the server holds is not sent again")
}

// commitLog counts the push commits that carried decisions.
type commitLog struct {
	mu        sync.Mutex
	decisions int
}

func (l *commitLog) withDecisions() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.decisions
}

type recordingTransport struct {
	next http.RoundTripper
	log  *commitLog
}

func (r recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil && strings.HasSuffix(req.URL.Path, "/push/commit") {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		var commit struct {
			Decisions []json.RawMessage `json:"decisions"`
		}
		if json.Unmarshal(body, &commit) == nil && len(commit.Decisions) > 0 {
			r.log.mu.Lock()
			r.log.decisions++
			r.log.mu.Unlock()
		}
	}
	return r.next.RoundTrip(req)
}

// recordCommits counts, for the rest of the test, the push commits the
// client sends with decisions in them.
func recordCommits(t *testing.T) *commitLog {
	t.Helper()
	log := &commitLog{}
	prev := http.DefaultTransport
	http.DefaultTransport = recordingTransport{next: prev, log: log}
	t.Cleanup(func() { http.DefaultTransport = prev })
	return log
}
