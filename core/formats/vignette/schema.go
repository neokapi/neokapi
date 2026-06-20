package vignette

import (
	coreschema "github.com/neokapi/neokapi/core/schema"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the Vignette format's parameters.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "Vignette CMS Export Format",
		Description: "Configuration for the Vignette CMS export/import XML format reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "vignette",
			Extensions: []string{".xml"},
			MimeTypes:  []string{"text/xml"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Content Extraction",
				Fields: []string{
					"partsNames", "partsConfigurations",
					"sourceId", "localeId", "monolingual",
					"extractNonTranslatableContent",
				},
			},
			{
				ID:    "output",
				Label: "Output",
				Fields: []string{
					"useCDATA",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"partsNames": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "Parts Names",
				Description: "Comma-separated list of <attribute name=\"…\"> values to extract from each importContentInstance.",
			}),
			"partsConfigurations": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "Parts Configurations",
				Description: "Comma-separated list of sub-filter configuration ids (one per entry in partsNames). Use 'default' for no sub-filtering; 'okf_html' decodes HTML entities and strips an outer <p> wrap.",
			}),
			"sourceId": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Default:     DefaultSourceID,
				Title:       "Source ID Attribute",
				Description: "The name attribute value of the <attribute> that links source and target instances.",
			}),
			"localeId": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Default:     DefaultLocaleID,
				Title:       "Locale ID Attribute",
				Description: "The name attribute value of the <attribute> that holds the locale identifier.",
			}),
			"monolingual": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Monolingual Mode",
				Description: "If true, disable source/target pairing and extract every importContentInstance independently.",
			}),
			"extractNonTranslatableContent": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Extract non-translatable content",
				Description: "If true (default), non-source-locale content instances that bilingual mode does not extract are surfaced as non-translatable content blocks (visible to ingestion/LLM consumers, skipped by machine translation) instead of being hidden in skeleton. Disable to keep them in skeleton. No effect in monolingual mode.",
			}),
			"useCDATA": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Use CDATA",
				Description: "If true (default), wrap written valueCLOB payloads in <![CDATA[…]]> sections on output.",
			}),
		},
	}
}
