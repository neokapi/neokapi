package model

// Segment is a single segment within a Block's source or target content.
type Segment struct {
	ID      string
	Content *Fragment
}
