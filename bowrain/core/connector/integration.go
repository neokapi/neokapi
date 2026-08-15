package connector

import (
	"context"
	"time"

	"github.com/neokapi/neokapi/core/model"
)

// ContentItem represents a piece of content discovered by a connector.
type ContentItem struct {
	ID          string
	Name        string
	Path        string
	Format      string
	Locale      model.LocaleID
	Blocks      []*model.Block
	Metadata    map[string]string
	LastChanged time.Time
}

// FetchOptions configures a fetch operation.
type FetchOptions struct {
	Paths   []string // Specific paths/IDs to fetch (empty = all)
	Locales []model.LocaleID
	Force   bool // Re-fetch even if unchanged
	DryRun  bool // Report what would change without modifying
}

// PublishOptions configures a publish operation.
type PublishOptions struct {
	Paths   []string // Specific paths/IDs to publish (empty = all)
	Locales []model.LocaleID
	Force   bool   // Publish even if remote hasn't changed
	DryRun  bool   // Report what would change without modifying
	Message string // Commit/change message for systems that support it
	// Metadata carries connector-specific publish parameters that don't
	// generalize into first-class fields — e.g. the forge connector reads
	// pr_title and pr_body for the pull request it maintains. Connectors
	// ignore keys they don't understand.
	Metadata map[string]string
}

// IntegrationConnector represents a system that Bowrain reaches into.
// Used by server-side integrations (WordPress, Figma, HubSpot, filesystem, Git).
// Terminology: from Bowrain's perspective.
type IntegrationConnector interface {
	ConnectorBase

	// Fetch retrieves source content FROM the external system INTO Bowrain.
	Fetch(ctx context.Context, opts FetchOptions) ([]*ContentItem, error)

	// Publish sends translated content FROM Bowrain TO the external system.
	Publish(ctx context.Context, items []*ContentItem, opts PublishOptions) error

	// List returns available content items without fetching full content.
	List(ctx context.Context) ([]*ContentItem, error)
}
