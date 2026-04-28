package preset

import "strings"

// FormatRef identifies a format, optionally pinned to a specific version
// and/or preset.
//
// Syntax: name[@version][:preset]
//
//	okf_openxml                 — latest version, default config
//	okf_openxml@0.38            — pinned version, default config
//	okf_openxml:wellFormed      — latest version, preset config
//	okf_openxml@0.38:wellFormed — pinned version + preset config
type FormatRef struct {
	Name    string // e.g. "okf_openxml"
	Version string // e.g. "1.46.0"; empty means "latest"
	Preset  string // e.g. "wellFormed"; empty means default config
}

// ParseFormatRef parses a format reference string using the syntax
// name[@version][:preset]. The "@" separator denotes a version and ":"
// denotes a preset; both are optional and can be combined (e.g.
// "okf_openxml@0.38:wellFormed").
func ParseFormatRef(s string) FormatRef {
	var ref FormatRef

	// Split on ":" first to extract preset.
	if i := strings.Index(s, ":"); i > 0 {
		ref.Preset = s[i+1:]
		s = s[:i]
	}

	// Split on "@" to extract version.
	if i := strings.LastIndex(s, "@"); i > 0 {
		ref.Version = s[i+1:]
		s = s[:i]
	}

	ref.Name = s
	return ref
}

// String returns the canonical string representation: name[@version][:preset].
func (r FormatRef) String() string {
	var b strings.Builder
	b.WriteString(r.Name)
	if r.Version != "" {
		b.WriteByte('@')
		b.WriteString(r.Version)
	}
	if r.Preset != "" {
		b.WriteByte(':')
		b.WriteString(r.Preset)
	}
	return b.String()
}

// RegistryName returns the name used for format registry lookups:
// "name@version" if versioned, or just "name" for latest.
func (r FormatRef) RegistryName() string {
	if r.Version != "" {
		return r.Name + "@" + r.Version
	}
	return r.Name
}

// IsVersioned reports whether this ref specifies an explicit version.
func (r FormatRef) IsVersioned() bool {
	return r.Version != ""
}

// IsPreset reports whether this ref specifies a preset.
func (r FormatRef) IsPreset() bool {
	return r.Preset != ""
}
