package connector

import (
	"errors"
	"fmt"

	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	"github.com/neokapi/neokapi/bowrain/core/config"
)

// NewKnowledgeClient builds a workspace-scoped Bowrain REST client for reading
// the brand knowledge graph — concepts, the concept timeline, change-sets, and
// their blast radius (Bowrain AD-021).
//
// Unlike the sync connector, this always requires a claimed, workspace-scoped
// project: the graph lives on the workspace content group (/api/v1/:ws/...),
// not under a project, so a claim-token-only project cannot read it. It also
// requires a bearer token, resolved exactly like push/pull — the OS keychain
// after `kapi auth login`, or BOWRAIN_AUTH_TOKEN in CI (config.LoadAuth).
func NewKnowledgeClient(project *Project) (*apiclient.BowrainClient, error) {
	recipe := project.Recipe
	if !recipe.HasServer() {
		return nil, errors.New("no server configuration in the kapi recipe (add a `server:` block)")
	}

	serverURL := recipe.Server.ServerURL()
	projectID := recipe.Server.ProjectID()
	workspace := recipe.Server.Workspace()

	if serverURL == "" {
		return nil, errors.New("server URL not configured in the recipe's `server:` block")
	}
	if workspace == "" {
		return nil, errors.New("the brand knowledge graph is workspace-scoped — claim this project into a workspace (kapi auth claim) so its recipe URL is <server>/<workspace>/<project>")
	}

	authInfo, err := config.LoadAuth()
	if err != nil {
		return nil, errors.New("reading the workspace knowledge graph requires authentication: run 'kapi auth login' (or set BOWRAIN_AUTH_TOKEN in CI)")
	}
	if authInfo.ServerURL != "" && authInfo.ServerURL != serverURL {
		return nil, fmt.Errorf("auth token is for %s but project points to %s", authInfo.ServerURL, serverURL)
	}

	client := apiclient.NewWorkspaceBowrainClient(serverURL, workspace, projectID, authInfo.AccessToken)
	if authInfo.RefreshToken != "" {
		client.SetRefreshToken(authInfo.RefreshToken, func(newAccess, newRefresh string) {
			authInfo.AccessToken = newAccess
			authInfo.RefreshToken = newRefresh
			_ = config.SaveAuth(*authInfo)
		})
	}
	return client, nil
}
