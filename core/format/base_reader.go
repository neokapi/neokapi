package format

import (
	"fmt"

	"github.com/asgeirf/gokapi/core/model"
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
