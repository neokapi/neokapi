package xcstrings

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"unsafe"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for Apple String Catalog (.xcstrings) files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new Apple String Catalog reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "xcstrings",
			FormatDisplayName: "Apple String Catalog",
			FormatMimeType:    "application/json",
			FormatExtensions:  []string{".xcstrings"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/json"},
		Extensions: []string{".xcstrings"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("xcstrings: nil document or reader")
	}
	r.Doc = doc
	return nil
}

// Read returns a channel of PartResults.
func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 64)
	go func() {
		defer close(ch)
		r.readContent(ctx, ch)
	}()
	return ch
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("xcstrings: reading: %w", err)}
		return
	}

	cat, err := parseCatalog(content)
	if err != nil {
		ch <- model.PartResult{Error: err}
		return
	}

	srcLocale := model.LocaleID(cat.SourceLanguage)
	if srcLocale.IsEmpty() {
		srcLocale = r.Doc.SourceLocale
	}
	if srcLocale.IsEmpty() {
		srcLocale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:             "doc1",
		Name:           r.Doc.URI,
		Format:         "xcstrings",
		Locale:         srcLocale,
		Encoding:       r.Doc.Encoding,
		MimeType:       "application/json",
		IsMultilingual: true,
		Properties: map[string]string{
			"xcstrings.sourceLanguage": cat.SourceLanguage,
			"xcstrings.version":        cat.Version,
		},
	}
	// Preserve the original document bytes so the writer can produce
	// byte-faithful output, splicing only changed values. unsafe.String
	// shares the backing array — content is not mutated after this point.
	layer.Properties["xcstrings.original"] = unsafe.String(unsafe.SliceData(content), len(content))

	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	blockCounter := 0
	for _, key := range cat.keys {
		e := cat.entries[key]
		if e.ExtractionState == "stale" && !r.cfg.ExtractStale {
			continue
		}
		r.emitEntry(ctx, ch, cat, key, e, srcLocale, &blockCounter)
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// emitEntry emits one Block per translatable leaf value in the entry.
func (r *Reader) emitEntry(ctx context.Context, ch chan<- model.PartResult,
	cat *catalog, key string, e *entry, srcLocale model.LocaleID, counter *int) {

	srcLang := cat.SourceLanguage

	for _, lang := range e.langOrder {
		loc := e.localizations[lang]
		r.emitLocalization(ctx, ch, key, e, lang, loc, srcLang, srcLocale, counter)
	}
}

// emitLocalization emits the leaf blocks for a single language's localization.
func (r *Reader) emitLocalization(ctx context.Context, ch chan<- model.PartResult,
	key string, e *entry, lang string, loc *localization, srcLang string,
	srcLocale model.LocaleID, counter *int) {

	switch {
	case loc.StringUnit != nil:
		vr := valueRef{Key: key, Lang: lang, Kind: kindStringUnit}
		r.emitLeaf(ctx, ch, e, vr, loc.StringUnit, key, srcLang, srcLocale, counter)
	case loc.Variations != nil:
		r.emitVariations(ctx, ch, e, key, lang, loc.Variations, "", srcLang, srcLocale, counter)
	}
}

// emitVariations walks plural/device/substitution subtrees, emitting a leaf
// block per stringUnit found. sub is the substitution name when recursing into
// a substitution's own variation subtree (empty at the top level).
func (r *Reader) emitVariations(ctx context.Context, ch chan<- model.PartResult,
	e *entry, key, lang string, v *variations, sub string,
	srcLang string, srcLocale model.LocaleID, counter *int) {

	pluralKind := kindPlural
	deviceKind := kindDevice
	if sub != "" {
		pluralKind = kindSubstitutionPlural
		deviceKind = kindSubstitutionDevice
	}

	for _, cat := range v.pluralOrder {
		su := v.plural[cat]
		if su == nil {
			continue
		}
		vr := valueRef{Key: key, Lang: lang, Kind: pluralKind, Sub: sub, Category: cat}
		r.emitLeaf(ctx, ch, e, vr, su, key, srcLang, srcLocale, counter)
	}
	for _, cat := range v.deviceOrder {
		su := v.device[cat]
		if su == nil {
			continue
		}
		vr := valueRef{Key: key, Lang: lang, Kind: deviceKind, Sub: sub, Category: cat}
		r.emitLeaf(ctx, ch, e, vr, su, key, srcLang, srcLocale, counter)
	}
	for _, name := range v.substitutionOrder {
		s := v.substitutions[name]
		if s == nil || s.vars == nil {
			continue
		}
		r.emitVariations(ctx, ch, e, key, lang, s.vars, name, srcLang, srcLocale, counter)
	}
}

// emitLeaf emits a single Block for a leaf stringUnit. The value is carried as
// source content when lang is the source language, otherwise as a target for
// that locale. The matching source value (or, absent a source localization,
// the entry key) is used as the Block's source so the block is self-contained.
func (r *Reader) emitLeaf(ctx context.Context, ch chan<- model.PartResult,
	e *entry, vr valueRef, su *stringUnit, key, srcLang string,
	srcLocale model.LocaleID, counter *int) {

	*counter++
	blockID := "tu" + strconv.Itoa(*counter)

	// Determine the source text for this leaf: the corresponding source-language
	// value at the same location, falling back to the entry key for plain
	// stringUnits.
	srcValue := r.sourceValueFor(e, vr, srcLang, key)

	block := &model.Block{
		ID:           blockID,
		Name:         vr.blockName(),
		Translatable: true,
		SourceLocale: srcLocale,
		Source:       []*model.Segment{{ID: "s1", Runs: runsFromValue(srcValue)}},
		Targets:      make(map[model.LocaleID][]*model.Segment),
		Properties:   make(map[string]string),
		Annotations:  make(map[string]model.Annotation),
	}

	vr.applyToBlockProps(block)
	block.Properties["xcstrings.value"] = su.Value
	if su.State != "" {
		block.Properties["state"] = su.State
	}

	// Developer comment becomes a block note.
	if e.Comment != "" {
		block.Annotations["note"] = &model.NoteAnnotation{
			Text:      e.Comment,
			From:      "developer",
			Annotates: "general",
		}
	}
	if e.ExtractionState != "" {
		block.Properties["xcstrings.extractionState"] = e.ExtractionState
	}

	lang := model.LocaleID(vr.Lang)
	if lang == srcLocale && vr.Lang == srcLang {
		// Source-language leaf — the value IS the source content.
		block.Source = []*model.Segment{{ID: "s1", Runs: runsFromValue(su.Value)}}
	} else {
		// Target leaf — value lives under the target locale.
		block.Targets[lang] = []*model.Segment{{ID: "t1", Runs: runsFromValue(su.Value)}}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

// sourceValueFor returns the source-language leaf value matching vr's location,
// falling back to the entry key when no source localization exists (the common
// case: Apple uses the key as the source string).
func (r *Reader) sourceValueFor(e *entry, vr valueRef, srcLang, key string) string {
	srcLoc := e.localizations[srcLang]
	if srcLoc == nil {
		return key
	}
	switch vr.Kind {
	case kindStringUnit:
		if srcLoc.StringUnit != nil {
			return srcLoc.StringUnit.Value
		}
	case kindPlural:
		if srcLoc.Variations != nil {
			if su := srcLoc.Variations.plural[vr.Category]; su != nil {
				return su.Value
			}
		}
	case kindDevice:
		if srcLoc.Variations != nil {
			if su := srcLoc.Variations.device[vr.Category]; su != nil {
				return su.Value
			}
		}
	case kindSubstitutionPlural, kindSubstitutionDevice:
		if srcLoc.Variations != nil {
			if sub := srcLoc.Variations.substitutions[vr.Sub]; sub != nil && sub.vars != nil {
				m := sub.vars.plural
				if vr.Kind == kindSubstitutionDevice {
					m = sub.vars.device
				}
				if su := m[vr.Category]; su != nil {
					return su.Value
				}
			}
		}
	}
	return key
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
