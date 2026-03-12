package model

import "time"

// BlockNote represents a user note or comment attached to a block.
type BlockNote struct {
	ID        string    `json:"id"`
	BlockID   string    `json:"blockId"`
	Author    string    `json:"author"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
}
