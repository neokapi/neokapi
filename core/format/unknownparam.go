package format

import (
	"fmt"

	fschema "github.com/neokapi/neokapi/core/format/schema"
	coreschema "github.com/neokapi/neokapi/core/schema"
)

// UnknownParameterError is the error a format config's ApplyMap returns for
// a key it does not take. Format config keys are snake_case, so a camelCase
// spelling of a key the format takes is named with the key to write instead:
// `json: extractAllPairs is now extract_all_pairs`. Any other key is reported
// as unknown.
func UnknownParameterError(cfg any, formatName, key string) error {
	if err := RenamedParameterError(cfg, formatName, key); err != nil {
		return err
	}
	return fmt.Errorf("%s: unknown parameter: %s", formatName, key)
}

// RenamedParameterError returns the `is now` error for key when key is a
// camelCase spelling of one of the format's snake_case keys, and nil
// otherwise. A config without a schema takes the snake_case spelling of any
// camelCase key as the one it would have accepted. A format whose ApplyMap
// ignores unknown keys calls it from its default case, so a recipe that
// still writes a renamed key fails instead of reading under the defaults.
func RenamedParameterError(cfg any, formatName, key string) error {
	known := func(string) bool { return true }
	if s, ok := cfg.(interface{ Schema() *fschema.FormatSchema }); ok {
		if fs := s.Schema(); fs != nil && len(fs.Properties) > 0 {
			known = func(k string) bool { _, ok := fs.Properties[k]; return ok }
		}
	}
	if snake, ok := coreschema.RenamedKey(key, known); ok {
		return fmt.Errorf("%s: %s is now %s", formatName, key, snake)
	}
	return nil
}
