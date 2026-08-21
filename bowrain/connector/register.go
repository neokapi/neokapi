package connector

import (
	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/bowrain/forge"
	"github.com/neokapi/neokapi/core/registry"
	venueconn "github.com/neokapi/neokapi/core/venue/connector"
)

// The registration functions below are split by *who supplies the config*, not
// by what a host is capable of. A connector registry turns a type name and a
// map of strings into a running connector, and on a multi-tenant server both
// of those come from a tenant: adding a connector needs only
// PermManageConnectors, which the built-in developer role carries. So a
// registry that can build a connector whose config names a filesystem path
// hands every tenant the host's filesystem, which is why the server registry
// has no such connector in it.
//
// There is no "register everything" function. The two surfaces differ by a
// security boundary, so naming which one you are is the point.

// RegisterServer registers the connectors a multi-tenant Bowrain server (and
// its ingest worker) may build from tenant-supplied configuration: the remote
// integrations, plus the forge connector, which reaches a repository over the
// forge's API and clones it into a location the server picks.
//
// It deliberately omits the local-filesystem connectors. Server-side paths
// that genuinely need a local checkout construct their connector directly in
// Go with a path the server chose — see fetchContextScanRepo in
// bowrain/jobs/brandscan_worker.go — which is not reachable from tenant input.
func RegisterServer(r *platconn.Registry, formatReg *registry.FormatRegistry) {
	RegisterRemote(r)
	registerForge(r, formatReg)
}

// RegisterLocal registers the connectors that source content from the local
// filesystem of the process: the file connector (a directory of resource
// files) and the git connector (a cloned repository, read through a file
// connector internally), alongside forge and the remote integrations.
//
// This is for a single-tenant host only — one where the filesystem the
// connector reads is already the operator's own, and the config comes from the
// operator rather than from a request. Never wire a multi-tenant server to it;
// use [RegisterServer].
//
// Under the product boundary the desktop app does not use this either: kapi
// owns local files and project configuration, and the desktop's local
// footprint is a working copy of the server, never a source of truth. The
// desktop uses [RegisterRemote].
func RegisterLocal(r *platconn.Registry, formatReg *registry.FormatRegistry) {
	r.Register("file", venueconn.CategoryFile, func(config map[string]string) (platconn.IntegrationConnector, error) {
		return NewFileConnector(formatReg, config)
	})
	r.Register("git", venueconn.CategoryCode, func(config map[string]string) (platconn.IntegrationConnector, error) {
		return NewGitConnector(formatReg, config)
	})
	registerForge(r, formatReg)
	RegisterRemote(r)
}

// registerForge registers the forge connector (GitHub / GitLab over the
// forge API, with branch-and-PR delivery).
func registerForge(r *platconn.Registry, formatReg *registry.FormatRegistry) {
	r.Register("forge", venueconn.CategoryCode, func(config map[string]string) (platconn.IntegrationConnector, error) {
		return NewForgeConnector(formatReg, config)
	})
}

// RegisterRemote registers the connectors that source content from remote
// systems over the network (no local filesystem): the CMS, design, and
// marketing integrations. Both the server and the desktop app may offer these,
// since they never treat local files as a source of truth.
//
// Their HTTP clients dial through the platform's SSRF policy (see
// bowrain/safehttp), because a base URL in a connector config is tenant input
// pointed at the network the server sits on.
func RegisterRemote(r *platconn.Registry) {
	r.Register("wordpress", venueconn.CategoryCMS, func(config map[string]string) (platconn.IntegrationConnector, error) {
		return NewWordPressConnector(config)
	})

	r.Register("figma", venueconn.CategoryDesign, func(config map[string]string) (platconn.IntegrationConnector, error) {
		return NewFigmaConnector(config)
	})

	r.Register("hubspot", venueconn.CategoryMarketing, func(config map[string]string) (platconn.IntegrationConnector, error) {
		return NewHubSpotConnector(config)
	})
}

// RegisterForgeApp re-registers the forge connector wired to the server's
// GitHub App, so configs with `auth: app` mint per-installation tokens
// instead of carrying one. Call after [RegisterServer] and before connector
// rehydration on servers that configure an app; static-token forge configs
// keep working unchanged.
func RegisterForgeApp(r *platconn.Registry, formatReg *registry.FormatRegistry, app *forge.GitHubApp) {
	r.Register("forge", venueconn.CategoryCode, func(config map[string]string) (platconn.IntegrationConnector, error) {
		return NewForgeConnectorWithApp(formatReg, config, app)
	})
}
