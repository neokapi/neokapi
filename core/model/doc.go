// Package model defines the content model for document processing pipelines.
// It provides the core types that flow through channels between tools: [Part],
// [Block], [Fragment], [Layer], and [Span]. A Part carries a typed Resource
// payload (Block, Data, Media, or Layer) and is the fundamental streaming unit
// in a Flow.
package model
