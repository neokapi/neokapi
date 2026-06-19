package image

import "fmt"

// Config is the image-format configuration. An image is, first and foremost, a
// localizable Media asset (emitted regardless of these toggles); OCR and layout
// are opt-in enrichment.
type Config struct {
	// OCR enables in-image text recognition via the kapi-vision plugin. When
	// false (or the plugin is absent), the image is emitted as a Media asset only
	// — the right mode for whole-image localization flows that replace the
	// picture rather than its text.
	OCR bool
	// Layout enables ML layout detection (regions + reading order) when OCR runs;
	// when false, structure falls back to geometric inference (tier 2).
	Layout bool
}

func defaultConfig() *Config { return &Config{OCR: true, Layout: true} }

func (c *Config) FormatName() string { return "image" }
func (c *Config) Reset()             { *c = Config{OCR: true, Layout: true} }
func (c *Config) Validate() error    { return nil }
func (c *Config) ApplyMap(values map[string]any) error {
	for key, v := range values {
		b, ok := v.(bool)
		if !ok {
			return fmt.Errorf("image: parameter %q must be a boolean", key)
		}
		switch key {
		case "ocr":
			c.OCR = b
		case "layout":
			c.Layout = b
		default:
			return fmt.Errorf("image: unknown parameter: %s", key)
		}
	}
	return nil
}
