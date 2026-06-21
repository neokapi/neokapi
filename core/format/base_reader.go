package format

import (
	"fmt"

	"github.com/neokapi/neokapi/core/model"
)

// BaseFormatReader provides shared behavior for format reader implementations.
// Embed this in concrete readers.
type BaseFormatReader struct {
	FormatName        string
	FormatDisplayName string
	FormatMimeType    string
	FormatExtensions  []string
	Cfg               DataFormatConfig
	Doc               *model.RawDocument

	// diags accumulates Reader Validation-Mode diagnostics. State lives on the
	// reader struct (not the channel) so it survives channel-close: Diagnostics()
	// is queried after the Read range loop, before Close. Only populated when
	// ValidationMode() != ValidationOff, so the default path stays untouched.
	diags []Diagnostic
}

// Name returns the format identifier.
func (b *BaseFormatReader) Name() string { return b.FormatName }

// DisplayName returns the human-readable format name.
func (b *BaseFormatReader) DisplayName() string { return b.FormatDisplayName }

// Config returns the current configuration.
func (b *BaseFormatReader) Config() DataFormatConfig { return b.Cfg }

// SetConfig applies a new configuration after validation.
func (b *BaseFormatReader) SetConfig(cfg DataFormatConfig) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	b.Cfg = cfg
	return nil
}

// ValidationMode reports the reader validation mode, read from the config when
// it carries one (ValidationConfig). Configs that don't opt into RVM report
// ValidationOff, so the reader's lenient path is byte-identical to before.
func (b *BaseFormatReader) ValidationMode() ValidationMode {
	if vc, ok := b.Cfg.(ValidationConfig); ok {
		return vc.ValidationMode()
	}
	return ValidationOff
}

// AddDiagnostic records a Reader Validation-Mode diagnostic. Readers call it
// only when ValidationMode() != ValidationOff, so nothing accumulates on the
// default path.
func (b *BaseFormatReader) AddDiagnostic(d Diagnostic) {
	b.diags = append(b.diags, d)
}

// Diagnostics returns the diagnostics recorded during Read, satisfying
// DiagnosticReader for every reader that embeds BaseFormatReader.
func (b *BaseFormatReader) Diagnostics() []Diagnostic { return b.diags }
