// Package klf implements the Kapi Localization Format (.klf) — a JSON
// serialization of the Block / Run model specified in Framework AD-002.
//
// A .klf file is a UTF-8 JSON document carrying one or more extracted
// documents, each with a flat sequence of Blocks. Blocks carry a
// Run[] source (and optional per-locale targets), Placeholder[]
// metadata, and translator-facing Properties. Run is a discriminated
// union: text, placeholder, paired code (pcOpen/pcClose), subblock
// reference, or structured plural/select construct.
//
// This package is the Go half of the format. The TypeScript half
// lives in packages/kapi-format (@neokapi/kapi-format). Data shapes match
// packages/kapi-format/src/block.ts byte-for-byte after JSON
// serialization, enforced by shared golden fixtures under
// packages/kapi-format/examples.
//
// The package also implements the annotation overlay layer
// (packages/kapi-format/src/annotation.ts): AnnotationFile, Annotation,
// four AnnotationAnchor shapes (block/run/range/form), RunPath,
// ResolveAnchor and ValidateAnchor with six machine-readable failure
// reasons.
//
// Zero dependencies beyond the standard library.
package klf
