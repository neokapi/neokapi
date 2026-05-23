// Package schema declares the bowrain extension schema for kapi recipes.
//
// The framework's core/project package is platform-neutral. This package
// adds bowrain-specific top-level keys (server, hooks, automations,
// assets, brand_voice) and per-content keys (collection, base, assets,
// asset_max_size) by registering decoders with core/project's extension
// registry.
//
// Blank-importing this package teaches a host binary to validate and
// round-trip bowrain recipes. The Go types (ServerSpec, HooksSpec, ...)
// are aliased back into bowrain/core/project for backwards compatibility.
package schema

import (
	"fmt"
	"net/url"
	"strings"
)

// ServerSpec captures the optional bowrain-server connection details for a
// project recipe. A recipe with no Server is a pure local project; a recipe
// with Server is bowrain-connected and can be operated by `kapi push`,
// `kapi pull`, etc.
//
// Only the connection coordinates live here. Lifecycle policy (hooks,
// automations) and content/governance features (assets, brand voice) are
// top-level on KapiProject — they describe project policy that may run
// regardless of which CLI is driving the recipe.
type ServerSpec struct {
	// URL is a compound project URL that encodes the server, workspace, and
	// project ID. Examples:
	//
	//	https://bowrain.example.com/my-team/abc123     (workspace project)
	//	https://bowrain.example.com/projects/abc123    (direct project)
	URL string `yaml:"url,omitempty" json:"url,omitempty"`

	// Stream determines which content stream to sync with.
	// Default: "$auto" — auto-detect from git branch / CI environment.
	// Set to a specific name (e.g. "v2.0") to always use that stream.
	// Empty is treated as "$auto".
	Stream string `yaml:"stream,omitempty" json:"stream,omitempty"`
}

// ServerURL returns the base server URL extracted from the compound URL.
func (s *ServerSpec) ServerURL() string {
	if s == nil {
		return ""
	}
	return ParseProjectURL(s.URL).ServerURL
}

// ProjectID returns the project ID extracted from the compound URL.
func (s *ServerSpec) ProjectID() string {
	if s == nil {
		return ""
	}
	return ParseProjectURL(s.URL).ProjectID
}

// Workspace returns the workspace slug extracted from the compound URL.
func (s *ServerSpec) Workspace() string {
	if s == nil {
		return ""
	}
	return ParseProjectURL(s.URL).Workspace
}

// Validate checks that the server spec is well-formed.
func (s *ServerSpec) Validate() error {
	if s == nil || s.URL == "" {
		return nil
	}
	u, err := url.Parse(s.URL)
	if err != nil {
		return fmt.Errorf("url: %q is not a valid URL: %w", s.URL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url: %q must use http or https", s.URL)
	}
	if u.Host == "" {
		return fmt.Errorf("url: %q must include a host", s.URL)
	}
	if info := ParseProjectURL(s.URL); info.ProjectID == "" {
		return fmt.Errorf("url: %q does not contain a project ID (expected <server>/<workspace>/<project> or <server>/projects/<project>)", s.URL)
	}
	return nil
}

// ProjectURLInfo holds the parts extracted from a compound project URL.
type ProjectURLInfo struct {
	ServerURL string
	Workspace string
	ProjectID string
}

// ParseProjectURL parses a compound project URL into its parts.
//
// Supported formats:
//
//	https://server.com/workspace/project-id   → workspace project
//	https://server.com/projects/project-id    → direct project (no workspace)
//	""                                        → empty (no server)
func ParseProjectURL(rawURL string) ProjectURLInfo {
	if rawURL == "" {
		return ProjectURLInfo{}
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return ProjectURLInfo{}
	}

	path := strings.Trim(u.Path, "/")
	segments := strings.Split(path, "/")

	serverURL := u.Scheme + "://" + u.Host

	switch {
	case len(segments) == 2 && segments[0] == "projects":
		return ProjectURLInfo{
			ServerURL: serverURL,
			ProjectID: segments[1],
		}
	case len(segments) == 2:
		return ProjectURLInfo{
			ServerURL: serverURL,
			Workspace: segments[0],
			ProjectID: segments[1],
		}
	case len(segments) == 1 && segments[0] != "":
		return ProjectURLInfo{
			ServerURL: serverURL,
			ProjectID: segments[0],
		}
	default:
		return ProjectURLInfo{ServerURL: serverURL}
	}
}

// FormatProjectURL constructs a compound project URL from its parts.
func FormatProjectURL(serverURL, workspace, projectID string) string {
	serverURL = strings.TrimRight(serverURL, "/")
	if serverURL == "" {
		return ""
	}

	switch {
	case workspace != "" && projectID != "":
		return serverURL + "/" + workspace + "/" + projectID
	case projectID != "":
		return serverURL + "/projects/" + projectID
	default:
		return serverURL
	}
}
