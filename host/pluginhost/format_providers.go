package pluginhost

import "strings"

// knownFormatPlugins names, for each format a plugin published from this
// repository reads, the plugin that supplies it. It lets kapi name the plugin to
// install when no plugin is installed. Nothing is dispatched from it, and
// TestKnownFormatPluginsMatchTheManifests pins it to those plugins' manifests.
var knownFormatPlugins = map[string]string{
	"pdf":        "pdfium",
	"sourcecode": "sourcecode",
}

// bridgeFormatPrefix begins the name of every format the okapi-bridge plugin
// supplies: the bridge names each Okapi filter's format "okf_" followed by the
// filter's format ID.
const bridgeFormatPrefix = "okf_"

// bridgePlugin is the registry name of the okapi-bridge plugin.
const bridgePlugin = "okapi-bridge"

// FormatProvider returns the name of the plugin that supplies format, which is
// the name `kapi plugins install` resolves in the registry. An installed plugin
// whose manifest declares the format comes first, then the plugin kapi knows to
// publish it. A retired plugin supplies nothing. ok is false when kapi knows no
// plugin that supplies format.
func FormatProvider(installed []*Plugin, format string) (plugin string, ok bool) {
	if format == "" {
		return "", false
	}
	for _, p := range installed {
		if p == nil || p.Manifest == nil || p.Retired != nil {
			continue
		}
		for _, f := range p.Manifest.Capabilities.Formats {
			if f.Name == format {
				return p.Name(), true
			}
		}
	}
	if plugin, ok := knownFormatPlugins[format]; ok {
		return plugin, true
	}
	if strings.HasPrefix(format, bridgeFormatPrefix) && len(format) > len(bridgeFormatPrefix) {
		return bridgePlugin, true
	}
	return "", false
}
