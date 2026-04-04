// Package registry provides thread-safe registries for format readers/writers
// and tools. [FormatRegistry] manages available DataFormats with auto-detection
// by MIME type, file extension, and content signatures. [ToolRegistry] manages
// available Tools by name. Both support lazy-loading of plugin-provided
// implementations.
package registry
