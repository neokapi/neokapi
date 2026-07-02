package plaintext

import (
	"github.com/neokapi/neokapi/core/format/schema"
	coreschema "github.com/neokapi/neokapi/core/schema"
)

// Schema returns the JSON Schema metadata for the Plain Text format's parameters.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "Plain Text Format",
		Description: "Configuration for the plain text format reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "plaintext",
			Extensions: []string{".txt"},
			MimeTypes:  []string{"text/plain"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "segmentation",
				Label: "Segmentation",
				Fields: []string{
					"segmentByLine", "paragraphs", "spliceLines", "spliceMarker",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"segmentByLine": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Description: "If true, each line becomes a separate block. If false, paragraphs (separated by blank lines) are blocks.",
			}),
			"paragraphs": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Paragraphs",
				Description: "If true, blank-line-separated paragraphs become blocks instead of individual lines.",
			}),
			"spliceLines": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Splice Lines",
				Description: "If true, lines ending with the splice marker are joined with the next line into a single block.",
			}),
			"spliceMarker": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Default:     `\`,
				Title:       "Splice Marker",
				Description: "Continuation marker recognized at the end of a line in splice mode.",
				Visible:     &coreschema.ConditionExpr{Field: "spliceLines", Eq: true},
			}),
		},
	}
}
