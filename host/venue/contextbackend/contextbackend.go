// Package contextbackend makes the bowrain context backend available to every
// project: a recipe with `context: {backend: bowrain}` shares its context with
// the Bowrain project its `bowrain:` block connects to, through
// host/venue/contextremote.
//
// Importing the package registers the "bowrain" backend with the host.
package contextbackend

import (
	"context"
	"errors"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/venue/contextremote"
)

func init() {
	host.RegisterContextRemote(project.ContextBackendBowrain, open)
}

// open builds the remote on the project the host resolved from the recipe's
// `bowrain:` block.
func open(_ context.Context, spec project.ContextBackend) (workspace.Remote, error) {
	if spec.Venue == nil {
		return nil, errors.New("bowrain: the backend was not resolved from the recipe's bowrain: block")
	}
	return contextremote.New(contextremote.Options{URL: spec.Venue.URL})
}
