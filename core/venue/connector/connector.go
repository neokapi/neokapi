// Package connector defines the contract a content source implements to
// exchange content with a venue: identity and lifecycle, and the Push/Pull
// operations named from the source system's perspective.
//
// A connector is implemented by whatever holds the content — a checkout, a CI
// job, a directory — and is driven by the project side of the exchange, so the
// contract belongs beside the wire it speaks rather than beside any one venue.
// The dispatch that runs a connector out of process claims the same four op
// names (host/pluginhost.SourceConnectorOpsClaimed).
package connector

import (
	"context"
	"time"
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

// SyncStatus reports the synchronization state between the store and a connector.
//
// The json tags are load-bearing: this struct IS the wire shape of
// GET /:ws/connectors/:id/status and of every element of the batch route's
// `statuses` map. Untagged it serialized PascalCase — the only bowrain response
// that did — so the adapter carried a hand-written PascalCase DTO beside the
// snake_case one the desktop binding already returned for the same reading.
type SyncStatus struct {
	ConnectorID string    `json:"connector_id"`
	LastSync    time.Time `json:"last_sync"`
	ItemCount   int       `json:"item_count"`
	FileCount   int       `json:"file_count"`   // total local files scanned
	WordCount   int       `json:"word_count"`   // total source words across all local blocks
	PendingPull int       `json:"pending_pull"` // Items changed externally since last pull
	PendingPush int       `json:"pending_push"` // Items changed locally since last push
	Errors      []string  `json:"errors"`
}

// ConnectorBase contains shared connector identity and lifecycle methods.
type ConnectorBase interface {
	ID() string
	Name() string
	Category() Category
	Status(ctx context.Context) (*SyncStatus, error)
	Configure(config map[string]string) error
	Close() error
}
