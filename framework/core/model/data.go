package model

// Data holds non-translatable document structure.
type Data struct {
	ID         string
	Name       string
	Skeleton   *Skeleton
	Properties map[string]string
	IsReferent bool // Whether this data is referenced by a skeleton
}

// ResourceID returns the Data's unique identifier.
func (d *Data) ResourceID() string { return d.ID }
