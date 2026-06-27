package i18next

import (
	"context"
	"io"

	"github.com/neokapi/neokapi/core/format"
	jsonfmt "github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for i18next JSON resource bundles. Writing
// is delegated to the generic JSON writer: i18next blocks carry the same
// json.keypath / json.original bookkeeping as plain JSON blocks, so the JSON
// writer's token-level value replacement reproduces the document
// byte-faithfully.
//
// The one transform this wrapper performs is flattening each top-level block's
// inline-coded runs back into a flat value before delegating. The reader (via
// the JSON code-finder) splits {{interpolation}} and $t() nesting into opaque
// placeholder runs; the JSON writer renders a value from a block's plain text,
// which would drop those codes. Re-rendering the runs with their captured data
// (model.RenderRunsWithData) splices the protected codes back in, so the
// untranslated source — and any inline-coded translation — round-trips exactly.
// Blocks inside child (HTML subfilter) layers are left untouched; the HTML
// sub-writer renders its own runs.
type Writer struct {
	format.BaseFormatWriter
	cfg      *Config
	inner    *jsonfmt.Writer
	resolver format.SubfilterResolver
}

// Ensure Writer forwards subfiltering to the inner JSON writer so the `_html`
// HTML subfilter child layers serialize correctly.
var _ format.SubfilterAware = (*Writer)(nil)

// Ensure Writer consumes a byte-exact skeleton by forwarding the store to the
// inner JSON writer, whose writeFromSkeleton path reproduces the document.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// StreamingWriter marks this writer streaming (it forwards to the streaming JSON
// writer), so the file runner pairs the streaming i18next reader with a
// synchronized streaming skeleton store rather than a buffered one they race on.
var _ format.StreamingWriter = (*Writer)(nil)

func (w *Writer) StreamingWriter() bool { return true }

// NewWriter creates a new i18next writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	w := &Writer{
		BaseFormatWriter: format.BaseFormatWriter{FormatName: formatID},
		cfg:              cfg,
	}
	w.inner = jsonfmt.NewWriter()
	// Apply the i18next-derived settings (notably escapeForwardSlashes) to the
	// inner writer's live config. Config() returns the writer's own pointer, so
	// mutating it in place is what actually takes effect.
	cfg.applyToJSON(w.inner.Config())
	return w
}

// SetOutput configures the output destination by path on the inner writer.
func (w *Writer) SetOutput(path string) error { return w.inner.SetOutput(path) }

// SetOutputWriter configures an io.Writer as output on the inner writer.
func (w *Writer) SetOutputWriter(out io.Writer) error { return w.inner.SetOutputWriter(out) }

// SetLocale sets the target locale on the inner writer.
func (w *Writer) SetLocale(locale model.LocaleID) {
	w.Locale = locale
	w.inner.SetLocale(locale)
}

// SetEncoding sets the output encoding on the inner writer.
func (w *Writer) SetEncoding(encoding string) { w.inner.SetEncoding(encoding) }

// SetSubfilterResolver records the resolver and forwards it to the inner JSON
// writer so embedded HTML child layers can be re-serialized.
func (w *Writer) SetSubfilterResolver(resolver format.SubfilterResolver) {
	w.resolver = resolver
	w.inner.SetSubfilterResolver(resolver)
}

// SetSkeletonStore forwards the skeleton store to the inner JSON writer, whose
// writeFromSkeleton path reproduces the document byte-for-byte, splicing only
// translated values at each block reference.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.inner.SetSkeletonStore(store)
}

// Write transforms the incoming parts (flattening inline-coded runs on
// top-level blocks) and delegates serialization to the inner JSON writer.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	transformed := make(chan *model.Part, 64)

	go func() {
		defer close(transformed)
		depth := 0 // nesting depth of child (subfilter) layers
		for {
			select {
			case <-ctx.Done():
				return
			case p, ok := <-parts:
				if !ok {
					return
				}
				switch p.Type {
				case model.PartLayerStart:
					if l, ok := p.Resource.(*model.Layer); ok && !l.IsRoot() {
						depth++
					}
				case model.PartLayerEnd:
					if l, ok := p.Resource.(*model.Layer); ok && !l.IsRoot() {
						depth--
					}
				case model.PartBlock:
					if depth == 0 {
						if b, ok := p.Resource.(*model.Block); ok {
							flattenInlineCodes(b, w.Locale)
						}
					}
				}
				select {
				case transformed <- p:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return w.inner.Write(ctx, transformed)
}

// Close flushes and closes the inner writer.
func (w *Writer) Close() error { return w.inner.Close() }

// flattenInlineCodes collapses a block's inline-coded source (and, when present,
// the target for locale) into a single flat TextRun so the JSON writer's
// text-based value rendering reproduces the protected {{interpolation}} / $t()
// codes verbatim. Blocks whose runs are already plain text are left unchanged.
func flattenInlineCodes(b *model.Block, locale model.LocaleID) {
	if runsHaveInlineCodes(b.Source) {
		b.SetSourceText(model.RenderRunsWithData(b.SourceRuns()))
	}
	if locale != "" {
		if target := b.TargetRuns(locale); runsHaveInlineCodes(target) {
			b.SetTargetText(locale, model.RenderRunsWithData(target))
		}
	}
}

// runsHaveInlineCodes reports whether any run in the sequence is a non-text
// run (a protected inline code).
func runsHaveInlineCodes(runs []model.Run) bool {
	for i := range runs {
		if runs[i].Text == nil {
			return true
		}
	}
	return false
}
