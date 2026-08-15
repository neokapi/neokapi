package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/neokapi/neokapi/core/ref"
)

// Ref fetches the server's freshness ref for the client's stream.
//
// This is the round trip a client with no cached ref pays, and the only one: a
// deleted cache, a fresh clone, a project re-pointed at another server all end
// here, and what comes back is everything the cache would have said. Push and
// pull carry the same value on answers they already owe, so the ordinary loop
// never calls this at all.
func (c *BowrainClient) Ref(ctx context.Context) (ref.Ref, error) {
	if c == nil {
		return ref.Ref{}, errors.New("bowrain: project is not connected to a server")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.streamPrefix()+"/ref", nil)
	if err != nil {
		return ref.Ref{}, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return ref.Ref{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return ref.Ref{}, NewStatusError("ref", resp.StatusCode, body)
	}
	var out ref.Ref
	return out, json.NewDecoder(resp.Body).Decode(&out)
}

// GovernanceMoved reports whether an error is the server refusing a governance
// write because the component it asserted had moved, and names the component.
//
// It is how a caller turns a 409 into the one instruction that resolves it —
// pull, then send again — rather than into a retry that can never succeed.
func GovernanceMoved(err error) (ref.Component, bool) {
	var status *StatusError
	if !errors.As(err, &status) || status.StatusCode != http.StatusConflict {
		return "", false
	}
	if status.Code != governanceMovedCode {
		return "", false
	}
	var detail struct {
		Component string `json:"component"`
	}
	if jsonErr := json.Unmarshal([]byte(status.Body), &detail); jsonErr != nil {
		return "", true
	}
	return ref.Component(detail.Component), true
}

// governanceMovedCode is the server's stable code for a compare-and-swap
// failure (bowrain/apierror.CodeGovernanceMoved). Spelled here rather than
// imported because bowrain/core is the framework-only module and may not depend
// on the server's.
const governanceMovedCode = "governance_moved"
