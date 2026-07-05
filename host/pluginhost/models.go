package pluginhost

// EnsureModelEnv, when set by the host (the cli package), is invoked just before
// a Mode-C daemon plugin is spawned. It ensures the plugin's declared default
// model asset is staged — the host downloads + verifies it (with a progress
// bar), reusing the cache — and returns environment variables to inject into the
// daemon process (e.g. KAPI_<NAME>_MODELS_ROOT pointing at the plugin's model
// cache, and KAPI_<NAME>_MODEL_DIR for the pre-staged default).
//
// It is a hook rather than a direct call because cli.EnsureModel lives in the
// parent cli package, and pluginhost must not import cli (cli → pluginhost only).
// cli registers the implementation in an init().
var EnsureModelEnv func(plugin *Plugin) (map[string]string, error)

// ensureModelEnv runs the registered hook for a plugin that declares model
// assets, returning the env vars to add to its daemon. A no-op when no hook is
// registered or the plugin declares no models.
func ensureModelEnv(plugin *Plugin) (map[string]string, error) {
	if EnsureModelEnv == nil || plugin.Manifest == nil || len(plugin.Manifest.Models) == 0 {
		return nil, nil
	}
	return EnsureModelEnv(plugin)
}
