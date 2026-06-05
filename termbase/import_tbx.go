package termbase

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// TBXImportOptions controls how a TBX (TermBase eXchange) document is interpreted.
type TBXImportOptions struct {
	// DefaultStatus is applied to terms that carry no recognizable status note.
	// When empty, model.TermApproved is used.
	DefaultStatus model.TermStatus
	// Domain assigns a fallback domain to concepts that do not declare a
	// subjectField descrip of their own.
	Domain string
	// Source sets the TermSource for imported concepts ("terminology" or
	// "brand_vocabulary"). Empty leaves it unset.
	Source TermSource
	// IDPrefix is used to generate concept IDs when an entry has no id attribute.
	IDPrefix string
}

// --- Permissive XML structures covering both TBX dialects ---------------------
//
// TBX-Basic / TBX v3:  <tbx><text><body><conceptEntry><langSec><termSec><term>
// Legacy MARTIF/2008:  <martif><text><body><termEntry><langSet><tig|ntig><term>
//
// The structs below tolerate either root and either entry/lang/term nesting,
// so a single decode pass handles both dialects.

type tbxDocument struct {
	XMLName xml.Name    `xml:"tbx"`
	Text    tbxTextBody `xml:"text"`
}

type tbxMartif struct {
	XMLName xml.Name    `xml:"martif"`
	Text    tbxTextBody `xml:"text"`
}

type tbxTextBody struct {
	Body tbxBody `xml:"body"`
}

type tbxBody struct {
	// conceptEntry (TBX v3) and termEntry (MARTIF) are both collected.
	ConceptEntries []tbxEntry `xml:"conceptEntry"`
	TermEntries    []tbxEntry `xml:"termEntry"`
}

// tbxEntry models both <conceptEntry> and <termEntry>.
type tbxEntry struct {
	ID string `xml:"id,attr"`
	// concept-level descriptions: <descrip type="definition">, type="subjectField"
	Descrips    []tbxDescrip    `xml:"descrip"`
	DescripGrps []tbxDescripGrp `xml:"descripGrp"`
	// langSec (TBX v3) and langSet (MARTIF) — both captured.
	LangSecs []tbxLangSec `xml:"langSec"`
	LangSets []tbxLangSec `xml:"langSet"`
}

// tbxDescripGrp wraps a descrip in MARTIF concept entries.
type tbxDescripGrp struct {
	Descrips []tbxDescrip `xml:"descrip"`
}

type tbxDescrip struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

// tbxLangSec models both <langSec> and <langSet>.
type tbxLangSec struct {
	Lang string `xml:"lang,attr"` // xml:lang attribute
	// termSec (TBX v3); tig / ntig (MARTIF). All captured.
	TermSecs []tbxTermSec `xml:"termSec"`
	Tigs     []tbxTermSec `xml:"tig"`
	Ntigs    []tbxTermSec `xml:"ntig"`
}

// tbxTermSec models <termSec>, <tig> and <ntig>.
type tbxTermSec struct {
	Term      string        `xml:"term"`
	TermNotes []tbxTermNote `xml:"termNote"`
	// MARTIF ntig nests term + notes inside a <termGrp>.
	TermGrp *tbxTermGrp `xml:"termGrp"`
	// MARTIF descripGrp can also carry termNotes at this level.
	Descrips []tbxDescrip `xml:"descrip"`
}

type tbxTermGrp struct {
	Term      string        `xml:"term"`
	TermNotes []tbxTermNote `xml:"termNote"`
}

type tbxTermNote struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

// ImportTBX reads a TBX (ISO 30042) document and imports its concepts.
// It detects the dialect from the root element (<tbx> for TBX-Basic/v3,
// <martif> for legacy TBX 2008/MARTIF) and maps each conceptEntry/termEntry
// to a termbase Concept. Returns the number of concepts imported.
func ImportTBX(ctx context.Context, tb TermBase, reader io.Reader, opts TBXImportOptions) (int, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return 0, fmt.Errorf("read TBX: %w", err)
	}

	entries, err := decodeTBX(data)
	if err != nil {
		return 0, err
	}

	defaultStatus := opts.DefaultStatus
	if defaultStatus == "" {
		defaultStatus = model.TermApproved
	}

	prefix := opts.IDPrefix
	if prefix == "" {
		prefix = "tbx"
	}

	imported := 0
	for _, entry := range entries {
		concept := Concept{
			ID:     entry.ID,
			Domain: opts.Domain,
			Source: opts.Source,
		}
		if concept.ID == "" {
			concept.ID = fmt.Sprintf("%s-%d", prefix, imported+1)
		}

		// Concept-level descriptions (definition, subjectField).
		for _, d := range collectConceptDescrips(entry) {
			switch strings.ToLower(strings.TrimSpace(d.Type)) {
			case "definition":
				if v := strings.TrimSpace(d.Value); v != "" {
					concept.Definition = v
				}
			case "subjectfield":
				if v := strings.TrimSpace(d.Value); v != "" {
					concept.Domain = v
				}
			}
		}

		for _, lang := range append(append([]tbxLangSec{}, entry.LangSecs...), entry.LangSets...) {
			locale := model.LocaleID(strings.TrimSpace(lang.Lang))
			for _, ts := range collectTermSecs(lang) {
				term := buildTerm(ts, locale, defaultStatus)
				if term.Text == "" {
					continue
				}
				concept.Terms = append(concept.Terms, term)
			}
		}

		if len(concept.Terms) == 0 {
			// Skip entries that yielded no usable terms.
			continue
		}

		if err := tb.AddConcept(ctx, concept); err != nil {
			return imported, fmt.Errorf("add concept %s: %w", concept.ID, err)
		}
		imported++
	}

	return imported, nil
}

// decodeTBX parses the document under either supported root element.
func decodeTBX(data []byte) ([]tbxEntry, error) {
	root := detectRoot(data)
	switch root {
	case "tbx":
		var doc tbxDocument
		if err := xml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parse TBX document: %w", err)
		}
		return append(append([]tbxEntry{}, doc.Text.Body.ConceptEntries...), doc.Text.Body.TermEntries...), nil
	case "martif":
		var doc tbxMartif
		if err := xml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parse MARTIF document: %w", err)
		}
		return append(append([]tbxEntry{}, doc.Text.Body.TermEntries...), doc.Text.Body.ConceptEntries...), nil
	default:
		return nil, fmt.Errorf("unrecognized TBX root element %q (expected <tbx> or <martif>)", root)
	}
}

// detectRoot returns the local name of the first start element, validating the
// XML is at least well-formed up to the root.
func detectRoot(data []byte) string {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		if se, ok := tok.(xml.StartElement); ok {
			return se.Name.Local
		}
	}
}

// collectConceptDescrips gathers concept-level descrips, flattening any
// descripGrp wrappers used by MARTIF.
func collectConceptDescrips(entry tbxEntry) []tbxDescrip {
	descrips := append([]tbxDescrip{}, entry.Descrips...)
	for _, grp := range entry.DescripGrps {
		descrips = append(descrips, grp.Descrips...)
	}
	return descrips
}

// collectTermSecs gathers all term sections in a language section across the
// TBX v3 (termSec) and MARTIF (tig/ntig) spellings.
func collectTermSecs(lang tbxLangSec) []tbxTermSec {
	secs := append([]tbxTermSec{}, lang.TermSecs...)
	secs = append(secs, lang.Tigs...)
	secs = append(secs, lang.Ntigs...)
	return secs
}

// buildTerm converts a parsed term section into a termbase Term.
func buildTerm(ts tbxTermSec, locale model.LocaleID, defaultStatus model.TermStatus) Term {
	text := strings.TrimSpace(ts.Term)
	notes := ts.TermNotes
	// MARTIF ntig nests the term and notes within a termGrp.
	if ts.TermGrp != nil {
		if text == "" {
			text = strings.TrimSpace(ts.TermGrp.Term)
		}
		notes = append(notes, ts.TermGrp.TermNotes...)
	}

	term := Term{
		Text:   text,
		Locale: locale,
		Status: defaultStatus,
	}

	for _, n := range notes {
		val := strings.TrimSpace(n.Value)
		if val == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(n.Type)) {
		case "partofspeech", "grammaticalcategory":
			term.PartOfSpeech = val
		case "grammaticalgender", "gender":
			term.Gender = val
		case "administrativestatus", "normativeauthorization", "status":
			if s := parseTBXStatus(val); s != "" {
				term.Status = s
			}
		case "usagenote", "note", "geographicalusage":
			if term.Note == "" {
				term.Note = val
			}
		}
	}

	return term
}

// parseTBXStatus maps TBX administrativeStatus / normativeAuthorization values
// (and a few plain status spellings) onto the termbase status vocabulary.
func parseTBXStatus(s string) model.TermStatus {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "preferredterm-admn-sts", "preferred", "preferredterm", "standardizedterm":
		return model.TermPreferred
	case "admittedterm-admn-sts", "admitted", "admittedterm":
		return model.TermAdmitted
	case "approvedterm-admn-sts", "approved", "approvedterm":
		return model.TermApproved
	case "deprecatedterm-admn-sts", "deprecated", "deprecatedterm":
		return model.TermDeprecated
	case "supersededterm-admn-sts", "superseded", "supersededterm":
		// No distinct "superseded" status in the termbase vocabulary;
		// the closest semantic match is "deprecated".
		return model.TermDeprecated
	case "forbiddenterm-admn-sts", "forbidden", "forbiddenterm", "prohibited":
		return model.TermForbidden
	case "proposedterm-admn-sts", "proposed", "proposedterm":
		return model.TermProposed
	default:
		return ""
	}
}
