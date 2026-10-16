package preset

import "strings"

// FormatRef identifies a format, optionally naming the engine that serves
// it and pinned to a specific version and/or preset.
//
// Syntax: name[@engine][@version][:preset]
//
//	okf_openxml                : latest version, default config
//	okf_openxml@0.38           : pinned version, default config
//	okf_openxml:wellFormed     : latest version, preset config
//	okf_openxml@0.38:wellFormed: pinned version + preset config
//	html@okapi-bridge          : the okapi-bridge format for HTML
//	html@native                : the built-in format for HTML
//
// A token after "@" that starts with a digit is a version; any other is an
// engine. The engine resolves against the registry (FormatRegistry.EngineFormat),
// so RegistryName carries the name as written and the host applies the engine.
type FormatRef struct {
	Name    string // e.g. "okf_openxml"
	Engine  string // e.g. "okapi-bridge"; empty names no engine
	Version string // e.g. "1.46.0"; empty means "latest"
	Preset  string // e.g. "wellFormed"; empty means default config
}

// ParseFormatRef parses a format reference string using the syntax
// name[@engine][@version][:preset]. The "@" separator introduces an engine or
// a version (a version starts with a digit) and ":" a preset; all are
// optional and combine (e.g. "html@okapi-bridge@1.47.0:strict").
func ParseFormatRef(s string) FormatRef {
	var ref FormatRef

	// Split on ":" first to extract preset.
	if i := strings.Index(s, ":"); i > 0 {
		ref.Preset = s[i+1:]
		s = s[:i]
	}

	// Split on "@" to extract the engine and the version.
	parts := strings.Split(s, "@")
	ref.Name = parts[0]
	if ref.Name == "" {
		// A leading "@" belongs to the name, as it did before engines were
		// part of a reference.
		ref.Name = s
		return ref
	}
	for _, p := range parts[1:] {
		switch {
		case p == "":
		case isVersionToken(p):
			ref.Version = p
		default:
			ref.Engine = p
		}
	}
	return ref
}

// isVersionToken reports a token after "@" that names a version rather than
// an engine: one that starts with a digit.
func isVersionToken(s string) bool {
	return s != "" && s[0] >= '0' && s[0] <= '9'
}

// String returns the canonical string representation:
// name[@engine][@version][:preset].
func (r FormatRef) String() string {
	var b strings.Builder
	b.WriteString(r.Name)
	if r.Engine != "" {
		b.WriteByte('@')
		b.WriteString(r.Engine)
	}
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
// "name@version" if versioned, or just "name" for latest. The engine is not
// part of it: a host resolves the engine's format with
// FormatRegistry.EngineFormat and then versions that.
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

// HasEngine reports whether this ref names the engine that serves it.
func (r FormatRef) HasEngine() bool {
	return r.Engine != ""
}
