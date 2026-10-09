package host

import "github.com/neokapi/neokapi/core/project"

// RootEnvVars names every environment variable kapi resolves a root from: the
// data root and the workspace under it, the config root, the plugin roots,
// the caches, and the project binding. A kapi that sees these sees what the
// shell that set them sees; one that does not falls back to the platform
// defaults under HOME and keeps a second set of stores there.
//
// It is the one list. A launcher that cannot pass the whole environment
// through (Codex starts a stdio MCP server with a fixed set of variables and
// forwards others only by name) names these, and TestRootEnvVars_CoversEveryRootResolver
// holds the list to the resolvers, so a variable added to one of them cannot
// be left out here.
var RootEnvVars = []string{
	// The data root (host/datadir.go), which holds the workspace.
	EnvDataDir,
	"XDG_DATA_HOME",
	// The config root (host/resource.go, host/config, host/credentials), and
	// the XDG variable os.UserConfigDir reads under it on Linux.
	"KAPI_CONFIG_DIR",
	"XDG_CONFIG_HOME",
	// The plugin roots and the plugin discovery switch (host/pluginhost).
	"KAPI_PLUGINS_DIR",
	"KAPI_PLUGINS_DIR_ONLY",
	// The caches: the XDG cache root and each cache's own override
	// (host/pluginhost, host/pluginhost/registry, host/kpzcache.go,
	// host/modelassets.go).
	"XDG_CACHE_HOME",
	"KAPI_PLUGIN_CACHE",
	"KAPI_REGISTRY_CACHE",
	"KAPI_KPZ_CACHE",
	"KAPI_MODELS_CACHE",
	// The project binding (core/project/recipepath.go).
	project.ProjectEnvVar,
	project.NoProjectEnvVar,
}
