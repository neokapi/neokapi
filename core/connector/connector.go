// Package connector defines the bidirectional connector interface for
// pulling content from and pushing content to external systems.
package connector

import (
	"context"
	"time"

	"github.com/gokapi/gokapi/core/model"
)

// Category classifies the type of external system a connector integrates with.
type Category string

const (
	CategoryFile      Category = "file"
	CategoryCode      Category = "code"
	CategoryCMS       Category = "cms"
	CategoryDesign    Category = "design"
	CategoryMarketing Category = "marketing"
	CategoryTMS       Category = "tms"
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
	PendingPull int // Items changed externally since last pull
	PendingPush int // Items changed locally since last push
	Errors      []string
}

// PullOptions configures a pull operation.
type PullOptions struct {
	Paths   []string // Specific paths/IDs to pull (empty = all)
	Locales []model.LocaleID
	Force   bool // Re-pull even if unchanged
	DryRun  bool // Report what would change without modifying
}

// PushOptions configures a push operation.
type PushOptions struct {
	Paths   []string // Specific paths/IDs to push (empty = all)
	Locales []model.LocaleID
	Force   bool   // Push even if remote hasn't changed
	DryRun  bool   // Report what would change without modifying
	Message string // Commit/change message for systems that support it
}

// Connector is the bidirectional interface for external content systems.
type Connector interface {
	// Identity
	ID() string
	Name() string
	Category() Category

	// Content operations
	Pull(ctx context.Context, opts PullOptions) ([]*ContentItem, error)
	Push(ctx context.Context, items []*ContentItem, opts PushOptions) error
	List(ctx context.Context) ([]*ContentItem, error)
	Sync(ctx context.Context) (*SyncStatus, error)

	// Configuration
	Configure(config map[string]string) error

	// Lifecycle
	Close() error
}
