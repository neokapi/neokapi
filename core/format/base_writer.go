package format

import (
	"io"
	"os"

	"github.com/neokapi/neokapi/core/model"
)

// BaseFormatWriter provides shared behavior for format writer implementations.
// Embed this in concrete writers.
type BaseFormatWriter struct {
	FormatName string
	Output     io.Writer
	OutputFile *os.File
	Locale     model.LocaleID
	Encoding   string
	// RequiresSkeleton declares that this writer can only serialize by injecting
	// translated text back into the *original* file's skeleton — it cannot
	// reconstruct a whole document from the content model alone. Packaged /
	// binary formats (OpenXML, ODF, IDML, ICML, MIF, EPUB, image) set this true;
	// they are same-format / merge writers and never a cross-format conversion
	// target. Default false: a writer is generative (writes standalone) unless it
	// declares the need. See AD-005 "Writer output modes".
	RequiresSkeleton bool
}

// Name returns the format identifier.
func (b *BaseFormatWriter) Name() string { return b.FormatName }

// Generative reports whether the writer can serialize a complete document from
// the content model alone (no source skeleton). It is the inverse of the
// declared RequiresSkeleton need. A generative writer is a valid cross-format
// conversion target; a skeleton-bound one (RequiresSkeleton) is not.
func (b *BaseFormatWriter) Generative() bool { return !b.RequiresSkeleton }

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
