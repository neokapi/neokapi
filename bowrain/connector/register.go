package connector

import (
	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/core/registry"
)

// RegisterAll registers all built-in connectors with the given registry.
// The FileConnector and GitConnector require a FormatRegistry for format
// detection and parsing.
func RegisterAll(r *platconn.Registry, formatReg *registry.FormatRegistry) {
	r.Register("file", platconn.CategoryFile, func(config map[string]string) (platconn.IntegrationConnector, error) {
		return NewFileConnector(formatReg, config)
	})

	r.Register("git", platconn.CategoryCode, func(config map[string]string) (platconn.IntegrationConnector, error) {
		return NewGitConnector(formatReg, config)
	})

	r.Register("wordpress", platconn.CategoryCMS, func(config map[string]string) (platconn.IntegrationConnector, error) {
		return NewWordPressConnector(config)
	})

	r.Register("figma", platconn.CategoryDesign, func(config map[string]string) (platconn.IntegrationConnector, error) {
		return NewFigmaConnector(config)
	})

	r.Register("hubspot", platconn.CategoryMarketing, func(config map[string]string) (platconn.IntegrationConnector, error) {
		return NewHubSpotConnector(config)
	})
}
