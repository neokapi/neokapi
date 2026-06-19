package cli

import (
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/cli/pluginhost"
	"github.com/neokapi/neokapi/core/av"
)

// The kapi-av plugin bundles ffmpeg/ffprobe (LGPL) per platform. Unlike kapi-asr/
// kapi-vision it runs no subprocess of its own — the demux engine (core/av) is
// in-process and just needs to find the bundled binaries. This wires a locator
// that core/av calls lazily on first video use, so an unrelated kapi command
// pays no discovery cost. Resolution order in core/av is SetBinDir → this
// locator → $KAPI_AV_DIR → PATH.
func init() {
	av.SetBinLocator(locateAVBundle)
}

const avPluginName = "av"

// locateAVBundle returns the directory of the discovered kapi-av plugin (which
// holds ffmpeg/ffprobe), or "" if it isn't installed (core/av then falls back to
// $KAPI_AV_DIR / PATH).
func locateAVBundle() string {
	for _, p := range pluginhost.Discover(pluginhost.DiscoverOptions{EnvPluginsDir: os.Getenv("KAPI_PLUGINS_DIR")}) {
		if p.Name() == avPluginName {
			return filepath.Dir(p.BinaryPath)
		}
	}
	return ""
}
