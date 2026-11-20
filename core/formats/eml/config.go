package eml

import "github.com/neokapi/neokapi/core/format"

const (
	name        = "eml"
	displayName = "Email message (EML)"
	mediaType   = "message/rfc822"
)

type Config struct{}

func (*Config) FormatName() string { return name }
func (*Config) Reset()             {}
func (*Config) Validate() error    { return nil }
func (c *Config) ApplyMap(values map[string]any) error {
	for key := range values {
		return format.UnknownParameterError(c, name, key)
	}
	return nil
}

func Signature() format.FormatSignature {
	return format.FormatSignature{MIMETypes: []string{mediaType}, Extensions: []string{".eml"}}
}
