package i18next

import (
	"context"
	"errors"

	"github.com/neokapi/neokapi/core/format"
	jsonfmt "github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/model"
)

const (
	formatID      = "i18next"
	displayName   = "i18next JSON"
	formatMime    = "application/json"
	pluralGroupNS = "i18next"
)

// Reader implements DataFormatReader for i18next / react-i18next JSON resource
// bundles. It is a thin wrapper over the generic JSON reader: the inner reader
// does all tokenizing, naming, subfiltering, inline-code detection, and
// byte-faithful round-trip bookkeeping; this wrapper configures it with the
// i18next preset and post-processes the emitted block stream to attach i18next
// plural/context metadata.
type Reader struct {
	format.BaseFormatReader
	cfg      *Config
	inner    *jsonfmt.Reader
	resolver format.SubfilterResolver
}

// Ensure Reader forwards subfiltering to the inner JSON reader so the `_html`
// HTML subfilter resolves at pipeline time.
var _ format.SubfilterAware = (*Reader)(nil)

// NewReader creates a new i18next reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        formatID,
			FormatDisplayName: displayName,
			FormatMimeType:    formatMime,
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// SetConfig applies a new configuration after validation, keeping the typed
// i18next config in sync so Open builds the right inner JSON config.
func (r *Reader) SetConfig(cfg format.DataFormatConfig) error {
	if err := r.BaseFormatReader.SetConfig(cfg); err != nil {
		return err
	}
	if c, ok := cfg.(*Config); ok {
		r.cfg = c
	}
	return nil
}

// SetSubfilterResolver records the resolver and forwards it to the inner JSON
// reader (created in Open).
func (r *Reader) SetSubfilterResolver(resolver format.SubfilterResolver) {
	r.resolver = resolver
}

// Signature returns detection metadata. i18next files use the generic .json
// extension and the application/json MIME, both of which are owned by the
// generic json format and cannot be reliably auto-distinguished. The format is
// therefore selected explicitly (-f i18next / recipe config); no extension or
// MIME is advertised here so detection never steals .json from the json format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{}
}

// Open builds the inner JSON reader from the i18next config and opens it.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("i18next: nil document or reader")
	}
	r.Doc = doc

	inner := jsonfmt.NewReader()
	// Mutate the inner reader's live config in place. The JSON reader keeps a
	// private *Config pointer that Config() returns; calling SetConfig would
	// only swap the embedded base's pointer, leaving the reader's own pointer
	// stale. Applying the i18next-derived settings to the live config is the
	// reliable way to configure it.
	if jc, ok := inner.Config().(*jsonfmt.Config); ok {
		r.cfg.applyToJSON(jc)
	}
	if r.resolver != nil {
		inner.SetSubfilterResolver(r.resolver)
	}
	if err := inner.Open(ctx, doc); err != nil {
		return err
	}
	r.inner = inner
	return nil
}

// Read delegates to the inner JSON reader and post-processes the stream: the
// root layer's format is relabelled to i18next, and each top-level translatable
// block is annotated with i18next plural/context metadata. All other parts
// (data, child layers from the HTML subfilter, and the blocks' identities)
// pass through untouched so byte-faithful round-trip is preserved.
func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
	out := make(chan model.PartResult, 64)
	in := r.inner.Read(ctx)

	go func() {
		defer close(out)
		depth := 0 // nesting depth of child (subfilter) layers
		for pr := range in {
			if pr.Error != nil {
				select {
				case out <- pr:
				case <-ctx.Done():
					return
				}
				continue
			}

			switch pr.Part.Type {
			case model.PartLayerStart:
				if layer, ok := pr.Part.Resource.(*model.Layer); ok {
					if layer.IsRoot() {
						layer.Format = formatID
					} else {
						depth++
					}
				}
			case model.PartLayerEnd:
				if layer, ok := pr.Part.Resource.(*model.Layer); ok && !layer.IsRoot() {
					depth--
				}
			case model.PartBlock:
				// Only annotate top-level i18next blocks. Blocks emitted inside a
				// child layer belong to a subfiltered value (e.g. HTML) and are
				// not i18next keys.
				if depth == 0 {
					if block, ok := pr.Part.Resource.(*model.Block); ok {
						annotateBlock(block, blockKeyPath(block), r.cfg)
					}
				}
			}

			select {
			case out <- pr:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

// blockKeyPath recovers the raw dotted JSON key path the inner reader recorded
// for a block. The JSON reader stores it on the json.keypath property when the
// block name differs from the path (the UseFullKeyPath case used here, where
// the name is a slash path); when that property is absent it falls back to the
// block name, normalising a slash path back to the dotted form the analysis
// helpers expect.
func blockKeyPath(block *model.Block) string {
	if kp, ok := block.Properties["json.keypath"]; ok && kp != "" {
		return kp
	}
	name := block.Name
	// Normalise a leading-slash full key path (/a/b) to the dotted form (a.b).
	if len(name) > 0 && name[0] == '/' {
		name = name[1:]
	}
	return replaceAll(name, '/', '.')
}

// replaceAll replaces every occurrence of old with new in s. A tiny local
// helper avoids importing strings just for one byte substitution.
func replaceAll(s string, old, new byte) string {
	var changed bool
	for i := 0; i < len(s); i++ {
		if s[i] == old {
			changed = true
			break
		}
	}
	if !changed {
		return s
	}
	b := []byte(s)
	for i := range b {
		if b[i] == old {
			b[i] = new
		}
	}
	return string(b)
}

// Close releases the inner reader's resources.
func (r *Reader) Close() error {
	if r.inner != nil {
		return r.inner.Close()
	}
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
