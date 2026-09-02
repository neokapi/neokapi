package source

import (
	"errors"
	"fmt"

	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/host/venue/config"
	"github.com/neokapi/neokapi/host/venue/schema"
)

// ErrNotWorkspaceClaimed reports that a project is not claimed into a
// workspace, so there is no knowledge graph for it to sync with. It marks the
// one legitimate silent skip for terminology sync.
//
// It exists to separate that skip from every failure. A caller in the
// terminology fold that treats ANY error here as "nothing to do" —
// `return nil, nil` — turns an expired keychain token, or a token issued for a
// different server, into a silently content-only `kapi push`: the terms never
// travel, nothing says so, and `kapi status` goes on reporting the last
// successful snapshot.
var ErrNotWorkspaceClaimed = errors.New("project is not claimed into a workspace")

// NewKnowledgeClient builds a workspace-scoped Bowrain REST client for reading
// the brand knowledge graph — concepts, the concept timeline, change-sets, and
// their blast radius (Bowrain AD-021).
//
// Unlike the sync connector, this always requires a claimed, workspace-scoped
// project: the graph lives on the workspace content group (/api/v1/:ws/...),
// not under a project, so a claim-token-only project cannot read it. It also
// requires a bearer token, resolved exactly like push/pull — the OS keychain
// after `kapi auth login`, or BOWRAIN_AUTH_TOKEN in CI (config.LoadAuth).
//
// "Not claimed into a workspace" wraps ErrNotWorkspaceClaimed; everything else
// is a plain error and callers must treat it as a failure.
func NewKnowledgeClient(project *Project) (*apiclient.BowrainClient, error) {
	recipe := project.Recipe
	if !recipe.HasServer() {
		return nil, fmt.Errorf("%w: no server configuration in the kapi recipe (add a `%s:` block)", ErrNotWorkspaceClaimed, schema.VenueKey)
	}

	serverURL := config.NormalizeServerURL(recipe.Server.ServerURL())
	projectID := recipe.Server.ProjectID()
	workspace := recipe.Server.Workspace()

	if serverURL == "" {
		return nil, fmt.Errorf("%w: server URL not configured in the recipe's `%s:` block", ErrNotWorkspaceClaimed, schema.VenueKey)
	}
	if workspace == "" {
		return nil, fmt.Errorf("%w: the brand knowledge graph is workspace-scoped. Claim this project into a workspace (kapi auth claim) so its recipe URL is <server>/<workspace>/<project>", ErrNotWorkspaceClaimed)
	}

	// An absent or unreadable credential is NOT a skip. The project says it
	// belongs to a workspace; being unable to prove who we are is a failure to
	// reach that workspace, and reporting it as "nothing to sync" is how a
	// fortnight of terminology edits stayed on one laptop.
	authInfo, err := config.LoadAuth()
	if err != nil {
		return nil, errors.New("reading the workspace knowledge graph requires authentication: run 'kapi auth login' (or set BOWRAIN_AUTH_TOKEN in CI)")
	}
	if authServer := config.NormalizeServerURL(authInfo.ServerURL); authServer != "" && authServer != serverURL {
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
