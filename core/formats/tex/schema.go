package tex

import (
	coreschema "github.com/neokapi/neokapi/core/schema"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the TeX/LaTeX format's parameters.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "TeX/LaTeX Format",
		Description: "Configuration for the TeX/LaTeX format reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "tex",
			Extensions: []string{".tex", ".latex"},
			MimeTypes:  []string{"application/x-tex", "text/x-tex"},
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
				Description: "If true (default), non-translatable contextual content such as verbatim/lstlisting code and math is surfaced as content blocks (visible to ingestion/LLM consumers, skipped by machine translation) instead of being hidden in skeleton. Disable to keep it opaque.",
			}),
		},
	}
}
