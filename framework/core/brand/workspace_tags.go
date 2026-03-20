package brand

// TagDimension defines a workspace-configurable tag dimension.
// Used with graph.Validity to scope edges by business-defined criteria.
type TagDimension struct {
	Name        string   `json:"name"` // e.g., "market", "channel", "product"
	Description string   `json:"description,omitempty"`
	Values      []string `json:"values"` // allowed values
	Required    bool     `json:"required"`
}
