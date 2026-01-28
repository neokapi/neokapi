package model

// GroupStart signals the beginning of a structural group within a Layer.
type GroupStart struct {
	ID   string
	Name string
	Type string
}

// ResourceID returns the GroupStart's unique identifier.
func (gs *GroupStart) ResourceID() string { return gs.ID }

// GroupEnd signals the end of a structural group.
type GroupEnd struct {
	ID string
}

// ResourceID returns the GroupEnd's unique identifier.
func (ge *GroupEnd) ResourceID() string { return ge.ID }
