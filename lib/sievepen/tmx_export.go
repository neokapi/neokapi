package sievepen

import (
	"encoding/xml"
	"fmt"
	"io"

	"github.com/gokapi/gokapi/core/model"
)

// EntryProvider is implemented by TM backends that can list all entries.
type EntryProvider interface {
	Entries() []TMEntry
}

// ExportTMX writes translation memory entries to TMX format.
// The TM must implement EntryProvider (both InMemoryTM and SQLiteTM do).
func ExportTMX(tm TranslationMemory, writer io.Writer, sourceLocale, targetLocale model.LocaleID) error {
	provider, ok := tm.(EntryProvider)
	if !ok {
		return fmt.Errorf("TM does not support entry listing")
	}

	var entries []TMEntry
	for _, e := range provider.Entries() {
		if e.SourceLocale == sourceLocale && e.TargetLocale == targetLocale {
			entries = append(entries, e)
		}
	}

	doc := tmxDocument{
		Header: tmxHeader{
			CreationTool:        "gokapi-sievepen",
			CreationToolVersion: "2.0",
			SegType:             "sentence",
			AdminLang:           string(sourceLocale),
			SrcLang:             string(sourceLocale),
			DataType:            "plaintext",
		},
		Body: tmxBody{
			TUs: make([]tmxTU, 0, len(entries)),
		},
	}

	for _, entry := range entries {
		tu := tmxTU{
			TUID: entry.ID,
			TUVs: []tmxTUV{
				{
					Lang: string(entry.SourceLocale),
					Seg:  entry.SourceText(),
				},
				{
					Lang: string(entry.TargetLocale),
					Seg:  entry.TargetText(),
				},
			},
		}

		if !entry.CreatedAt.IsZero() {
			tu.CreatedAt = entry.CreatedAt.UTC().Format("20060102T150405Z")
		}
		if !entry.UpdatedAt.IsZero() {
			tu.ChangedAt = entry.UpdatedAt.UTC().Format("20060102T150405Z")
		}

		for k, v := range entry.Properties {
			tu.Properties = append(tu.Properties, tmxProp{Type: k, Value: v})
		}

		// Export entity mappings as TMX properties.
		for _, em := range entry.Entities {
			tu.Properties = append(tu.Properties, tmxProp{
				Type:  fmt.Sprintf("entity:%s", em.PlaceholderID),
				Value: fmt.Sprintf("%s:%s", em.Type, em.SourceValue),
			})
		}

		doc.Body.TUs = append(doc.Body.TUs, tu)
	}

	if _, err := fmt.Fprint(writer, xml.Header); err != nil {
		return fmt.Errorf("failed to write XML header: %w", err)
	}

	encoder := xml.NewEncoder(writer)
	encoder.Indent("", "  ")
	if err := encoder.Encode(doc); err != nil {
		return fmt.Errorf("failed to encode TMX: %w", err)
	}

	return nil
}
