package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The terminology and context ledger parts ride in the commit manifest beside
// the content. A push the venue reports unchanged still commits when it
// carries either, since they have to land; with neither it takes the fast
// exit and commits nothing.
func TestPushCarriesItsParts(t *testing.T) {
	terms := &TermsEdits{Creates: []CreateConceptParams{{Domain: "ui", Terms: []TermInfo{{Text: "Sign up", Locale: "en"}}}}}
	ledger := []ContextObjectWire{{Name: "log/w1/000000000000000000000001.jsonl", Data: []byte("{}\n")}}

	for _, tc := range []struct {
		name       string
		opts       []PushOption
		wantCommit bool
		wantTerms  bool
		wantLedger int
		oldServer  bool
	}{
		{name: "a server that predates the parts refuses terms", opts: []PushOption{CarryTerms(terms)}, oldServer: true},
		{name: "a server that predates the parts refuses the ledger", opts: []PushOption{CarryContextObjects(ledger)}, oldServer: true},
		{name: "nothing beside an unchanged push", wantCommit: false},
		{name: "terminology edits", opts: []PushOption{CarryTerms(terms)}, wantCommit: true, wantTerms: true},
		{name: "ledger objects", opts: []PushOption{CarryContextObjects(ledger)}, wantCommit: true, wantLedger: 1},
		{name: "empty terminology edits carry nothing", opts: []PushOption{CarryTerms(&TermsEdits{})}, wantCommit: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var commit *PushCommitRequest
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/push/init"):
					resp := PushInitResponse{Status: "unchanged", Parts: PushParts}
					if tc.oldServer {
						resp.Parts = nil
					}
					_ = json.NewEncoder(w).Encode(resp)
				case strings.HasSuffix(r.URL.Path, "/push/commit"):
					commit = &PushCommitRequest{}
					_ = json.NewDecoder(r.Body).Decode(commit)
					w.WriteHeader(http.StatusAccepted)
					_ = json.NewEncoder(w).Encode(SyncPushResponse{PushID: "push1"})
				default:
					w.WriteHeader(http.StatusOK)
				}
			}))
			t.Cleanup(srv.Close)
			c := NewClaimTokenClient(srv.URL, "proj1", "tok")

			resp, err := c.Push(context.Background(), nil, nil, nil, nil, tc.opts...)
			if tc.oldServer {
				require.ErrorIs(t, err, ErrPartUnsupported, "a part an older server would drop is never sent")
				assert.Nil(t, commit)
				return
			}
			require.NoError(t, err)
			if !tc.wantCommit {
				assert.Nil(t, commit, "nothing to land, nothing committed")
				assert.Equal(t, PushUnchanged, resp.PushID)
				return
			}
			require.NotNil(t, commit)
			assert.Equal(t, "push1", resp.PushID)
			if tc.wantTerms {
				require.NotNil(t, commit.Terms)
				require.Len(t, commit.Terms.Creates, 1)
			} else {
				assert.Nil(t, commit.Terms)
			}
			assert.Len(t, commit.ContextObjects, tc.wantLedger)
		})
	}
}
