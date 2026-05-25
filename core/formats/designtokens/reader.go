// Package designtokens implements a first-class neokapi format for the W3C
// Design Tokens Community Group (DTCG) token format — the "Design Tokens"
// interchange format (first stable revision "2025.10",
// https://www.designtokens.org/tr/drafts/format/) used by Style Dictionary,
// Tokens Studio, and the broader design-tokens tooling ecosystem.
//
// DTCG files are ordinary JSON: nested groups whose leaves are token objects
// carrying $value (+ $type, optional $description, $extensions, $deprecated).
// The heavy lifting (tokenizing, byte-faithful round-trip, key-path naming) is
// therefore delegated to core/formats/json. This package adds only the DTCG
// localization scoping that the generic reader cannot infer.
//
// Localization scoping is the whole point of the format. Design tokens are
// overwhelmingly non-linguistic — a $value is a colour, dimension, font
// family, duration, cubic-bézier, or an alias reference like {color.primary} —
// and must never be translated. The single field that carries human-readable,
// translatable prose is $description (token and group documentation). The
// reader configures the JSON reader to extract *only* $description values
// (extractAllPairs=false plus an extractionRules regex targeting the
// $description path segment); every token value and the entire structure pass
// through as non-translatable Data, preserving the byte-faithful round-trip.
package designtokens

import (
	"bytes"
	"context"
	"errors"

	"github.com/neokapi/neokapi/core/format"
	jsonfmt "github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/model"
)

const (
	formatID    = "designtokens"
	displayName = "Design Tokens (DTCG)"
	formatMime  = "application/json"
	// formatExt is the unique DTCG extension. The .tokens.json double extension
	// resolves to .json (→ the generic json format) by extension alone; select
	// this format explicitly with -f designtokens for .tokens.json files.
	formatExt = ".tokens"
)

// Reader implements DataFormatReader for W3C DTCG design-token files. It is a
// thin wrapper over the generic JSON reader: the inner reader does all
// tokenizing, naming, and byte-faithful round-trip bookkeeping; this wrapper
// configures it with the design-tokens preset (extract only $description) and
// relabels the root layer's format to designtokens.
type Reader struct {
	format.BaseFormatReader
	cfg   *Config
	inner *jsonfmt.Reader
}

// NewReader creates a new design-tokens reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        formatID,
			FormatDisplayName: displayName,
			FormatMimeType:    formatMime,
			FormatExtensions:  []string{formatExt},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// SetConfig applies a new configuration after validation, keeping the typed
// design-tokens config in sync so Open builds the right inner JSON config.
func (r *Reader) SetConfig(cfg format.DataFormatConfig) error {
	if err := r.BaseFormatReader.SetConfig(cfg); err != nil {
		return err
	}
	if c, ok := cfg.(*Config); ok {
		r.cfg = c
	}
	return nil
}

// Signature returns detection metadata. The unique .tokens extension is claimed
// outright; a Sniff recognises DTCG content (a token object carrying $value and
// $type) so design-token documents opened without the .tokens extension are
// still recognised. The generic .json extension and application/json MIME are
// deliberately NOT advertised — they are owned by the json format and DTCG
// files commonly use the .tokens.json double extension that resolves to json.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		Extensions: []string{formatExt},
		Sniff:      Sniff,
	}
}

// Sniff reports whether the supplied bytes look like a W3C DTCG design-tokens
// document. The DTCG format defines a token as a JSON object that has a $value
// and (after $type cascade resolution) a $type; the literal "$value" key is
// the single most distinctive marker since $type may be inherited from a parent
// group. Requiring both "$value" and "$type" keeps the sniff specific enough to
// avoid claiming plain JSON, while still recognising real token files (every
// conformant DTCG file declares at least one explicit $type — either on a token
// or, for cascade, on a group). Plain JSON, i18next bundles, and ARB files have
// neither marker and are correctly rejected.
func Sniff(data []byte) bool {
	return bytes.Contains(data, []byte(`"$value"`)) &&
		bytes.Contains(data, []byte(`"$type"`))
}

// Open builds the inner JSON reader from the design-tokens config and opens it.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("designtokens: nil document or reader")
	}
	r.Doc = doc

	inner := jsonfmt.NewReader()
	// Mutate the inner reader's live config in place. The JSON reader keeps a
	// private *Config pointer that Config() returns; calling SetConfig would
	// only swap the embedded base's pointer, leaving the reader's own pointer
	// stale. Applying the design-tokens-derived settings to the live config is
	// the reliable way to configure it.
	if jc, ok := inner.Config().(*jsonfmt.Config); ok {
		r.cfg.applyToJSON(jc)
	}
	if err := inner.Open(ctx, doc); err != nil {
		return err
	}
	r.inner = inner
	return nil
}

// Read delegates to the inner JSON reader and relabels the root layer's format
// to designtokens. Every other part (the $description blocks and the
// non-translatable Data carrying token values and structure) passes through
// untouched so byte-faithful round-trip is preserved.
func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
	out := make(chan model.PartResult, 64)
	in := r.inner.Read(ctx)

	go func() {
		defer close(out)
		for pr := range in {
			if pr.Error == nil && pr.Part != nil && pr.Part.Type == model.PartLayerStart {
				if layer, ok := pr.Part.Resource.(*model.Layer); ok && layer.IsRoot() {
					layer.Format = formatID
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
