package connector

import (
	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/core/registry"
)

// RegisterAll registers all built-in connectors with the given registry —
// both the local-filesystem connectors (file, git) and the remote/CMS
// connectors (wordpress, figma, hubspot). This is the server/worker surface:
// the Bowrain server sources content from local checkouts on the host it runs
// on, so it needs every connector type.
//
// The desktop app must NOT use this. Under the product boundary, kapi owns
// local files + project configuration; the desktop's local footprint is a
// working copy / cache of the server, never a source of truth. The desktop
// uses [RegisterRemote] instead, which omits the local-filesystem connectors.
func RegisterAll(r *platconn.Registry, formatReg *registry.FormatRegistry) {
	registerLocal(r, formatReg)
	RegisterRemote(r)
}

// registerLocal registers the connectors that source content from the local
// filesystem of the process: the file connector (a directory of resource
// files) and the git connector (a cloned repository, read through a file
// connector internally). Both require a FormatRegistry for format detection
// and parsing. These are server-side only.
func registerLocal(r *platconn.Registry, formatReg *registry.FormatRegistry) {
	r.Register("file", platconn.CategoryFile, func(config map[string]string) (platconn.IntegrationConnector, error) {
		return NewFileConnector(formatReg, config)
	})

	r.Register("git", platconn.CategoryCode, func(config map[string]string) (platconn.IntegrationConnector, error) {
		return NewGitConnector(formatReg, config)
	})
}

// RegisterRemote registers the connectors that source content from remote
// systems over the network (no local filesystem): the CMS, design, and
// marketing integrations. Both the server and the desktop app may offer these,
// since they never treat local files as a source of truth.
func RegisterRemote(r *platconn.Registry) {
	r.Register("wordpress", platconn.CategoryCMS, func(config map[string]string) (platconn.IntegrationConnector, error) {
		return NewWordPressConnector(config)
	})

	r.Register("figma", platconn.CategoryDesign, func(config map[string]string) (platconn.IntegrationConnector, error) {
		return NewFigmaConnector(config)
	})

	r.Register("hubspot", platconn.CategoryMarketing, func(config map[string]string) (platconn.IntegrationConnector, error) {
		return NewHubSpotConnector(config)
	})

	r.Register("google-workspace", platconn.CategoryProductivity, func(config map[string]string) (platconn.IntegrationConnector, error) {
		return NewGoogleWorkspaceConnector(config)
	})

	r.Register("microsoft365", platconn.CategoryProductivity, func(config map[string]string) (platconn.IntegrationConnector, error) {
		return NewMicrosoft365Connector(config)
	})
}
