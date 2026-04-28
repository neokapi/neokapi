//go:build parity

package parity

// filterFQCN maps the short Okapi filter id (as it appears in the
// okapi-bridge v2 manifest at capabilities.formats[].name) to the
// fully qualified Java class name accepted by BridgeService.Process.
//
// The bridge's FilterRegistry.createFilter calls Class.forName on the
// header.filter_class string, so parity tests must pass the FQCN
// even though `kapi formats list` and the manifest show the short id.
//
// This map is the parity harness's compensation for the missing
// `filter_class` field on the v2 manifest — see okapi-bridge issue
// for the upstream fix that would let us drive this from the
// manifest at sandbox prep time.
//
// Pinned to Okapi Framework 1.48.0. When a future release renames or
// adds a filter, update this map (or replace it once the bridge
// publishes filter_class in the manifest).
var filterFQCN = map[string]string{
	"okf_archive":              "net.sf.okapi.filters.archive.ArchiveFilter",
	"okf_autoxliff":            "net.sf.okapi.filters.autoxliff.AutoXLIFFFilter",
	"okf_baseplaintext":        "net.sf.okapi.filters.plaintext.base.BasePlainTextFilter",
	"okf_basetable":            "net.sf.okapi.filters.table.base.BaseTableFilter",
	"okf_commaseparatedvalues": "net.sf.okapi.filters.table.csv.CommaSeparatedValuesFilter",
	"okf_doxygen":              "net.sf.okapi.filters.doxygen.DoxygenFilter",
	"okf_dtd":                  "net.sf.okapi.filters.dtd.DTDFilter",
	"okf_fixedwidthcolumns":    "net.sf.okapi.filters.table.fwc.FixedWidthColumnsFilter",
	"okf_html":                 "net.sf.okapi.filters.html.HtmlFilter",
	"okf_html5":                "net.sf.okapi.filters.its.html5.HTML5Filter",
	"okf_icml":                 "net.sf.okapi.filters.icml.ICMLFilter",
	"okf_idml":                 "net.sf.okapi.filters.idml.IDMLFilter",
	"okf_json":                 "net.sf.okapi.filters.json.JSONFilter",
	"okf_markdown":             "net.sf.okapi.filters.markdown.MarkdownFilter",
	"okf_mif":                  "net.sf.okapi.filters.mif.MIFFilter",
	"okf_mosestext":            "net.sf.okapi.filters.mosestext.MosesTextFilter",
	"okf_multiparsers":         "net.sf.okapi.filters.multiparsers.MultiParsersFilter",
	"okf_odf":                  "net.sf.okapi.filters.openoffice.ODFFilter",
	"okf_openoffice":           "net.sf.okapi.filters.openoffice.OpenOfficeFilter",
	"okf_openxml":              "net.sf.okapi.filters.openxml.OpenXMLFilter",
	"okf_paraplaintext":        "net.sf.okapi.filters.plaintext.paragraphs.ParaPlainTextFilter",
	"okf_pdf":                  "net.sf.okapi.filters.pdf.PdfFilter",
	"okf_pensieve":             "net.sf.okapi.filters.pensieve.PensieveFilter",
	"okf_phpcontent":           "net.sf.okapi.filters.php.PHPContentFilter",
	"okf_plaintext":            "net.sf.okapi.filters.plaintext.PlainTextFilter",
	"okf_po":                   "net.sf.okapi.filters.po.POFilter",
	"okf_properties":           "net.sf.okapi.filters.properties.PropertiesFilter",
	"okf_rainbowkit":           "net.sf.okapi.filters.rainbowkit.RainbowKitFilter",
	"okf_regex":                "net.sf.okapi.filters.regex.RegexFilter",
	"okf_regexplaintext":       "net.sf.okapi.filters.plaintext.regex.RegexPlainTextFilter",
	"okf_rtf":                  "net.sf.okapi.filters.rtf.RTFFilter",
	"okf_sdlpackage":           "net.sf.okapi.filters.sdlpackage.SdlPackageFilter",
	"okf_splicedlines":         "net.sf.okapi.filters.plaintext.spliced.SplicedLinesFilter",
	"okf_table":                "net.sf.okapi.filters.table.TableFilter",
	"okf_tabseparatedvalues":   "net.sf.okapi.filters.table.tsv.TabSeparatedValuesFilter",
	"okf_tex":                  "net.sf.okapi.filters.tex.TEXFilter",
	"okf_tmx":                  "net.sf.okapi.filters.tmx.TmxFilter",
	"okf_transifex":            "net.sf.okapi.filters.transifex.TransifexFilter",
	"okf_transtable":           "net.sf.okapi.filters.transtable.TransTableFilter",
	"okf_ts":                   "net.sf.okapi.filters.ts.TsFilter",
	"okf_ttml":                 "net.sf.okapi.filters.ttml.TTMLFilter",
	"okf_ttx":                  "net.sf.okapi.filters.ttx.TTXFilter",
	"okf_txml":                 "net.sf.okapi.filters.txml.TXMLFilter",
	"okf_vignette":             "net.sf.okapi.filters.vignette.VignetteFilter",
	"okf_vtt":                  "net.sf.okapi.filters.subtitles.VTTFilter",
	"okf_wiki":                 "net.sf.okapi.filters.wiki.WikiFilter",
	"okf_xini":                 "net.sf.okapi.filters.xini.XINIFilter",
	"okf_xinirainbowkit":       "net.sf.okapi.filters.xini.rainbowkit.XINIRainbowKitFilter",
	"okf_xliff":                "net.sf.okapi.filters.xliff.XLIFFFilter",
	"okf_xliff2":               "net.sf.okapi.filters.xliff2.XLIFF2Filter",
	"okf_xml":                  "net.sf.okapi.filters.xml.XMLFilter",
	"okf_xmlstream":            "net.sf.okapi.filters.xmlstream.XmlStreamFilter",
	"okf_yaml":                 "net.sf.okapi.filters.yaml.YamlFilter",
}

// FilterFQCN looks up the fully qualified Java class name for an Okapi
// filter id. Returns the input unchanged if no mapping is registered —
// the bridge will then fail with "cannot instantiate filter: <id>",
// which is the right behavior to surface a missing registry entry.
func FilterFQCN(okapiID string) string {
	if fqcn, ok := filterFQCN[okapiID]; ok {
		return fqcn
	}
	return okapiID
}
