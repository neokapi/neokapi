package sievepen

import (
	"encoding/xml"
	"fmt"
	"io"
	"time"

	"github.com/neokapi/neokapi/core/model"
)

// TMX XML structures for parsing.
type tmxDocument struct {
	XMLName xml.Name  `xml:"tmx"`
	Header  tmxHeader `xml:"header"`
	Body    tmxBody   `xml:"body"`
}

type tmxHeader struct {
	CreationTool        string `xml:"creationtool,attr"`
	CreationToolVersion string `xml:"creationtoolversion,attr"`
	SegType             string `xml:"segtype,attr"`
	AdminLang           string `xml:"adminlang,attr"`
	SrcLang             string `xml:"srclang,attr"`
	DataType            string `xml:"datatype,attr"`
}

type tmxBody struct {
	TUs []tmxTU `xml:"tu"`
}

type tmxTU struct {
	TUID       string    `xml:"tuid,attr"`
	SrcLang    string    `xml:"srclang,attr"`
	CreatedAt  string    `xml:"creationdate,attr"`
	ChangedAt  string    `xml:"changedate,attr"`
	Properties []tmxProp `xml:"prop"`
	TUVs       []tmxTUV  `xml:"tuv"`
}

type tmxProp struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

type tmxTUV struct {
	Lang string `xml:"http://www.w3.org/XML/1998/namespace lang,attr"`
	Seg  string `xml:"seg"`
}

// ImportTMX reads a TMX file and imports matching translation units into the TM.
// Plain text TMX segments are stored as plain Fragments (no spans/entities).
// They participate in plain matching only. Over time, as content is
// re-processed through the entity-annotate pipeline, entries can be enriched.
func ImportTMX(tm TranslationMemory, reader io.Reader, sourceLocale, targetLocale model.LocaleID) (int, error) {
	var doc tmxDocument
	decoder := xml.NewDecoder(reader)
	if err := decoder.Decode(&doc); err != nil {
		return 0, fmt.Errorf("failed to parse TMX: %w", err)
	}

	imported := 0
	for i, tu := range doc.Body.TUs {
		var sourceText, targetText string
		var foundSource, foundTarget bool

		for _, tuv := range tu.TUVs {
			lang := model.LocaleID(tuv.Lang)
			if lang == sourceLocale {
				sourceText = tuv.Seg
				foundSource = true
			}
			if lang == targetLocale {
				targetText = tuv.Seg
				foundTarget = true
			}
		}

		if !foundSource || !foundTarget {
			continue
		}

		id := tu.TUID
		if id == "" {
			id = fmt.Sprintf("tu-%d", i+1)
		}

		createdAt := parseTime(tu.CreatedAt)
		updatedAt := parseTime(tu.ChangedAt)
		if updatedAt.IsZero() {
			updatedAt = createdAt
		}

		props := make(map[string]string)
		for _, p := range tu.Properties {
			props[p.Type] = p.Value
		}

		entry := TMEntry{
			ID:           id,
			Source:       model.NewFragment(sourceText),
			Target:       model.NewFragment(targetText),
			SourceLocale: sourceLocale,
			TargetLocale: targetLocale,
			CreatedAt:    createdAt,
			UpdatedAt:    updatedAt,
			Properties:   props,
		}

		if err := tm.Add(entry); err != nil {
			return imported, fmt.Errorf("failed to add entry %s: %w", id, err)
		}
		imported++
	}

	return imported, nil
}

// parseTime attempts to parse a TMX date string (YYYYMMDDTHHmmssZ).
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse("20060102T150405Z", s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, s)
		if err != nil {
			return time.Time{}
		}
	}
	return t
}
