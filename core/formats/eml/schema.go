package eml

import "github.com/neokapi/neokapi/core/format/schema"

func (*Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title: displayName, Description: "Extract the subject and plain-text MIME bodies from an email message.", Type: "object",
		FormatMeta: schema.FormatMeta{ID: name, Extensions: []string{".eml"}, MimeTypes: []string{mediaType}},
	}
}
