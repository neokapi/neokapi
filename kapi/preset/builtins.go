package preset

import (
	"github.com/neokapi/neokapi/core/preset"
)

// RegisterBuiltins registers built-in framework presets into the given registry.
//
// Deprecated: Use preset.RegisterBuiltins from core/preset directly.
func RegisterBuiltins(reg *preset.PresetRegistry) {
	preset.RegisterBuiltins(reg)
}
