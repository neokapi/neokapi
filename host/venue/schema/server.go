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
// are aliased back into host/venue/project for backwards compatibility.
package schema

import (
	"fmt"
	"net/url"
	"os"
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

	// Converge is the server-side continuous-convergence policy: whether the
	// server starts a convergence run of its own when a push completes.
	//   on-push (default when empty) — every push that lands content triggers a
	//                                   server convergence run (the loop, with
	//                                   checks, gates, and parking).
	//   manual                        — a push never converges on its own; a run
	//                                   starts only from `kapi up` or the REST
	//                                   convergence-run endpoint.
	// Transport (`kapi push`) is pure regardless of this value — it moves state
	// and never produces translations; this policy governs the *server's* own
	// clock, the way "push triggers CI" is a repo policy, not a property of push.
	Converge ConvergePolicy `yaml:"converge,omitempty" json:"converge,omitempty"`
}

// ConvergePolicy is the server-side continuous-convergence policy of a
// connected project (server.converge).
type ConvergePolicy string

const (
	// ConvergeOnPush starts a server convergence run whenever a push completes.
	// It is the default for a connected project (an empty converge resolves to
	// it).
	ConvergeOnPush ConvergePolicy = "on-push"
	// ConvergeManual never converges on push; a run starts only on demand
	// (`kapi up`, or the REST convergence-run endpoint).
	ConvergeManual ConvergePolicy = "manual"
)

// ResolvedConverge returns the effective convergence policy, defaulting an
// empty value to on-push (the connected-project default).
func (s *ServerSpec) ResolvedConverge() ConvergePolicy {
	if s == nil || s.Converge == "" {
		return ConvergeOnPush
	}
	return s.Converge
}

// ProjectURLEnvVar redirects a bowrain-connected recipe at a different
// server/workspace/project, without editing the recipe.
//
// It exists for dogfooding. A repo whose committed recipe points at production
// cannot be iterated on locally without editing that recipe, and an edited
// recipe is one `git commit -a` away from redirecting everyone's pushes. This
// makes the local run an environment variable instead — the same shape as
// BOWRAIN_AUTH_TOKEN, which already supplies the credential the same way.
//
// It REDIRECTS, it does not CONNECT: a recipe with no server block stays a
// pure local project however this is set. Arming a project for a server is a
// decision that belongs in the recipe, in git, where it can be reviewed.
const ProjectURLEnvVar = "BOWRAIN_PROJECT_URL"

// resolvedURL is the compound project URL in force: the environment override
// when set and the recipe is already connected, otherwise the recipe's own.
func (s *ServerSpec) resolvedURL() string {
	if s == nil || s.URL == "" {
		return ""
	}
	if v := strings.TrimSpace(os.Getenv(ProjectURLEnvVar)); v != "" {
		return v
	}
	return s.URL
}

// ServerURL returns the base server URL extracted from the compound URL.
func (s *ServerSpec) ServerURL() string {
	return ParseProjectURL(s.resolvedURL()).ServerURL
}

// ProjectID returns the project ID extracted from the compound URL.
func (s *ServerSpec) ProjectID() string {
	return ParseProjectURL(s.resolvedURL()).ProjectID
}

// Workspace returns the workspace slug extracted from the compound URL.
func (s *ServerSpec) Workspace() string {
	return ParseProjectURL(s.resolvedURL()).Workspace
}

// Validate checks that the server spec is well-formed.
func (s *ServerSpec) Validate() error {
	if s == nil || s.URL == "" {
		return nil
	}
	// Validate the URL actually in force, so a malformed override is rejected
	// here rather than surfacing later as a confusing connection error.
	raw := s.resolvedURL()
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("url: %q is not a valid URL: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url: %q must use http or https", raw)
	}
	if u.Host == "" {
		return fmt.Errorf("url: %q must include a host", raw)
	}
	if info := ParseProjectURL(raw); info.ProjectID == "" {
		return fmt.Errorf("url: %q does not contain a project ID (expected <server>/<workspace>/<project> or <server>/projects/<project>)", raw)
	}
	switch s.Converge {
	case "", ConvergeOnPush, ConvergeManual:
	default:
		return fmt.Errorf("converge: %q is not valid (expected %q or %q)", s.Converge, ConvergeOnPush, ConvergeManual)
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
