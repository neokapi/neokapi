package messageformat

import (
	"github.com/neokapi/neokapi/core/format/schema"
	coreschema "github.com/neokapi/neokapi/core/schema"
)

// Schema returns the JSON Schema metadata for the ICU MessageFormat format's
// parameters.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "ICU MessageFormat Filter",
		Description: "ICU MessageFormat patterns, one per input line",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "messageformat",
			Extensions: []string{".mf", ".messageformat"},
			MimeTypes:  []string{"text/x-messageformat"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Content Extraction",
				Fields: []string{
					"extractNonTranslatableContent",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"extractNonTranslatableContent": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Extract non-translatable content",
				Description: "If true (default), literal prose that frames a plural/select branch (the sentence frame around a picker) is surfaced as a non-translatable content block (visible to ingestion/LLM consumers, skipped by machine translation) instead of being hidden in skeleton. Disable to keep it in skeleton.",
			}),
		},
	}
}
