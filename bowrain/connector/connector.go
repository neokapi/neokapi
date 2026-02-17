// Package connector provides concrete integration connectors (file, git,
// WordPress, Figma, HubSpot). Interface types and the connector registry
// are defined in platform/connector and re-exported here via type aliases.
package connector

import platconn "github.com/gokapi/gokapi/platform/connector"

// Type aliases — canonical definitions live in platform/connector.
type (
	Category             = platconn.Category
	ContentItem          = platconn.ContentItem
	SyncStatus           = platconn.SyncStatus
	ConnectorBase        = platconn.ConnectorBase
	IntegrationConnector = platconn.IntegrationConnector
	SourceConnector      = platconn.SourceConnector
	FetchOptions         = platconn.FetchOptions
	PublishOptions       = platconn.PublishOptions
	Factory              = platconn.Factory
	ConnectorInfo        = platconn.ConnectorInfo
	Registry             = platconn.Registry
)

// Re-export constants.
const (
	CategoryFile      = platconn.CategoryFile
	CategoryCode      = platconn.CategoryCode
	CategoryCMS       = platconn.CategoryCMS
	CategoryDesign    = platconn.CategoryDesign
	CategoryMarketing = platconn.CategoryMarketing
	CategoryTMS       = platconn.CategoryTMS
)

// Re-export constructor.
var NewRegistry = platconn.NewRegistry
