package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/billing"
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
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "the server has no GitHub App configured"})
	}
	instID, err := strconv.ParseInt(c.Param("installationID"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "installation id must be numeric"})
	}

	repos, err := s.GitHubApp.ListInstallationRepos(c.Request().Context(), instID)
	if err != nil {
		return c.JSON(http.StatusBadGateway, ErrorResponse{Error: err.Error()})
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

// HandleBindInstallationRepo binds one installed repository to a project:
// POST /:ws/github/installations/:installationID/repositories. It creates a
// persisted `auth: app` forge connector — the delivery loop takes over from
// the next push.
func (s *Server) HandleBindInstallationRepo(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageConnectors); err != nil {
		return err
	}
	if s.GitHubApp == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "the server has no GitHub App configured"})
	}
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}
	// The forge connector is the sold git feature, same as the generic API.
	if err := billing.RequireFeature(c, billing.FeatureConnectorsGit, s.billingGuardEvent()); err != nil {
		return err
	}
	instID, err := strconv.ParseInt(c.Param("installationID"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "installation id must be numeric"})
	}

	var req BindInstallationRepoRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.Repository == "" || req.ProjectID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "repository and project_id are required"})
	}

	// The repository must actually belong to this installation — the binding
	// inherits the app's authority, so the claim is verified against GitHub,
	// never taken from the request.
	repos, err := s.GitHubApp.ListInstallationRepos(c.Request().Context(), instID)
	if err != nil {
		return c.JSON(http.StatusBadGateway, ErrorResponse{Error: err.Error()})
	}
	var match *forge.InstallationRepo
	for i := range repos {
		if strings.EqualFold(repos[i].FullName, req.Repository) {
			match = &repos[i]
			break
		}
	}
	if match == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "the installation does not cover that repository"})
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
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
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
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		}
	}

	return c.JSON(http.StatusCreated, map[string]string{
		"connector_id": conn.ID(),
		"repository":   match.FullName,
		"project_id":   req.ProjectID,
		"branch":       branch,
	})
}
