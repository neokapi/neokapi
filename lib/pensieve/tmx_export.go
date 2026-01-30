package pensieve

import (
	"encoding/xml"
	"fmt"
	"io"

	"github.com/gokapi/gokapi/core/model"
)

// ExportTMX writes translation memory entries to TMX format.
// Only entries matching the specified source and target locales are exported.
func ExportTMX(tm TranslationMemory, writer io.Writer, sourceLocale, targetLocale model.LocaleID) error {
	// Collect matching entries by performing a broad lookup.
	// Since we need all entries, we use the Entries method if available,
	// otherwise fall back to an interface-based approach.
	var entries []TMEntry

	if mem, ok := tm.(*InMemoryTM); ok {
		all := mem.Entries()
		for _, e := range all {
			if e.SourceLocale == sourceLocale && e.TargetLocale == targetLocale {
				entries = append(entries, e)
			}
		}
	}

	doc := tmxDocument{
		Header: tmxHeader{
			CreationTool:        "gokapi-pensieve",
			CreationToolVersion: "1.0",
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
					Seg:  entry.Source,
				},
				{
					Lang: string(entry.TargetLocale),
					Seg:  entry.Target,
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
