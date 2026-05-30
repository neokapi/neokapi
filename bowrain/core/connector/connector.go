// Package connector defines the interfaces for external content system
// integration. Two distinct connector perspectives require two interfaces:
//
// IntegrationConnector: Server-side connectors that Bowrain reaches into
// (WordPress, Figma, HubSpot, filesystem, Git). Uses Fetch/Publish terminology
// from Bowrain's perspective.
//
// SourceConnector: Client-side connectors that push content TO Bowrain
// (kapi CLI). Uses Push/Pull terminology from the source system's perspective.
package connector

import (
	"context"
	"time"

	"github.com/neokapi/neokapi/core/model"
)

// Category classifies the type of external system a connector integrates with.
type Category string

const (
	CategoryFile         Category = "file"
	CategoryCode         Category = "code"
	CategoryCMS          Category = "cms"
	CategoryDesign       Category = "design"
	CategoryMarketing    Category = "marketing"
	CategoryTMS          Category = "tms"
	CategoryProductivity Category = "productivity" // office/productivity suites (Google Workspace, Microsoft 365)
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

// SyncStatus reports the synchronization state between the store and a connector.
type SyncStatus struct {
	ConnectorID string
	LastSync    time.Time
	ItemCount   int
	FileCount   int // total local files scanned
	WordCount   int // total source words across all local blocks
	PendingPull int // Items changed externally since last pull
	PendingPush int // Items changed locally since last push
	Errors      []string
}

// ---------------------------------------------------------------------------
// ConnectorBase: shared identity and lifecycle
// ---------------------------------------------------------------------------

// ConnectorBase contains shared connector identity and lifecycle methods.
type ConnectorBase interface {
	ID() string
	Name() string
	Category() Category
	Status(ctx context.Context) (*SyncStatus, error)
	Configure(config map[string]string) error
	Close() error
}

// ---------------------------------------------------------------------------
// IntegrationConnector: server-side connectors (Bowrain reaches out)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// SourceConnector: client-side connectors (kapi pushes to Bowrain)
// ---------------------------------------------------------------------------

// PushOptions configures a push operation from a source system to Bowrain.
type PushOptions struct {
	Paths  []string // Specific file paths to push (empty = all)
	Force  bool     // Push all blocks, ignoring sync cache
	DryRun bool     // Report what would be pushed without sending
}

// PullOptions configures a pull operation from Bowrain to a source system.
type PullOptions struct {
	Locales []model.LocaleID // Target locales to pull (empty = all)
	Force   bool             // Overwrite local changes
	DryRun  bool             // Report what would be pulled without writing
}

// PushResult summarizes the result of a push operation.
type PushResult struct {
	BlocksPushed int
	AssetsPushed int // number of media assets pushed (Bowrain AD-007)
	FilesScanned int
	ChunkCount   int
	WordCount    int    // total source words across pushed blocks
	PushID       string // server-assigned push correlation ID (empty if nothing stored)
}

// PullResult summarizes the result of a pull operation.
type PullResult struct {
	BlocksPulled int
	FilesWritten int
	LocalesCount int
}

// SourceConnector represents a content source that pushes to and pulls from Bowrain.
// Used by systems outside Bowrain (kapi CLI, Git hooks, CI/CD).
// Terminology: from the source system's perspective.
type SourceConnector interface {
	ConnectorBase

	// Push sends source content FROM the source system TO Bowrain.
	Push(ctx context.Context, opts PushOptions) (*PushResult, error)

	// Pull retrieves translated content FROM Bowrain TO the source system.
	Pull(ctx context.Context, opts PullOptions) (*PullResult, error)
}
