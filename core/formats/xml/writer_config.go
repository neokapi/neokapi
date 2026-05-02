package xml

import (
	"fmt"
)

// WriterCfg holds writer-side serialization knobs for the XML format.
// These are independent of the reader Config — readers parse documents,
// writers reconstruct them, and each side has its own concerns.
//
// Defaults preserve the source document verbatim (no extra prologue,
// no transformation), which is the right behavior for round-trip
// translation workflows. Set fields explicitly to deviate.
type WriterCfg struct {
	// EmitDeclaration writes a `<?xml version="..." encoding="..."?>`
	// prologue even when the source had none. Default false: the
	// writer preserves the source's prologue (which may be no
	// declaration at all). Set to true when interoperating with tools
	// that require a declaration (e.g. matching upstream Okapi's
	// default behavior in parity testing).
	EmitDeclaration bool

	// DeclarationVersion is the version attribute when emitting a
	// declaration. Default "1.0". Ignored when EmitDeclaration is
	// false.
	DeclarationVersion string

	// DeclarationEncoding is the encoding attribute when emitting a
	// declaration. Default "UTF-8". Ignored when EmitDeclaration is
	// false.
	DeclarationEncoding string
}

// NewWriterCfg returns a WriterCfg with documented defaults.
func NewWriterCfg() *WriterCfg {
	return &WriterCfg{
		DeclarationVersion:  "1.0",
		DeclarationEncoding: "UTF-8",
	}
}

// FormatName implements DataFormatConfig.
func (c *WriterCfg) FormatName() string { return "xml" }

// Reset implements DataFormatConfig — restores documented defaults.
func (c *WriterCfg) Reset() {
	c.EmitDeclaration = false
	c.DeclarationVersion = "1.0"
	c.DeclarationEncoding = "UTF-8"
}

// Validate implements DataFormatConfig.
func (c *WriterCfg) Validate() error {
	if c.EmitDeclaration {
		if c.DeclarationVersion == "" {
			return fmt.Errorf("xml writer: DeclarationVersion required when EmitDeclaration=true")
		}
		if c.DeclarationEncoding == "" {
			return fmt.Errorf("xml writer: DeclarationEncoding required when EmitDeclaration=true")
		}
	}
	return nil
}

// ApplyMap implements DataFormatConfig.
func (c *WriterCfg) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "emitDeclaration":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("emitDeclaration: expected bool, got %T", val)
			}
			c.EmitDeclaration = b
		case "declarationVersion":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("declarationVersion: expected string, got %T", val)
			}
			c.DeclarationVersion = s
		case "declarationEncoding":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("declarationEncoding: expected string, got %T", val)
			}
			c.DeclarationEncoding = s
		default:
			// Silently ignore unknown keys for forward compatibility,
			// matching the reader Config convention.
		}
	}
	return nil
}
