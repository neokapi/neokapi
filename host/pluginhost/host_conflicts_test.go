package pluginhost

import (
	"fmt"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostCapabilityConflictsStayExcluded(t *testing.T) {
	for _, count := range []int{2, 3, 4, 5} {
		t.Run(fmt.Sprintf("%d providers", count), func(t *testing.T) {
			plugins := make([]*Plugin, 0, count)
			for n := range count {
				plugins = append(plugins, &Plugin{Manifest: &manifest.Manifest{
					Plugin: fmt.Sprintf("provider-%d", n),
					Capabilities: manifest.Capabilities{
						Commands:         []manifest.Command{{Name: "shared"}},
						MCPTools:         []manifest.MCPTool{{Name: "shared"}},
						Formats:          []manifest.Format{{Name: "shared"}},
						Segmenters:       []manifest.Segmenter{{Name: "shared"}},
						Comments:         []manifest.CommentLanguage{{Language: "shared"}},
						ConfigNamespaces: []manifest.ConfigNamespace{{Prefix: "shared"}},
					},
				}})
			}
			warnings := []string{}
			host := NewHost(plugins, func(msg string) { warnings = append(warnings, msg) })
			assert.Nil(t, host.CommandRoute("shared"))
			assert.Nil(t, host.MCPRoute("shared"))
			assert.Nil(t, host.FormatRoute("shared"))
			assert.Nil(t, host.SegmenterRoute("shared"))
			assert.Empty(t, host.CommentRoutes())
			assert.Nil(t, host.ConfigNamespaceRoute("shared"))
			assert.Len(t, warnings, 6*(count-1), "every additional provider is reported")
		})
	}
}

func TestHostCapabilityNamesAreScopedByKind(t *testing.T) {
	commandPlugin := &Plugin{Manifest: &manifest.Manifest{
		Plugin: "commands",
		Capabilities: manifest.Capabilities{
			Commands: []manifest.Command{{Name: "shared"}},
		},
	}}
	formatPlugin := &Plugin{Manifest: &manifest.Manifest{
		Plugin: "formats",
		Capabilities: manifest.Capabilities{
			Formats: []manifest.Format{{Name: "shared"}},
		},
	}}
	warnings := []string{}
	host := NewHost([]*Plugin{commandPlugin, formatPlugin}, func(msg string) { warnings = append(warnings, msg) })
	require.NotNil(t, host.CommandRoute("shared"))
	require.NotNil(t, host.FormatRoute("shared"))
	assert.Same(t, commandPlugin, host.CommandRoute("shared").Plugin)
	assert.Same(t, formatPlugin, host.FormatRoute("shared").Plugin)
	assert.Empty(t, warnings)
}
