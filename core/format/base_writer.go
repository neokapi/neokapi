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
	// Binary declares that the writer emits bytes that are not text in a
	// charset: a ZIP container, an image, a compiled catalog, or bytes another
	// process already encoded. The run's encoding is recorded on such a writer
	// and never applied to its output, and neither are the shared output
	// options. A reader declares the same on its FormatSignature.
	Binary bool

	// OutputOpts are the shared byte-level output options (BOM policy,
	// newline style, output charset). When non-passthrough, Output is
	// wrapped with the post-encode chain so every writer that embeds the
	// base inherits the behavior. Set via SetOutputOptions. An output.encoding
	// here wins over the run's Encoding; with none, the writer encodes its
	// text in Encoding.
	OutputOpts OutputOptions

	// rawOutput is the unwrapped destination (file or caller writer);
	// outputWrap is the post-encode chain pending a flush on Close.
	rawOutput  io.Writer
	outputWrap io.Closer
	// encodingErr holds a failure to build the output chain after SetEncoding,
	// which cannot report one itself; the next SetOutput, SetOutputWriter or
	// Close returns it.
	encodingErr error
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
// effective output options. Passthrough options leave Output as the raw
// destination.
func (b *BaseFormatWriter) applyOutputWrap() error {
	b.outputWrap = nil
	b.encodingErr = nil
	b.Output = b.rawOutput
	opts := b.EffectiveOutputOptions()
	if b.rawOutput == nil || opts.IsZero() {
		return nil
	}
	wc, err := opts.Wrap(b.rawOutput)
	if err != nil {
		return err
	}
	b.Output = wc
	b.outputWrap = wc
	return nil
}

// EffectiveOutputOptions are the output options the writer's bytes pass
// through: the configured OutputOpts, with the run's Encoding filling in an
// unset output.encoding. A Binary writer has none.
func (b *BaseFormatWriter) EffectiveOutputOptions() OutputOptions {
	if b.Binary {
		return OutputOptions{}
	}
	opts := b.OutputOpts
	if opts.Encoding == "" {
		opts.Encoding = b.Encoding
	}
	return opts
}

// SetLocale sets the target locale for writing.
func (b *BaseFormatWriter) SetLocale(locale model.LocaleID) {
	b.Locale = locale
}

// SetEncoding sets the charset the writer encodes its text in. A failure to
// build the output chain (an unknown charset) is reported by the next
// SetOutput, SetOutputWriter or Close.
func (b *BaseFormatWriter) SetEncoding(encoding string) {
	b.Encoding = encoding
	if err := b.applyOutputWrap(); err != nil {
		b.encodingErr = err
	}
}

// Close flushes the output-option chain (if any) and closes the output file
// if one was opened.
func (b *BaseFormatWriter) Close() error {
	firstErr := b.encodingErr
	b.encodingErr = nil
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
