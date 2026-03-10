package project

import (
	"net/url"
	"strings"
)

// ProjectURLInfo contains the parts extracted from a compound project URL.
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

	// Extract path segments (skip leading empty segment from "/").
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
		// Single segment — treat as project ID.
		return ProjectURLInfo{
			ServerURL: serverURL,
			ProjectID: segments[0],
		}
	default:
		// Just a server URL with no path.
		return ProjectURLInfo{ServerURL: serverURL}
	}
}

// FormatProjectURL constructs a compound project URL from parts.
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
