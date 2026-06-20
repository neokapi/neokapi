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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/neokapi/neokapi/core/format"
	jsonfmt "github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
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
	cfg           *Config
	inner         *jsonfmt.Reader
	skeletonStore *format.SkeletonStore

	// String-valued $deprecated prose (deprecation message / migration
	// guidance) recovered by a pre-scan in Open (issue #928). DTCG allows
	// $deprecated to be a boolean (a bare deprecation flag, kept as opaque
	// structure) or a string (human-readable guidance). The inner JSON reader
	// drops the string form: it is excluded by the extraction rules and, with
	// non-translatable content surfacing disabled for token values, becomes an
	// opaque Data part whose text is lost. These maps let Read re-surface that
	// prose as semantic context — preferentially as a NoteAnnotation on the
	// token/group's $description block, otherwise as text on the token's own
	// (already-emitted) Data part. Both are parity-safe and need no flag.
	deprecatedNoteByDesc map[string]string // dotted $description key path → deprecation prose
	deprecatedDataText   map[string]string // dotted $deprecated key path  → deprecation prose
}

// Ensure Reader emits a byte-exact skeleton by forwarding the store to the inner
// JSON reader, which does the token-level skeleton emission.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

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

// SetSkeletonStore records the skeleton store and forwards it to the inner JSON
// reader (created in Open), which performs the byte-faithful skeleton emission.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
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

	// Read the document once: the bytes are needed both to pre-scan for string
	// $deprecated prose (surfaced as semantic Note/Data, issue #928) and to feed
	// the inner JSON reader. Bound by the shared safeio byte budget so an
	// oversized stream fails with the same typed error as the inner reader.
	data, err := io.ReadAll(safeio.DefaultBudget().Reader(doc.Reader))
	if err != nil {
		return fmt.Errorf("designtokens: reading: %w", err)
	}
	_ = doc.Reader.Close() // original fully consumed; inner reads the buffered copy
	doc.Reader = io.NopCloser(bytes.NewReader(data))
	r.scanDeprecated(data)

	inner := jsonfmt.NewReader()
	// Mutate the inner reader's live config in place. The JSON reader keeps a
	// private *Config pointer that Config() returns; calling SetConfig would
	// only swap the embedded base's pointer, leaving the reader's own pointer
	// stale. Applying the design-tokens-derived settings to the live config is
	// the reliable way to configure it.
	if jc, ok := inner.Config().(*jsonfmt.Config); ok {
		r.cfg.applyToJSON(jc)
	}
	if r.skeletonStore != nil {
		inner.SetSkeletonStore(r.skeletonStore)
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
			if pr.Error == nil && pr.Part != nil {
				switch pr.Part.Type {
				case model.PartLayerStart:
					if layer, ok := pr.Part.Resource.(*model.Layer); ok && layer.IsRoot() {
						layer.Format = formatID
					}
				case model.PartBlock:
					r.attachDeprecatedNote(pr.Part)
				case model.PartData:
					r.carryDeprecatedText(pr.Part)
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

// scanDeprecated pre-scans the raw DTCG document for string-valued $deprecated
// fields and records where their prose should be surfaced (issue #928). A string
// $deprecated on a token/group that also carries a (string) $description — and
// only when descriptions are extracted — is attached to that $description block
// as a NoteAnnotation, so the deprecation/migration guidance rides the
// translatable block as context. Otherwise (no description, or descriptions
// disabled) the prose is carried on the token's own opaque Data part. Boolean
// $deprecated:true carries no prose and is left as opaque structure. Best-effort:
// a parse failure (e.g. malformed input the inner reader will report, or a
// JSON5-only document) simply yields no surfacing.
func (r *Reader) scanDeprecated(data []byte) {
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return
	}
	r.deprecatedNoteByDesc = make(map[string]string)
	r.deprecatedDataText = make(map[string]string)
	r.collectDeprecated(root, "")
	if len(r.deprecatedNoteByDesc) == 0 {
		r.deprecatedNoteByDesc = nil
	}
	if len(r.deprecatedDataText) == 0 {
		r.deprecatedDataText = nil
	}
}

// collectDeprecated walks the parsed JSON tree, mirroring the inner JSON
// reader's dotted key-path naming (dots between object keys, [i] for array
// indices), and records each string $deprecated value under the path the inner
// reader will use to name the corresponding block ($description) or Data part
// ($deprecated).
func (r *Reader) collectDeprecated(node any, path string) {
	switch n := node.(type) {
	case map[string]any:
		if msg, ok := n["$deprecated"].(string); ok {
			if _, hasDesc := n["$description"].(string); r.cfg.ExtractDescriptions && hasDesc {
				r.deprecatedNoteByDesc[joinDotPath(path, "$description")] = msg
			} else {
				r.deprecatedDataText[joinDotPath(path, "$deprecated")] = msg
			}
		}
		for key, child := range n {
			r.collectDeprecated(child, joinDotPath(path, key))
		}
	case []any:
		for i, child := range n {
			r.collectDeprecated(child, path+"["+strconv.Itoa(i)+"]")
		}
	}
}

// joinDotPath joins a parent key path and a child key with a dot, matching the
// inner JSON reader's buildPath.
func joinDotPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

// attachDeprecatedNote attaches a token/group's string $deprecated prose to its
// $description block as a developer NoteAnnotation (issue #928). Notes are not
// part of the parity canonical stream and do not change the part stream, so this
// is parity-safe and needs no flag.
func (r *Reader) attachDeprecatedNote(part *model.Part) {
	if len(r.deprecatedNoteByDesc) == 0 {
		return
	}
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return
	}
	if msg, found := r.deprecatedNoteByDesc[descriptionKeyPath(block)]; found {
		block.AddNote(&model.NoteAnnotation{Text: msg, From: "developer"})
	}
}

// carryDeprecatedText carries a token/group's string $deprecated prose onto its
// already-emitted (opaque) Data part as Properties["text"], for deprecated nodes
// that have no $description block to annotate (issue #928). Parity compares only
// Data.ID, so an added property is invisible to it and the part stream is
// otherwise unchanged — parity-safe, no flag.
func (r *Reader) carryDeprecatedText(part *model.Part) {
	if len(r.deprecatedDataText) == 0 {
		return
	}
	data, ok := part.Resource.(*model.Data)
	if !ok {
		return
	}
	if msg, found := r.deprecatedDataText[data.Name]; found {
		if data.Properties == nil {
			data.Properties = make(map[string]string)
		}
		data.Properties["text"] = msg
	}
}

// descriptionKeyPath returns the dotted JSON key path of a block. The
// design-tokens reader names $description blocks by their full slash path, so
// the dotted path lives in the json.keypath property; fall back to the name.
func descriptionKeyPath(block *model.Block) string {
	if kp, ok := block.Properties["json.keypath"]; ok {
		return kp
	}
	return block.Name
}
