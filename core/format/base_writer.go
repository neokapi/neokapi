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
	// Interchange declares that this is a bilingual translation-interchange
	// format (XLIFF, PO, TMX, …). These belong to the extract→translate→merge
	// loop — `kapi extract` captures the source skeleton so `kapi merge` can
	// round-trip translations back into the original format — so they are NOT
	// offered as `convert` targets (a converted interchange file carries no
	// skeleton and cannot be merged back). See AD-005 "Writer output modes".
	Interchange bool

	// OutputOpts are the shared byte-level output options (BOM policy,
	// newline style, output charset). When non-passthrough, Output is
	// wrapped with the post-encode chain so every writer that embeds the
	// base inherits the behavior. Set via SetOutputOptions.
	OutputOpts OutputOptions

	// rawOutput is the unwrapped destination (file or caller writer);
	// outputWrap is the post-encode chain pending a flush on Close.
	rawOutput  io.Writer
	outputWrap io.Closer
}

// Name returns the format identifier.
func (b *BaseFormatWriter) Name() string { return b.FormatName }

// Generative reports whether the writer can serialize a complete document from
// the content model alone (no source skeleton). It is the inverse of the
// declared RequiresSkeleton need. A generative writer is a valid cross-format
// conversion target; a skeleton-bound one (RequiresSkeleton) is not.
func (b *BaseFormatWriter) Generative() bool { return !b.RequiresSkeleton }

// IsInterchange reports whether this is a bilingual translation-interchange
// format (the extract/merge workflow), which is excluded from `convert` targets.
func (b *BaseFormatWriter) IsInterchange() bool { return b.Interchange }

// SetOutput configures the output destination by file path.
func (b *BaseFormatWriter) SetOutput(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	b.OutputFile = f
	b.rawOutput = f
	return b.applyOutputWrap()
}

// SetOutputWriter configures an io.Writer as output.
func (b *BaseFormatWriter) SetOutputWriter(w io.Writer) error {
	b.rawOutput = w
	return b.applyOutputWrap()
}

// SetOutputOptions configures the shared byte-level output options (BOM,
// newline style, charset). Implements OutputConfigurable. May be called
// before or after SetOutput/SetOutputWriter, as long as it happens before
// the first write.
func (b *BaseFormatWriter) SetOutputOptions(opts OutputOptions) error {
	b.OutputOpts = opts
	return b.applyOutputWrap()
}

// applyOutputWrap (re)derives Output from the raw destination and the
// configured output options. Passthrough options leave Output as the raw
// destination, preserving historical behavior exactly.
func (b *BaseFormatWriter) applyOutputWrap() error {
	b.outputWrap = nil
	b.Output = b.rawOutput
	if b.rawOutput == nil || b.OutputOpts.IsZero() {
		return nil
	}
	wc, err := b.OutputOpts.Wrap(b.rawOutput)
	if err != nil {
		return err
	}
	b.Output = wc
	b.outputWrap = wc
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

// Close flushes the output-option chain (if any) and closes the output file
// if one was opened.
func (b *BaseFormatWriter) Close() error {
	var firstErr error
	if b.outputWrap != nil {
		firstErr = b.outputWrap.Close()
		b.outputWrap = nil
	}
	if b.OutputFile != nil {
		if err := b.OutputFile.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
