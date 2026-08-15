package backend

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/host/venue/schema"
)

// ProjectServer describes where a project's `kapi up` runs. A project whose
// recipe declares a `bowrain:` block is Bowrain-connected: the canonical run
// executes on the server. The desktop still runs the local engine for "Bring
// up to date", so the UI discloses the venue honestly rather than implying a
// remote run happened.
//
// The venue is read through KapiProject.Venue, which finds the block by the
// registered extension's venue flag rather than by key name — no AGPL bowrain
// dependency, since schema is the Apache-2.0 recipe vocabulary the desktop
// already imports.
type ProjectServer struct {
	// Connected reports whether the recipe carries a `bowrain:` block with a URL.
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
// A tab with no recipe, or a recipe without a venue block, returns a
// not-connected result (never an error) so the badge simply stays hidden. A
// malformed block reads as not connected too — the recipe editor / validation
// path is where a bad block is reported, not the venue badge.
func (a *App) GetProjectServer(tabID string) (*ProjectServer, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil {
		return &ProjectServer{}, nil
	}
	venue, ok := op.Project.Venue()
	if !ok || venue.URL == "" {
		return &ProjectServer{}, nil
	}
	info := schema.ParseProjectURL(venue.URL)
	host := info.ServerURL
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:] // strip the scheme for the compact badge label
	}
	return &ProjectServer{
		Connected: true,
		URL:       venue.URL,
		Host:      host,
		ServerURL: info.ServerURL,
	}, nil
}
