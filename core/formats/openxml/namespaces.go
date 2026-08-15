// Namespace bookkeeping shared by the OpenXML readers: the prefix registry that
// maps a namespace URI back to the prefix the source used, the static prefix
// table it falls back to, and the WordprocessingML namespace predicates.

package openxml

import (
	"encoding/xml"
	"strings"
	"sync"
)

// nsRegistry tracks namespace URI → prefix mappings discovered during parsing.
// It supplements the static nsPrefixMap with dynamic mappings from xmlns: attributes.
var nsRegistry = struct {
	sync.RWMutex
	m map[string]string
}{m: make(map[string]string)}

// isolateNamespaces snapshots the global namespace registry and replaces it with
// an empty one for the duration of a nested parse, returning a restore func to
// defer. Drawing-payload extraction decodes its fragment under a synthetic
// wrapper that declares the canonical OpenXML prefixes (wrapDrawingXMLForDecode),
// so resolving the drawing's element prefixes against an empty registry — which
// falls through to the static nsPrefixMap — keeps that serialization
// deterministic and immune to a *different* document's leaked declaration (e.g.
// a prior file binding the markup-compatibility namespace to `ve:` instead of
// the canonical `mc:`). The surrounding document's registry is restored
// afterward, so its own — possibly non-canonical — prefixes still round-trip
// unchanged. Without this, test/processing order (or `-shuffle`) decided whether
// a drawing emitted `mc:` or a leaked `ve:`.
func isolateNamespaces() func() {
	nsRegistry.Lock()
	saved := nsRegistry.m
	nsRegistry.m = map[string]string{}
	nsRegistry.Unlock()
	return func() {
		nsRegistry.Lock()
		nsRegistry.m = saved
		nsRegistry.Unlock()
	}
}

// registerNamespaces scans an element's attributes for xmlns declarations
// and records the URI → prefix mapping.
func registerNamespaces(attrs []xml.Attr) {
	nsRegistry.Lock()
	for _, a := range attrs {
		if a.Name.Space == "xmlns" {
			// xmlns:prefix="URI" → map URI to prefix
			nsRegistry.m[a.Value] = a.Name.Local
		} else if a.Name.Space == "" && a.Name.Local == "xmlns" {
			// xmlns="URI" (default namespace) → map URI to "" (no prefix)
			nsRegistry.m[a.Value] = ""
		}
	}
	nsRegistry.Unlock()
}

// resolvePrefix returns the namespace prefix for a URI, checking the dynamic
// registry first (which reflects the document's actual declarations), then
// falling back to the static map.
func resolvePrefix(ns string) string {
	nsRegistry.RLock()
	p, ok := nsRegistry.m[ns]
	nsRegistry.RUnlock()
	if ok {
		return p
	}
	if p, ok := nsPrefixMap[ns]; ok {
		return p
	}
	return ""
}

// writeElementName writes an element name with its namespace prefix.
func writeElementName(buf *strings.Builder, name xml.Name) {
	if name.Space != "" {
		prefix := resolvePrefix(name.Space)
		if prefix != "" {
			buf.WriteString(prefix)
			buf.WriteString(":")
		}
		// If no known prefix, write local name only — the namespace is
		// already declared on a parent element via xmlns.
	}
	buf.WriteString(name.Local)
}

// writeAttrName writes an attribute name, handling xmlns declarations.
func writeAttrName(buf *strings.Builder, name xml.Name) {
	if name.Space == "xmlns" {
		// Namespace declaration: xmlns:prefix
		buf.WriteString("xmlns:")
		buf.WriteString(name.Local)
		return
	}
	if name.Space == "" && name.Local == "xmlns" {
		// Default namespace declaration
		buf.WriteString("xmlns")
		return
	}
	if name.Space != "" {
		prefix := resolvePrefix(name.Space)
		if prefix != "" {
			buf.WriteString(prefix)
			buf.WriteString(":")
		}
		// Unknown namespace — omit the prefix. The namespace is
		// already declared on a parent element and the attribute
		// name alone is sufficient for well-formed output.
	}
	buf.WriteString(name.Local)
}

// nsPrefix maps namespace URI → prefix for known OpenXML namespaces.
//
// Strict OOXML (ISO/IEC 29500-1 §A.1) variants for the core
// drawingml/wordprocessingDrawing/officeDocument-math URIs share
// the same canonical prefix as their transitional siblings — the
// nsPrefixMap is consulted to write the prefix back when the source
// element bound `a:` (or `wp:`/`m:`) to a strict URI. 859.docx is
// the canonical fixture: a strict-conformance document whose
// drawing payload binds `a:` to `http://purl.oclc.org/ooxml/
// drawingml/main`. Without these entries, captureRawElement's
// writeElementName falls through to the unknown-prefix path and
// emits the element without a prefix (e.g. `<graphicFrameLocks
// xmlns:a="..."/>` instead of `<a:graphicFrameLocks xmlns:a="..."/>`),
// which the canonicalizer interprets as default-namespace and
// diverges from upstream.
var nsPrefixMap = map[string]string{
	wmlNamespace:       "w",
	wmlStrictNamespace: "w",
	dmlNamespace:       "a",
	"http://purl.oclc.org/ooxml/drawingml/main":                                 "a",
	"http://schemas.openxmlformats.org/officeDocument/2006/relationships":       "r",
	"http://purl.oclc.org/ooxml/officeDocument/relationships":                   "r",
	"http://schemas.openxmlformats.org/markup-compatibility/2006":               "mc",
	"http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing":    "wp",
	"http://purl.oclc.org/ooxml/drawingml/wordprocessingDrawing":                "wp",
	"http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing":       "xdr",
	"http://schemas.openxmlformats.org/drawingml/2006/chart":                    "c",
	"http://schemas.openxmlformats.org/drawingml/2006/diagram":                  "dgm",
	"http://schemas.openxmlformats.org/drawingml/2006/picture":                  "pic",
	"http://schemas.openxmlformats.org/officeDocument/2006/math":                "m",
	"http://purl.oclc.org/ooxml/officeDocument/math":                            "m",
	"http://schemas.openxmlformats.org/officeDocument/2006/extended-properties": "ep",
	"http://schemas.openxmlformats.org/officeDocument/2006/custom-properties":   "cp",
	"http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes":      "vt",
	"http://schemas.openxmlformats.org/spreadsheetml/2006/main":                 "x",
	"http://schemas.openxmlformats.org/presentationml/2006/main":                "p",
	"http://schemas.openxmlformats.org/package/2006/relationships":              "pr",
	"http://schemas.openxmlformats.org/package/2006/content-types":              "ct",
	"http://schemas.openxmlformats.org/package/2006/metadata/core-properties":   "coreProperties",
	"http://schemas.microsoft.com/office/word/2010/wordml":                      "w14",
	"http://schemas.microsoft.com/office/word/2012/wordml":                      "w15",
	"http://schemas.microsoft.com/office/word/2015/wordml/symex":                "w16se",
	"http://schemas.microsoft.com/office/spreadsheetml/2009/9/main":             "x14",
	"http://schemas.microsoft.com/office/spreadsheetml/2010/11/main":            "x15",
	"http://schemas.microsoft.com/office/powerpoint/2010/main":                  "p14",
	"http://schemas.microsoft.com/office/powerpoint/2012/main":                  "p15",
	"http://schemas.microsoft.com/office/drawing/2010/main":                     "a14",
	"http://schemas.microsoft.com/office/drawing/2014/main":                     "a16",
	// Mac DrawingML extension namespace used by `<ma14:wrappingTextBoxFlag>`
	// inside DrawingML `<a:ext>` elements (ECMA-376 Part 1 §20.1 / Microsoft
	// Office DrawingML extensions). Hidden_Textbox.docx is the canonical
	// fixture — without this entry writeElementName falls into the
	// unknown-prefix path and emits `<wrappingTextBoxFlag xmlns:ma14="..."/>`
	// instead of `<ma14:wrappingTextBoxFlag xmlns:ma14="..."/>`, which the
	// canon comparator interprets as default-namespace and flags as
	// divergent (the canonical xmlns="..." pseudo-declaration is missing).
	"http://schemas.microsoft.com/office/mac/drawingml/2011/main":     "ma14",
	"http://purl.org/dc/elements/1.1/":                                "dc",
	"http://purl.org/dc/terms/":                                       "dcterms",
	"http://schemas.openxmlformats.org/officeDocument/2006/customXml": "ds",
	"urn:schemas-microsoft-com:vml":                                   "v",
	"urn:schemas-microsoft-com:office:office":                         "o",
	"urn:schemas-microsoft-com:office:word":                           "w10",
	"http://www.w3.org/2001/XMLSchema-instance":                       "xsi",
	"http://www.w3.org/2001/XMLSchema":                                "xsd",
	"http://www.w3.org/XML/1998/namespace":                            "xml",
	// Microsoft Office extension namespaces
	"http://schemas.microsoft.com/office/word/2010/wordprocessingCanvas":  "wpc",
	"http://schemas.microsoft.com/office/word/2010/wordprocessingDrawing": "wp14",
	"http://schemas.microsoft.com/office/word/2010/wordprocessingGroup":   "wpg",
	"http://schemas.microsoft.com/office/word/2010/wordprocessingInk":     "wpi",
	"http://schemas.microsoft.com/office/word/2010/wordprocessingShape":   "wps",
	"http://schemas.microsoft.com/office/word/2006/wordml":                "wne",
	"http://schemas.microsoft.com/office/mac/office/2008/main":            "mo",
	"urn:schemas-microsoft-com:mac:vml":                                   "mv",
	"http://schemas.microsoft.com/office/drawing/2012/chart":              "c15",
	"http://schemas.microsoft.com/office/drawing/2014/chartex":            "cx",
	"http://schemas.openxmlformats.org/drawingml/2006/lockedCanvas":       "lc",
	"http://schemas.microsoft.com/office/drawing/2008/diagram":            "dsp",
	"http://schemas.microsoft.com/office/drawing/2010/diagram":            "dgm14",
	"http://schemas.microsoft.com/office/thememl/2012/main":               "thm15",
	"http://schemas.microsoft.com/office/drawing/2017/decorative":         "adec",
	"http://schemas.microsoft.com/office/drawing/2018/hyperlinkcolor":     "ahlc",
	"http://schemas.microsoft.com/office/word/2016/wordml/cid":            "w16cid",
	"http://schemas.microsoft.com/office/word/2018/wordml":                "w16",
	"http://schemas.microsoft.com/office/word/2018/wordml/cex":            "w16cex",
	"http://schemas.microsoft.com/office/word/2020/wordml/sdtdatahash":    "w16sdtdh",
}

func isWML(el xml.StartElement) bool {
	return el.Name.Space == wmlNamespace || el.Name.Space == wmlStrictNamespace
}

func isWMLNoNS(el xml.StartElement) bool {
	return el.Name.Space == ""
}
