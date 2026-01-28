package tool

// ToolConfig holds configuration for a Tool.
type ToolConfig interface {
	// ToolName returns the tool this config applies to.
	ToolName() string

	// Reset restores default values.
	Reset()

	// Validate checks configuration validity.
	Validate() error
}
