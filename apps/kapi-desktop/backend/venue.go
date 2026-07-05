package backend

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/bowrain/plugin/schema"
)

// ProjectServer describes where a project's `kapi up` runs. A project whose
// recipe declares a `server:` block is Bowrain-connected: the canonical run
// executes on the server. The desktop still runs the local engine for "Bring
// up to date", so the UI discloses the venue honestly rather than implying a
// remote run happened.
//
// The field is decoded from the recipe's `server` extras key (the bowrain
// schema extension) without taking on any AGPL bowrain dependency — schema is
// the Apache-2.0 recipe vocabulary the desktop already blank-imports.
type ProjectServer struct {
	// Connected reports whether the recipe carries a `server:` block with a URL.
	Connected bool `json:"connected"`
	// URL is the compound project URL exactly as written in the recipe.
	URL string `json:"url,omitempty"`
	// Host is the server host (no scheme) — the short label the badge renders.
	Host string `json:"host,omitempty"`
	// ServerURL is the base server URL (scheme + host) extracted from the URL.
	ServerURL string `json:"serverURL,omitempty"`
}

// GetProjectServer reports the run venue for a project tab: whether the open
// recipe is connected to a Bowrain server and, if so, where. The desktop runs
// the local engine regardless; this only lets the home surfaces disclose that
// `kapi up` on a connected project canonically runs on the server.
//
// A tab with no recipe, or a recipe without a `server:` block, returns a
// not-connected result (never an error) so the badge simply stays hidden.
func (a *App) GetProjectServer(tabID string) (*ProjectServer, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil {
		return &ProjectServer{}, nil
	}
	node, ok := op.Project.Extras["server"]
	if !ok {
		return &ProjectServer{}, nil
	}
	var spec schema.ServerSpec
	if err := node.Decode(&spec); err != nil {
		// A malformed server block is surfaced quietly as "not connected" — the
		// recipe editor / validation path is where a bad block is reported, not
		// the venue badge.
		return &ProjectServer{}, nil
	}
	if spec.URL == "" {
		return &ProjectServer{}, nil
	}
	info := schema.ParseProjectURL(spec.URL)
	host := info.ServerURL
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:] // strip the scheme for the compact badge label
	}
	return &ProjectServer{
		Connected: true,
		URL:       spec.URL,
		Host:      host,
		ServerURL: info.ServerURL,
	}, nil
}
