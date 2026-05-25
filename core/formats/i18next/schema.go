package i18next

import (
	coreschema "github.com/neokapi/neokapi/core/schema"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the i18next format's parameters.
// The format is convention-driven, so it exposes only a few behavioural toggles
// on top of the generic JSON reader it delegates to.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "i18next JSON",
		Description: "Configuration for the i18next / react-i18next JSON localization format",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID: formatID,
			// No extension or MIME is claimed: i18next files use the generic
			// .json extension and application/json MIME owned by the json
			// format, so this format is selected explicitly (-f i18next).
			Extensions: []string{},
			MimeTypes:  []string{},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "i18next",
				Label: "i18next",
				Fields: []string{
					"protectInterpolation", "subfilterHtmlValues", "legacyPluralForms",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"protectInterpolation": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Protect interpolation and nesting",
				Default:     true,
				Description: "Detect i18next interpolation ({{var}}, {{var, format}}) and nesting ($t(key)) and protect them as inline codes so they are never translated.",
			}),
			"subfilterHtmlValues": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Subfilter HTML values (_html keys)",
				Default:     false,
				Description: "Hand values whose key ends in \"_html\" (the i18next convention for markup) to the HTML subfilter so tags are protected and text remains translatable. Off by default: the HTML subfilter is not byte-faithful for bare markup fragments.",
			}),
			"legacyPluralForms": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Recognise legacy plural forms",
				Default:     true,
				Description: "Recognise the legacy v1–v3 plural sibling keys (key_plural and the numeric key_0 / key_1 / key_2 … forms) in addition to the v4 CLDR suffixes (_zero / _one / _two / _few / _many / _other).",
			}),
		},
	}
}
