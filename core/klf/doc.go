// Package klf implements the Kapi Localization Format (.klf) — a JSON
// serialization of the Block / Run model defined in RFC 0001.
//
// A .klf file is a UTF-8 JSON document carrying one or more extracted
// documents, each with a flat sequence of Blocks. Blocks carry a
// Run[] source (and optional per-locale targets), Placeholder[]
// metadata, and translator-facing Properties. Run is a discriminated
// union: text, placeholder, paired code (pcOpen/pcClose), subblock
// reference, or structured plural/select construct.
//
// This package is the Go port of the TypeScript @neokapi/format types.
// Data shapes match @neokapi/format/src/block.ts byte-for-byte after
// JSON serialization, enforced by shared golden fixtures.
//
// The package also implements the annotation sidecar layer
// (@neokapi/format/src/annotation.ts): AnnotationFile, Annotation,
// four AnnotationAnchor shapes (block/run/range/form), RunPath,
// ResolveAnchor and ValidateAnchor with six machine-readable failure
// reasons.
//
// Phase 1 (RFC 0001) is purely additive: this package does not
// replace the existing core/model Fragment/Span model. The Phase 2
// migration that unifies core/model with the Run shape is tracked
// separately and lands under its own PR.
//
// Zero dependencies beyond the standard library.
package klf
