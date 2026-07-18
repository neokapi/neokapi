package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/billing"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/forge"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// The GitHub App post-install setup surface: after someone installs the app,
// GitHub redirects them to the web app's setup page with the installation id;
// these endpoints let that page list the repositories the installation covers
// (annotated with existing bindings) and bind a repository to a project —
// which is creating an `auth: app` forge connector through the same
// persistence path as the generic connectors API.

// InstallationRepoInfo is one repository of an installation, annotated with
// its existing binding when a forge connector already tracks it.
type InstallationRepoInfo struct {
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	ConnectorID   string `json:"connector_id,omitempty"`
	ProjectID     string `json:"project_id,omitempty"`
}

// BindInstallationRepoRequest binds one repository of an installation to a
// project.
type BindInstallationRepoRequest struct {
	Repository string `json:"repository"` // owner/name, as listed
	ProjectID  string `json:"project_id"`
	Branch     string `json:"branch,omitempty"`   // default: the repo's default branch
	Patterns   string `json:"patterns,omitempty"` // comma-separated content globs
	Name       string `json:"name,omitempty"`     // display name; default: the repo name
}

// HandleListInstallationRepos returns the repositories a GitHub App
// installation covers: GET /:ws/github/installations/:installationID/repositories.
func (s *Server) HandleListInstallationRepos(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageConnectors); err != nil {
		return err
	}
	if s.GitHubApp == nil {
		return apiErr(c, http.StatusServiceUnavailable, "the server has no GitHub App configured")
	}
	instID, err := strconv.ParseInt(c.Param("installationID"), 10, 64)
	if err != nil {
		return apiErr(c, http.StatusBadRequest, "installation id must be numeric")
	}

	repos, err := s.GitHubApp.ListInstallationRepos(c.Request().Context(), instID)
	if err != nil {
		return apiErr(c, http.StatusBadGateway, err.Error())
	}

	// Annotate with existing bindings so the setup page shows what is already
	// connected instead of offering a duplicate.
	bound := map[string]bstore.ConnectorConfig{}
	if s.ConnectorConfigStore != nil {
		wsID, _ := c.Get("workspace_id").(string)
		configs, err := s.ConnectorConfigStore.List(c.Request().Context(), wsID)
		if err == nil {
			for _, cfg := range configs {
				if cfg.Type != "forge" {
					continue
				}
				if repo, err := forge.ParseRepo(cfg.Config["repo"]); err == nil {
					bound[strings.ToLower(repo.Path)] = cfg
				}
			}
		}
	}

	out := make([]InstallationRepoInfo, 0, len(repos))
	for _, r := range repos {
		info := InstallationRepoInfo{
			FullName:      r.FullName,
			DefaultBranch: r.DefaultBranch,
			Private:       r.Private,
		}
		if cfg, ok := bound[strings.ToLower(r.FullName)]; ok {
			info.ConnectorID = cfg.ID
			info.ProjectID = cfg.Config["project_id"]
		}
		out = append(out, info)
	}
	return c.JSON(http.StatusOK, out)
}

// HandleDetectInstallationRepo inspects one repository of an installation for
// the setup wizard: GET /:ws/github/installations/:installationID/repositories/:owner/:name/detect.
// It reads the repository's file listing through the git trees API (no clone),
// and returns monorepo markers, i18n/content signals, a proposed `patterns`
// value for the bind request, and the files that proposal matches. Optional
// query parameters: `scope` confines detection to a subdirectory (monorepo
// workspace), `patterns` (comma-separated) overrides the proposal for match
// counting — live feedback while the user edits the patterns input — and
// `branch` picks the ref (default: the repository's HEAD).
func (s *Server) HandleDetectInstallationRepo(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageConnectors); err != nil {
		return err
	}
	if s.GitHubApp == nil {
		return apiErr(c, http.StatusServiceUnavailable, "the server has no GitHub App configured")
	}
	instID, err := strconv.ParseInt(c.Param("installationID"), 10, 64)
	if err != nil {
		return apiErr(c, http.StatusBadRequest, "installation id must be numeric")
	}
	repoPath := c.Param("owner") + "/" + c.Param("name")

	// The installation token scopes the tree read: a repository outside the
	// installation comes back as GitHub's own not-found.
	paths, truncated, err := s.GitHubApp.RepoTreePaths(
		c.Request().Context(), instID, repoPath, c.QueryParam("branch"), forge.MaxDetectTreeEntries)
	if err != nil {
		return apiErr(c, http.StatusBadGateway, err.Error())
	}

	det := forge.DetectRepoContent(paths, forge.DetectOptions{
		Scope:            c.QueryParam("scope"),
		PatternsOverride: c.QueryParam("patterns"),
	})
	det.Truncated = det.Truncated || truncated
	return c.JSON(http.StatusOK, det)
}

// HandleBindInstallationRepo binds one installed repository to a project:
// POST /:ws/github/installations/:installationID/repositories. It creates a
// persisted `auth: app` forge connector and kicks off the initial ingest in
// the background, so the project fills immediately — the delivery loop takes
// over from the next push.
func (s *Server) HandleBindInstallationRepo(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageConnectors); err != nil {
		return err
	}
	if s.GitHubApp == nil {
		return apiErr(c, http.StatusServiceUnavailable, "the server has no GitHub App configured")
	}
	if s.Services == nil {
		return apiErr(c, http.StatusServiceUnavailable, "store not configured")
	}
	// The forge connector is the sold git feature, same as the generic API.
	if err := billing.RequireFeature(c, billing.FeatureConnectorsGit, s.billingGuardEvent()); err != nil {
		return err
	}
	instID, err := strconv.ParseInt(c.Param("installationID"), 10, 64)
	if err != nil {
		return apiErr(c, http.StatusBadRequest, "installation id must be numeric")
	}

	var req BindInstallationRepoRequest
	if err := c.Bind(&req); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	if req.Repository == "" || req.ProjectID == "" {
		return apiErr(c, http.StatusBadRequest, "repository and project_id are required")
	}

	// The repository must actually belong to this installation — the binding
	// inherits the app's authority, so the claim is verified against GitHub,
	// never taken from the request.
	repos, err := s.GitHubApp.ListInstallationRepos(c.Request().Context(), instID)
	if err != nil {
		return apiErr(c, http.StatusBadGateway, err.Error())
	}
	var match *forge.InstallationRepo
	for i := range repos {
		if strings.EqualFold(repos[i].FullName, req.Repository) {
			match = &repos[i]
			break
		}
	}
	if match == nil {
		return apiErr(c, http.StatusNotFound, "the installation does not cover that repository")
	}

	branch := req.Branch
	if branch == "" {
		branch = match.DefaultBranch
	}
	name := req.Name
	if name == "" {
		name = match.FullName[strings.LastIndex(match.FullName, "/")+1:]
	}
	config := map[string]string{
		"auth":       "app",
		"repo":       "https://github.com/" + match.FullName + ".git",
		"branch":     branch,
		"project_id": req.ProjectID,
		"name":       name,
	}
	if req.Patterns != "" {
		config["patterns"] = req.Patterns
	}

	wsID, _ := c.Get("workspace_id").(string)
	conn, err := s.Services.Connector.AddConnector(wsID, "forge", config)
	if err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	if s.ConnectorConfigStore != nil {
		if _, err := s.ConnectorConfigStore.Upsert(c.Request().Context(), &bstore.ConnectorConfig{
			ID:          conn.ID(),
			WorkspaceID: wsID,
			Type:        "forge",
			Name:        conn.Name(),
			Config:      config,
		}); err != nil {
			_ = s.Services.Connector.RemoveConnector(wsID, conn.ID())
			return apiErr(c, http.StatusInternalServerError, err.Error())
		}
	}

	// The freshly bound repository ingests immediately, so the project fills
	// without waiting for the next push. A fetch clones the repository — far
	// too slow to block the bind response — so it runs exactly like a webhook
	// ingest: as a durable job (or the in-process fallback), ending in
	// EventPushCompleted, which starts the first convergence run. A fetch
	// failure never rolls the bind back; it lands on the connector's status
	// (last error) for the setup page and the connectors panel to surface.
	s.forgeIngest(c.Request().Context(), bstore.ConnectorConfig{
		ID:          conn.ID(),
		WorkspaceID: wsID,
		Type:        "forge",
		Name:        conn.Name(),
		Config:      config,
	}, forge.PushEvent{Branch: branch, RepoPath: match.FullName}, "github-setup")

	return c.JSON(http.StatusCreated, map[string]string{
		"connector_id":  conn.ID(),
		"repository":    match.FullName,
		"project_id":    req.ProjectID,
		"branch":        branch,
		"initial_fetch": "started",
	})
}
