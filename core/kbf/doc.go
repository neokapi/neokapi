// Package kbf implements the Kapi Bundle Format (.kbf.json), a JSON
// serialization of the Block / Run model specified in Framework AD-002.
//
// A .kbf.json file is a UTF-8 JSON document carrying one or more extracted
// documents, each with a flat sequence of Blocks. A Block carries its content
// as peer editions keyed by edition key (`fr`, `fr;tone=formal`,
// `en;channel=short`): the edition it was read in under the empty key, every
// translation and other edition under its own, each with its status,
// provenance and the edition it was derived from. Beside them sit
// Placeholder[] metadata and translator-facing Properties. Run is a
// discriminated union: text, placeholder, paired code (pcOpen/pcClose),
// subblock reference, or structured plural/select construct.
//
// The current schema is 2.0. A file in schema 1.0, whose blocks carried
// `source` runs beside `targets` keyed by locale, reads as the editions it
// describes, so every caller sees one shape.
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
package kbf
