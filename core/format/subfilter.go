package format

// SubfilterResolver creates format readers/writers for embedded content.
// Format readers that support subfiltering receive this via SetSubfilterResolver
// and use it to delegate embedded content parsing to another format.
//
// FormatRegistry naturally implements this interface via its NewReader/NewWriter
// methods. The interface decouples format packages from the registry package,
// preventing circular imports and enabling test mocks.
type SubfilterResolver interface {
	ResolveReader(formatName string) (DataFormatReader, error)
	ResolveWriter(formatName string) (DataFormatWriter, error)
}

// SubfilterMapping maps content locations to a format for subfiltering.
// Pattern syntax depends on the parent format:
//   - JSON: key path glob (e.g., "*.body", "translations.*.html")
//   - XML: element name or XPath-like expression
//   - CSV: column index
type SubfilterMapping struct {
	Pattern string // content location pattern
	Format  string // format reader/writer name (e.g., "html", "markdown")
}

// SubfilterAware marks format readers/writers that support subfiltering.
// Readers implementing this interface will have their resolver set by the
// pipeline before Open is called.
type SubfilterAware interface {
	SetSubfilterResolver(r SubfilterResolver)
}
