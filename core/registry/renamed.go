package registry

import "fmt"

// renamedTools maps a tool id that flows may still name to the id that replaced
// it. A step naming a renamed id fails with the new name rather than resolving
// to nothing.
var renamedTools = map[ToolID]ToolID{
	"source-gate": "translate-after",
}

// RenamedToolError is the error for a flow step that names a renamed tool id,
// or nil when name is current.
func RenamedToolError(name ToolID) error {
	if to, ok := renamedTools[name]; ok {
		return fmt.Errorf("tool %q is now %q. Rename the step's tool", name, to)
	}
	return nil
}

// UnknownToolError is the error for a tool id the registry does not hold. A
// renamed id says what replaced it.
func UnknownToolError(name ToolID) error {
	if err := RenamedToolError(name); err != nil {
		return fmt.Errorf("unknown tool: %s: %w", name, err)
	}
	return fmt.Errorf("unknown tool: %s", name)
}
