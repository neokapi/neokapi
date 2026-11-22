//go:build cgo

// Package uax29 is a sentence-segmentation engine backed by ICU's UAX-29
// sentence BreakIterator (Unicode Standard Annex #29, "Unicode Text
// Segmentation"). It registers itself under the name "uax29" in the
// [github.com/neokapi/neokapi/core/segment] registry when ICU is linked into
// the binary (any cgo build: ICU is provisioned for every release target).
// On non-cgo builds the build-tagged stub file is compiled instead and the
// engine is absent.
//
// Blank-import this package to make the engine available:
//
//	import _ "github.com/neokapi/neokapi/core/segment/uax29"
//
// then select it via segment.Build("uax29", base, nil) or, in the segment tool's
// recipe, "engine: uax29".
package uax29

import (
	"context"
	"fmt"
	"unicode/utf16"

	"github.com/neokapi/neokapi/core/internal/icu"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/segment"
)

func init() {
	segment.Register(segment.EngineDescriptor{
		Name:        "uax29",
		Label:       "Unicode baseline (UAX-29)",
		Description: "Unicode default sentence boundaries (ICU UAX-29). A language-agnostic baseline with no exceptions.",
		Order:       10,
		New: func(base segment.BaseConfig, _ map[string]any) (segment.Segmenter, error) {
			return New(base), nil
		},
	})
}

// icuEngine is a UAX-29 sentence segmenter. It is stateless apart from the
// resolved configuration, so a single instance is safe for concurrent use:
// each Segment call opens and closes its own ICU break iterator.
type icuEngine struct {
	// lang is the engine-level locale override (cfg.Language). Empty means the
	// per-call locale is used instead.
	lang string
	// mask is the inline-code masking policy applied before boundary
	// detection. It is read-only after construction.
	mask segment.MaskOptions
}

// New builds the "uax29" engine from the shared base options. The engine takes
// no engine-specific parameters.
func New(base segment.BaseConfig) segment.Segmenter {
	return &icuEngine{lang: base.Language, mask: base.Mask}
}

// Layer reports that this engine produces primary sentence segmentation.
func (e *icuEngine) Layer() string { return segment.LayerSentence }

// Segment flattens the runs, runs ICU's sentence break iterator over the
// masked text, and projects the interior sentence boundaries back to
// run-anchored spans.
func (e *icuEngine) Segment(ctx context.Context, runs []model.Run, loc model.LocaleID) ([]model.Span, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Mask inline codes; ICU only ever sees the flattened text. Flattening with
	// the configured mask options keeps boundary detection and span trimming
	// consistent with the other engines.
	fl := segment.Flatten(runs, e.mask)
	runesText := fl.Runes()
	if len(runesText) == 0 {
		return nil, nil
	}

	breaks, err := e.boundaries(runesText, e.locale(loc))
	if err != nil {
		return nil, err
	}
	return fl.Spans(breaks), nil
}

// boundaries converts the masked rune text to UTF-16, drives an ICU sentence
// BreakIterator, and returns the interior boundary positions as rune offsets
// into runes (suitable for [segment.Flattened.Spans]). Boundary 0 and the
// final boundary are excluded.
func (e *icuEngine) boundaries(runes []rune, locale string) ([]int, error) {
	// Encode the rune slice to UTF-16. utf16.Encode emits a surrogate pair for
	// each non-BMP rune, exactly matching what ICU expects in a UChar buffer.
	u16 := utf16.Encode(runes)
	if len(u16) == 0 {
		return nil, nil
	}

	// utf16Index[i] = number of runes that precede UTF-16 unit index i, i.e.
	// the rune offset corresponding to a UTF-16 boundary at i. Building this
	// once lets us map every ICU boundary in O(1) and correctly collapse
	// surrogate pairs (a non-BMP rune occupies two UTF-16 units but one rune).
	utf16ToRune := make([]int, len(u16)+1)
	runeIdx := 0
	for i := range u16 {
		utf16ToRune[i] = runeIdx
		// A high surrogate (0xD800..0xDBFF) is the first unit of a pair; the
		// following low surrogate belongs to the same rune, so we only advance
		// the rune counter on the unit that completes a code point. We advance
		// on every unit that is NOT a high surrogate leading into a low one.
		if !(u16[i] >= 0xD800 && u16[i] <= 0xDBFF) {
			runeIdx++
		}
	}
	utf16ToRune[len(u16)] = runeIdx // == len(runes)

	// ICU returns the boundaries after the first. The final one (== len(u16))
	// is not a segment-interior break, so it is dropped. Everything before it
	// is a sentence boundary.
	positions, err := icu.SentenceBreaks(u16, locale)
	if err != nil {
		return nil, fmt.Errorf("uax29: %w", err)
	}
	breaks := make([]int, 0, len(positions))
	for _, off := range positions {
		if off <= 0 || off >= len(u16) {
			continue // boundary 0 or final boundary
		}
		breaks = append(breaks, utf16ToRune[off])
	}
	return breaks, nil
}

// locale resolves the ICU locale string: the engine-level Language override if
// set, otherwise the per-call locale. BCP-47 hyphens are converted to the
// underscores ICU prefers; an empty/unknown locale falls back to ICU's root
// locale, which is acceptable for sentence breaking.
func (e *icuEngine) locale(loc model.LocaleID) string {
	s := e.lang
	if s == "" {
		s = string(loc)
	}
	return bcp47ToICU(s)
}

func bcp47ToICU(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] == '-' {
			b[i] = '_'
		}
	}
	return string(b)
}
