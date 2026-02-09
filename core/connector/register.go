package connector

import "github.com/gokapi/gokapi/core/registry"

// RegisterAll registers all built-in connectors with the given registry.
// The FileConnector and GitConnector require a FormatRegistry for format
// detection and parsing.
func RegisterAll(r *Registry, formatReg *registry.FormatRegistry) {
	r.Register("file", CategoryFile, func(config map[string]string) (Connector, error) {
		return NewFileConnector(formatReg, config)
	})

	r.Register("git", CategoryCode, func(config map[string]string) (Connector, error) {
		return NewGitConnector(formatReg, config)
	})

	r.Register("wordpress", CategoryCMS, func(config map[string]string) (Connector, error) {
		return NewWordPressConnector(config)
	})

	r.Register("figma", CategoryDesign, func(config map[string]string) (Connector, error) {
		return NewFigmaConnector(config)
	})

	r.Register("hubspot", CategoryMarketing, func(config map[string]string) (Connector, error) {
		return NewHubSpotConnector(config)
	})
}
