package termbase

import (
	"encoding/xml"
	"fmt"
	"io"

	"github.com/neokapi/neokapi/core/model"
)

// TBXExportOptions controls TBX serialization.
type TBXExportOptions struct {
	// SourceLocale, when set, restricts exported concepts to those that have a
	// term in this locale. Empty exports every concept and all its locales.
	SourceLocale model.LocaleID
}

// --- TBX-Basic v3 output structures -------------------------------------------

type tbxOut struct {
	XMLName xml.Name      `xml:"tbx"`
	Style   string        `xml:"style,attr"`
	Type    string        `xml:"type,attr"`
	Lang    string        `xml:"xml:lang,attr"`
	Text    tbxOutText    `xml:"text"`
	Header  *tbxOutHeader `xml:"tbxHeader"`
}

type tbxOutHeader struct {
	FileDesc tbxOutFileDesc `xml:"fileDesc"`
}

type tbxOutFileDesc struct {
	SourceDesc tbxOutSourceDesc `xml:"sourceDesc"`
}

type tbxOutSourceDesc struct {
	P string `xml:"p"`
}

type tbxOutText struct {
	Body tbxOutBody `xml:"body"`
}

type tbxOutBody struct {
	ConceptEntries []tbxOutConcept `xml:"conceptEntry"`
}

type tbxOutConcept struct {
	ID       string          `xml:"id,attr"`
	Descrips []tbxOutDescrip `xml:"descrip"`
	LangSecs []tbxOutLangSec `xml:"langSec"`
}

type tbxOutDescrip struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

type tbxOutLangSec struct {
	Lang     string          `xml:"xml:lang,attr"`
	TermSecs []tbxOutTermSec `xml:"termSec"`
}

type tbxOutTermSec struct {
	Term      string           `xml:"term"`
	TermNotes []tbxOutTermNote `xml:"termNote"`
}

type tbxOutTermNote struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

// ExportTBX writes all concepts as a TBX-Basic v3 document (root <tbx>).
// The output round-trips through ImportTBX to yield equivalent concepts.
func ExportTBX(tb TermBase, writer io.Writer, opts TBXExportOptions) error {
	doc := tbxOut{
		Style: "dca",
		Type:  "TBX-Basic",
		Lang:  "en",
		Header: &tbxOutHeader{
			FileDesc: tbxOutFileDesc{
				SourceDesc: tbxOutSourceDesc{P: "Exported by neokapi termbase"},
			},
		},
	}

	for _, concept := range tb.Concepts() {
		if !opts.SourceLocale.IsEmpty() {
			if concept.SourceTerm(opts.SourceLocale) == nil {
				continue
			}
		}

		out := tbxOutConcept{ID: concept.ID}
		if concept.Definition != "" {
			out.Descrips = append(out.Descrips, tbxOutDescrip{Type: "definition", Value: concept.Definition})
		}
		if concept.Domain != "" {
			out.Descrips = append(out.Descrips, tbxOutDescrip{Type: "subjectField", Value: concept.Domain})
		}

		// Group terms by locale, preserving first-seen locale order.
		var localeOrder []model.LocaleID
		byLocale := make(map[model.LocaleID][]Term)
		for _, t := range concept.Terms {
			if _, seen := byLocale[t.Locale]; !seen {
				localeOrder = append(localeOrder, t.Locale)
			}
			byLocale[t.Locale] = append(byLocale[t.Locale], t)
		}

		for _, locale := range localeOrder {
			langSec := tbxOutLangSec{Lang: string(locale)}
			for _, t := range byLocale[locale] {
				ts := tbxOutTermSec{Term: t.Text}
				if t.PartOfSpeech != "" {
					ts.TermNotes = append(ts.TermNotes, tbxOutTermNote{
						Type:  "partOfSpeech",
						Value: t.PartOfSpeech,
					})
				}
				if t.Gender != "" {
					ts.TermNotes = append(ts.TermNotes, tbxOutTermNote{
						Type:  "grammaticalGender",
						Value: t.Gender,
					})
				}
				if statusVal := tbxStatusValue(t.Status); statusVal != "" {
					ts.TermNotes = append(ts.TermNotes, tbxOutTermNote{
						Type:  "administrativeStatus",
						Value: statusVal,
					})
				}
				if t.Note != "" {
					ts.TermNotes = append(ts.TermNotes, tbxOutTermNote{
						Type:  "usageNote",
						Value: t.Note,
					})
				}
				langSec.TermSecs = append(langSec.TermSecs, ts)
			}
			out.LangSecs = append(out.LangSecs, langSec)
		}

		doc.Text.Body.ConceptEntries = append(doc.Text.Body.ConceptEntries, out)
	}

	if _, err := io.WriteString(writer, xml.Header); err != nil {
		return fmt.Errorf("write TBX header: %w", err)
	}

	enc := xml.NewEncoder(writer)
	enc.Indent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return fmt.Errorf("encode TBX: %w", err)
	}
	if err := enc.Flush(); err != nil {
		return fmt.Errorf("flush TBX: %w", err)
	}
	if _, err := io.WriteString(writer, "\n"); err != nil {
		return fmt.Errorf("write TBX trailing newline: %w", err)
	}
	return nil
}

// tbxStatusValue maps the termbase status vocabulary onto administrativeStatus
// values used on export. The values form a clean inverse of parseTBXStatus so
// every status round-trips back to itself: preferredTerm-admn-sts and
// admittedTerm-admn-sts are the TBX-Basic picklist names; the others extend the
// same naming scheme to keep the six termbase statuses distinct.
func tbxStatusValue(s model.TermStatus) string {
	switch s {
	case model.TermPreferred:
		return "preferredTerm-admn-sts"
	case model.TermAdmitted:
		return "admittedTerm-admn-sts"
	case model.TermApproved:
		return "approvedTerm-admn-sts"
	case model.TermDeprecated:
		return "deprecatedTerm-admn-sts"
	case model.TermForbidden:
		return "forbiddenTerm-admn-sts"
	case model.TermProposed:
		return "proposedTerm-admn-sts"
	default:
		return ""
	}
}
