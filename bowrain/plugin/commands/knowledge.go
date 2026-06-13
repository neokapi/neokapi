package commands

import (
	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	"github.com/neokapi/neokapi/bowrain/core/project"
	bconn "github.com/neokapi/neokapi/bowrain/plugin/connector"
)

// knowledgeClient discovers the kapi project (upward walk from the cwd) and
// builds a workspace-scoped Bowrain REST client for the brand knowledge-graph
// read commands (`kapi concepts`, `kapi experiments`, `kapi terms pull`). The
// project, recipe, workspace, and auth token are resolved exactly like the
// sync commands (status.go/pull.go) via bconn.NewKnowledgeClient.
func knowledgeClient() (*project.Project, *apiclient.BowrainClient, error) {
	proj, err := project.FindProject("")
	if err != nil {
		return nil, nil, err
	}
	client, err := bconn.NewKnowledgeClient(proj)
	if err != nil {
		return nil, nil, err
	}
	return proj, client, nil
}
