// Package model defines neokapi's unified content model: the format-agnostic
// representation that any format parses into, that tools edit, check, and
// localize, and that writers serialize back byte-for-byte. It provides the core
// types that flow through channels between tools: [Part], [Block], [Fragment],
// [Layer], and [Span]. A Part carries a typed Resource payload (Block, Data,
// Media, or Layer) and is the fundamental streaming unit in a Flow.
package model
