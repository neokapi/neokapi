package host

import (
	"fmt"
	"sync"

	"github.com/neokapi/neokapi/host/pluginhost"
)

// The media engines (asr, vision) are found by one discovery per run, not one
// per file: a sync asks whether an engine is available once for every media
// file it reads. RefreshMediaEngines forgets the result after a plugin is
// installed, updated or removed in a long-lived process (Kapi Desktop).

// mediaPluginSet is one discovery's plugins and refusals, by name.
type mediaPluginSet struct {
	plugins map[string]*pluginhost.Plugin
	refused map[string]pluginhost.Refusal
}

var mediaPlugins struct {
	sync.Mutex
	set *mediaPluginSet
}

// discoverMediaPlugins is the discovery behind the set, replaceable in a test
// that counts it.
var discoverMediaPlugins = func() ([]*pluginhost.Plugin, []pluginhost.Refusal) {
	return pluginhost.DiscoverAll(pluginhost.DefaultDiscoverOptions())
}

// currentMediaPlugins returns the run's discovery, made on first use.
func currentMediaPlugins() *mediaPluginSet {
	mediaPlugins.Lock()
	defer mediaPlugins.Unlock()
	if mediaPlugins.set == nil {
		plugins, refused := discoverMediaPlugins()
		set := &mediaPluginSet{plugins: map[string]*pluginhost.Plugin{}, refused: map[string]pluginhost.Refusal{}}
		for _, p := range plugins {
			set.plugins[p.Name()] = p
		}
		for _, r := range refused {
			set.refused[r.Name] = r
		}
		mediaPlugins.set = set
	}
	return mediaPlugins.set
}

// forgetMediaPlugins drops the run's discovery, so the next lookup finds the
// plugins installed now.
func forgetMediaPlugins() {
	mediaPlugins.Lock()
	mediaPlugins.set = nil
	mediaPlugins.Unlock()
}

// findMediaPlugin returns the loaded plugin named name, or an error that says
// whether it is not installed (install) or installed and refused (the reason,
// which names the fix).
func findMediaPlugin(name, what string) (*pluginhost.Plugin, error) {
	set := currentMediaPlugins()
	if p, ok := set.plugins[name]; ok {
		return p, nil
	}
	if r, ok := set.refused[name]; ok {
		return nil, fmt.Errorf("%s: the %q plugin is installed but not loaded: %s", what, name, r.Reason)
	}
	return nil, fmt.Errorf("%s: the %q plugin is not installed (kapi plugins install %s)", what, name, name)
}

// mediaPluginRefusal returns why the installed plugin name is not loaded.
func mediaPluginRefusal(name string) (string, bool) {
	r, ok := currentMediaPlugins().refused[name]
	return r.Reason, ok
}
