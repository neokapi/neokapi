package formats

import "github.com/neokapi/neokapi/core/registry"

// registerSourceCode is a no-op: source files are read by the kapi-sourcecode
// plugin (cgo + tree-sitter grammars), which registers its "sourcecode" reader
// at runtime via the plugin format factory.
//
// Building it into core would pull cgo and a grammar per language into every
// kapi binary — including the wasm build, which cannot link them at all — and
// would let a parser fault on a malformed file crash the process. The plugin
// keeps both isolated, exactly as kapi-pdfium does for PDF. With no plugin
// installed, sourcecode is simply an unsupported format.
//
// What core does keep is core/formats/sourcecode.Config, which the plugin
// imports: the config has one definition rather than one on each side of the
// boundary.
func registerSourceCode(*registry.FormatRegistry) {}
