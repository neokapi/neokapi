package properties

import (
	coreschema "github.com/neokapi/neokapi/core/schema"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the Java Properties format's parameters.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "Java Properties Format",
		Description: "Configuration for the Java .properties format reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "properties",
			Extensions: []string{".properties"},
			MimeTypes:  []string{"text/x-java-properties"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "parsing",
				Label: "Parsing",
				Fields: []string{
					"separator",
				},
			},
			{
				ID:    "extraction",
				Label: "Extraction",
				Fields: []string{
					"use_key_condition", "extract_only_matching_key", "key_condition",
					"extra_comments", "extract_non_translatable_content",
				},
			},
			{
				ID:    "notes",
				Label: "Notes",
				Fields: []string{
					"comments_are_notes",
				},
			},
			{
				ID:    "output",
				Label: "Output",
				Fields: []string{
					"use_java_escapes", "escape_extended_chars",
				},
			},
			{
				ID:    "inlineCodes",
				Label: "Inline Codes",
				Fields: []string{
					"use_code_finder", "code_finder_rules",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"separator": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Default:     "=",
				Title:       "Separator",
				Description: "Key-value separator character (typically '=' or ':')",
			}),
			"use_key_condition": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Enable Key Condition",
				Description: "Filter entries for extraction based on a regex condition applied to their keys",
			}),
			"extract_only_matching_key": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Extract Only Matching Keys",
				Description: "When enabled, extract only entries whose keys match the condition; otherwise exclude matching keys",
				Visible:     &coreschema.ConditionExpr{Field: "use_key_condition", Eq: true},
			}),
			"key_condition": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Default:     ".*text.*",
				Title:       "Key Condition Pattern",
				Description: "Regular expression pattern to test against property keys for extraction filtering",
				Widget:      "regex",
				Visible:     &coreschema.ConditionExpr{Field: "use_key_condition", Eq: true},
			}),
			"extra_comments": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Additional Comment Markers",
				Description: "Recognize semicolon and double-slash comment styles in addition to standard # and ! markers",
			}),
			"extract_non_translatable_content": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Surface Excluded Values",
				Description: "Surface the value text of excluded entries (keys filtered out, or under #_skip / #_bskip) as non-translatable content blocks visible to ingestion but skipped by translation",
			}),
			"comments_are_notes": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Comments as Notes",
				Description: "Treat comments as translator notes attached to the following entry",
			}),
			"use_java_escapes": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Retain Java Escapes",
				Description: "Keep Java property escape sequences (\\: \\= \\# \\!) in extracted text instead of resolving them",
			}),
			"escape_extended_chars": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Escape Extended Characters",
				Description: "Escape extended Unicode characters (non-ASCII) in output using \\uXXXX notation",
			}),
			"use_code_finder": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Enable Inline Code Detection",
				Description: "Enable regex-based inline code detection within property values",
			}),
			"code_finder_rules": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Code Finder Rules",
				Description: "Regex patterns that match inline codes within translatable text",
				Widget:      "code-finder",
				Visible:     &coreschema.ConditionExpr{Field: "use_code_finder", Eq: true},
			}),
		},
	}
}
