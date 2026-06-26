// Package archive provides a read-only reader that looks inside an archive
// container (ZIP, TAR, TAR.GZ) so `kapi inspect` / analysis tools can surface
// the translatable content of each recognised entry. It is the *inspection*
// face of the container model.
//
// Processing that produces output (translate, pseudo-translate, …) does NOT go
// through this reader: a container is localized via the container binding
// (AD-026 §6) — each entry is run as its own file with a real reader/writer and
// skeleton round-trip, then the results are repacked. That binding lives in the
// CLI run path over the provider-agnostic core/container substrate; this package
// only reads. There is intentionally no archive writer.
package archive

import (
	"github.com/neokapi/neokapi/core/format"
)

// skipFormats are formats whose content is not worth surfacing as inline blocks
// during inspection: binary assets (whole-asset localisation) and nested
// containers (shown as a single Data entry rather than recursed).
var skipFormats = map[string]bool{
	"image":   true,
	"audio":   true,
	"video":   true,
	"pdf":     true,
	"archive": true,
}

// canInspect reports whether an entry of the given detected format should be
// parsed through its own reader to surface its content (true), or listed as a
// single Data entry (false). Reading needs only a reader; the write-time
// generativity/interchange concerns do not apply here.
func canInspect(resolver format.SubfilterResolver, fmtName string) bool {
	if fmtName == "" || skipFormats[fmtName] {
		return false
	}
	_, err := resolver.ResolveReader(fmtName)
	return err == nil
}
