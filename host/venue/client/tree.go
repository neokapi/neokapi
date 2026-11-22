package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/venue"
)

// Fetching the venue's tree, rather than asking it questions.
//
// The producer already keeps a mirror of what the venue holds — the sync cache
// is block key → hash per file, and the changed set is computed against it.
// What it could not do was trust that mirror on its own: the cache is not
// committed, so a fresh clone has none, and another machine may have pushed
// since. So it asked instead, once per item, sequentially, before any content
// moved.
//
// That was never a negotiation problem. It is a fetch problem, and the answer
// is the one git gives: fetch the tree, diff locally, send one pack. The ref
// the fetch returns is what makes the mirror trustworthy — the commit asserts
// it, so a tree that went stale between fetch and push is refused rather than
// acted on.

// errNotConnected is the same guard the push carries: callers build the client
// from the recipe's server: block, which is absent until the project is
// connected, and a nil-pointer panic in streamPrefix says nothing useful.
func errNotConnected() error {
	return errors.New("bowrain: project is not connected to a server. Run 'kapi init --server <url>' to connect")
}

// TreeResponse is the venue's content for a scope, reduced to hashes.
type TreeResponse struct {
	// Ref is the freshness ref this tree was computed at. A push built from
	// this tree asserts it at commit.
	Ref *ref.Ref `json:"ref,omitempty"`
	// RootHash folds the whole tree, so a caller holding a warm mirror can ask
	// whether anything moved without reading it back.
	RootHash string           `json:"root_hash"`
	Items    []venue.TreeItem `json:"items"`
}

// Tree returns the venue's tree for the stream, optionally narrowed to a scope.
//
// An empty scope asks for everything the stream holds — which is what a push
// declaring no scope needs, since it computes its diff against the venue's
// whole content and claims authority over none of it.
//
// A scope travels in the body of a POST. A push's scope excludes every file its
// patterns match that no format reads, and as a query string that passes the
// 8 KB URL limit of the CDN in front of a hosted server. A server that predates
// the POST route answers 404 or 405, and the scope is then sent as a query.
func (c *BowrainClient) Tree(ctx context.Context, scope venue.Scope) (*TreeResponse, error) {
	if c == nil {
		return nil, errNotConnected()
	}
	patterns := make([]string, 0, len(scope))
	for _, pat := range scope {
		if pat != "" {
			patterns = append(patterns, pat)
		}
	}
	if len(patterns) == 0 {
		return c.fetchTree(ctx, http.MethodGet, c.streamPrefix()+"/tree", nil)
	}
	body, err := json.Marshal(map[string][]string{"scope": patterns})
	if err != nil {
		return nil, err
	}
	tree, err := c.fetchTree(ctx, http.MethodPost, c.streamPrefix()+"/tree", body)
	var se *StatusError
	if err == nil || !errors.As(err, &se) ||
		(se.StatusCode != http.StatusNotFound && se.StatusCode != http.StatusMethodNotAllowed) {
		return tree, err
	}
	q := url.Values{}
	for _, pat := range patterns {
		q.Add("scope", pat)
	}
	return c.fetchTree(ctx, http.MethodGet, c.streamPrefix()+"/tree?"+q.Encode(), nil)
}

func (c *BowrainClient) fetchTree(ctx context.Context, method, endpoint string, body []byte) (*TreeResponse, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, NewStatusError("fetch tree", resp.StatusCode, b)
	}
	var out TreeResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode tree: %w", err)
	}
	return &out, nil
}

// Records is every transfer hash the venue's tree holds.
//
// Global rather than per item, which is what makes a rename free: a block that
// moved between files is content the venue already has, and asking about it
// per item would upload it again under the new name.
func (t *TreeResponse) Records() map[string]struct{} {
	out := map[string]struct{}{}
	if t == nil {
		return out
	}
	for _, item := range t.Items {
		for _, r := range item.Record {
			if r != "" {
				out[r] = struct{}{}
			}
		}
	}
	return out
}

// Tree renders the response as the shared tree shape.
func (t *TreeResponse) Tree() venue.Tree {
	out := venue.Tree{}
	if t == nil {
		return out
	}
	for _, item := range t.Items {
		out[item.Path] = item
	}
	return out
}
