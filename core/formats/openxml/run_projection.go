// How a run's properties become inline codes, shared by the two OpenXML markup
// families that project them the same way.

package openxml

import "slices"

// runPropsProjection turns one run's properties into inline codes: one declared
// code per formatting type the model names, then one opaque paired code holding
// what it does not.
//
// SpreadsheetML rich text and DrawingML text reach the same answer from
// different markup. A CT_Rst run states its formatting as <rPr> children
// (ECMA-376 Part 1 §18.4.7); a DrawingML run states most of it as attributes on
// <a:rPr> (§21.1.2.3.9). What both need is the same: a translator sees where
// the bold and the underline begin, and everything else survives as bytes the
// writer replays. Keeping the order, the pairing and the set of named types in
// one place is what keeps the two families in step, so a formatting type added
// to the model reaches both.
//
// A family supplies the source bytes each half carries and the vocabulary the
// opaque code is tagged with.
type runPropsProjection struct {
	// namedXML returns the source bytes behind one named formatting type.
	// SpreadsheetML returns the child element that declared it, so
	// `<u val="double"/>` survives; DrawingML returns nothing, because its
	// whole <a:rPr> travels on the opaque code and the writer rewrites the
	// named attributes in place.
	namedXML func(rp runProps, spanType string) string
	// opaqueXML returns the source bytes the model does not name, empty when
	// the run carries none.
	opaqueXML func(rp runProps) string

	opaqueType    string
	opaqueSubType string
	// attrKey is the run-attribute key the source bytes travel under. They
	// stay off the code's Data, which is what a writer replays for a code
	// whose type it does not know.
	attrKey string
}

// namedRunProp is one formatting type the model names, and the test that says
// whether a run carries it. The order is the order the codes open in, so a
// close walks it backwards and the ids pair.
type namedRunProp struct {
	spanType string
	subType  string
	on       func(rp runProps) bool
}

var namedRunProps = []namedRunProp{
	{TypeBold, SubTypeBold, func(rp runProps) bool { return rp.bold }},
	{TypeItalic, SubTypeItalic, func(rp runProps) bool { return rp.italic }},
	{TypeUnderline, SubTypeUnderline, func(rp runProps) bool { return rp.underline != "" }},
	{TypeStrikethrough, SubTypeStrikethrough, func(rp runProps) bool { return rp.strike }},
	{TypeSuperscript, SubTypeSuperscript, func(rp runProps) bool { return rp.vertAlign == "superscript" }},
	{TypeSubscript, SubTypeSubscript, func(rp runProps) bool { return rp.vertAlign == "subscript" }},
}

// appendOpening emits the opening codes for a run's properties.
func (pj runPropsProjection) appendOpening(rp runProps, b *runBuilder, ids *spanIDs) {
	emit := func(spanType, subType, raw string) {
		var attrs map[string]string
		if raw != "" {
			attrs = map[string]string{pj.attrKey: raw}
		}
		b.AddPcOpenAttrs(ids.openSpan(), spanType, subType, "", "", "", true, true, true, attrs)
	}
	for _, n := range namedRunProps {
		if n.on(rp) {
			emit(n.spanType, n.subType, pj.namedXML(rp, n.spanType))
		}
	}
	if other := pj.opaqueXML(rp); other != "" {
		emit(pj.opaqueType, pj.opaqueSubType, other)
	}
}

// appendClosing closes what appendOpening opened, innermost first.
func (pj runPropsProjection) appendClosing(rp runProps, b *runBuilder, ids *spanIDs) {
	emit := func(spanType, subType string) {
		b.AddPcClose(ids.closeSpan(), spanType, subType, "", "")
	}
	if pj.opaqueXML(rp) != "" {
		emit(pj.opaqueType, pj.opaqueSubType)
	}
	for _, n := range slices.Backward(namedRunProps) {
		if n.on(rp) {
			emit(n.spanType, n.subType)
		}
	}
}

// smlRunPropsProjection is the SpreadsheetML binding: each named code carries
// the <rPr> child that declared it, and the opaque code carries the children
// the model does not name, in source order.
var smlRunPropsProjection = runPropsProjection{
	namedXML: func(rp runProps, spanType string) string {
		return rp.smlNamedRPrXML(smlRPrChildName(spanType))
	},
	opaqueXML:     runProps.smlOpaqueRPr,
	opaqueType:    TypeSMLRunProps,
	opaqueSubType: SubTypeSMLRunProps,
	attrKey:       AttrSMLRPr,
}

// smlRPrChildName maps a named formatting type to the <rPr> child that declares
// it in SpreadsheetML (ECMA-376 Part 1 §18.4.7).
func smlRPrChildName(spanType string) string {
	switch spanType {
	case TypeBold:
		return "b"
	case TypeItalic:
		return "i"
	case TypeUnderline:
		return "u"
	case TypeStrikethrough:
		return "strike"
	case TypeSuperscript, TypeSubscript:
		return "vertAlign"
	}
	return ""
}

// dmlRunPropsProjection is the DrawingML binding: the whole <a:rPr> travels on
// the opaque code, and the named codes carry nothing of their own because the
// attributes that declare them live inside those same bytes.
var dmlRunPropsProjection = runPropsProjection{
	namedXML:      func(runProps, string) string { return "" },
	opaqueXML:     func(rp runProps) string { return rp.dmlRPr },
	opaqueType:    TypeDMLRunProps,
	opaqueSubType: SubTypeDMLRunProps,
	attrKey:       AttrDMLRPr,
}
