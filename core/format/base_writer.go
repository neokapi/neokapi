package format

import (
	"io"
	"os"

	"github.com/asgeirf/gokapi/core/model"
)

// BaseFormatWriter provides shared behavior for format writer implementations.
// Embed this in concrete writers.
type BaseFormatWriter struct {
	FormatName string
	Output     io.Writer
	OutputFile *os.File
	Locale     model.LocaleID
	Encoding   string
}

// Name returns the format identifier.
func (b *BaseFormatWriter) Name() string { return b.FormatName }

// SetOutput configures the output destination by file path.
func (b *BaseFormatWriter) SetOutput(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	b.OutputFile = f
	b.Output = f
	return nil
}

// SetOutputWriter configures an io.Writer as output.
func (b *BaseFormatWriter) SetOutputWriter(w io.Writer) error {
	b.Output = w
	return nil
}

// SetLocale sets the target locale for writing.
func (b *BaseFormatWriter) SetLocale(locale model.LocaleID) {
	b.Locale = locale
}

// SetEncoding sets the output encoding.
func (b *BaseFormatWriter) SetEncoding(encoding string) {
	b.Encoding = encoding
}

// Close flushes and closes the output file if one was opened.
func (b *BaseFormatWriter) Close() error {
	if b.OutputFile != nil {
		return b.OutputFile.Close()
	}
	return nil
}
