package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/cli/pluginhost"
)

// Wire the generic host-owned-model staging into the daemon launcher. pluginhost
// can't import cli (where EnsureModel lives), so it exposes a hook we fill in.
func init() { pluginhost.EnsureModelEnv = daemonModelEnv }

// daemonModelEnv stages a daemon plugin's declared default model and returns the
// env the daemon needs to find it. It is generic over any plugin that declares
// a `models` section:
//
//   - KAPI_<NAME>_MODELS_ROOT — the plugin's model-cache root
//     ($XDG_CACHE_HOME/kapi/models/<plugin>), so a plugin whose model is
//     runtime-selectable (e.g. sat) can resolve <root>/<id>/<version>/ for any
//     model the host has staged.
//   - KAPI_<NAME>_MODEL_DIR — the staged default model's directory, for the
//     common single-model case.
//
// The default is fetched + verified here (in the host process, so the progress
// bar renders on the user's terminal). Bundled defaults are skipped — they ship
// in the tarball and need no staging.
func daemonModelEnv(plugin *pluginhost.Plugin) (map[string]string, error) {
	if plugin.Manifest == nil || len(plugin.Manifest.Models) == 0 {
		return nil, nil
	}
	root, err := ModelCacheRoot()
	if err != nil {
		return nil, err
	}
	name := plugin.Name()
	upper := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	env := map[string]string{
		"KAPI_" + upper + "_MODELS_ROOT": filepath.Join(root, name),
	}

	asset, ok := plugin.Manifest.DefaultModel()
	if !ok || asset.Bundled {
		return env, nil // nothing to pre-stage
	}
	dir, err := EnsureModel(context.Background(), asset, ModelEnsureOptions{
		Plugin: name,
		Logf:   func(f string, a ...any) { fmt.Fprintf(os.Stderr, name+": "+f+"\n", a...) },
	})
	if err != nil {
		return nil, err
	}
	env["KAPI_"+upper+"_MODEL_DIR"] = dir
	return env, nil
}
