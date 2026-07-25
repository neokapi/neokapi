package tools

import (
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
)

// applyStepConfig applies a flow step's `config:` map onto a config already at
// its documented defaults, then pins the run's target language.
//
// It is the one place the two rules every config factory has to obey live:
//
//   - **The step's map is applied over the defaults**, not instead of them, so a
//     step that mentions one knob keeps the documented value of the rest.
//     schema.ApplyConfig is a json.Unmarshal, so the keys are the config
//     struct's JSON names — camelCase, and an unknown key is silently ignored.
//   - **The run's target language wins** over a locale spelled in the step
//     config: one flow has to serve every locale it is run for, so a recipe
//     cannot pin a step to `en` and quietly make `--target-lang nb` a no-op.
//     setLocale is nil for a monolingual tool, which has no locale to pin.
//
// Eleven factories had hand-rolled copies of these six lines. Sharing them
// keeps a fix (or a mistake) in one place — #1478 found one such copy already
// drifting from its original.
func applyStepConfig[C any](toolName string, config map[string]any, cfg *C, targetLang string, setLocale func(*C, model.LocaleID)) error {
	if err := schema.ApplyConfig(config, cfg); err != nil {
		return fmt.Errorf("%s config: %w", toolName, err)
	}
	if setLocale != nil && targetLang != "" {
		setLocale(cfg, model.LocaleID(targetLang))
	}
	return nil
}
