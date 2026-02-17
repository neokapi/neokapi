package model

import "time"

// ContentRef links a block to its external source in a connector system.
type ContentRef struct {
	ConnectorID string    // ID of the connector that owns this content
	ExternalID  string    // Unique ID in the external system
	ExternalURL string    // URL to view the content in the external system
	SyncedAt    time.Time // Last sync timestamp
}
