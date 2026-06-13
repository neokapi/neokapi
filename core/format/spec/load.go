package spec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Load reads and validates a spec file. The returned Spec carries the
// directory of its source file in unexported state so Example.InputFile
// resolves against it.
func Load(path string) (*Spec, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("spec: resolve path: %w", err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("spec: read %s: %w", path, err)
	}
	var s Spec
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("spec: parse %s: %w", path, err)
	}
	s.dir = filepath.Dir(abs)
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("spec %s: %w", path, err)
	}
	return &s, nil
}

// Validate enforces the structural invariants spec authors are most
// likely to break: duplicate ids, AppliesTo referencing unknown
// variants, examples with no input, etc.
func (s *Spec) Validate() error {
	if s.Format == "" {
		return errors.New("format is required")
	}
	switch s.Kind {
	case "", KindTopLevel, KindSubfilter:
	default:
		return fmt.Errorf("kind %q: must be %q or %q", s.Kind, KindTopLevel, KindSubfilter)
	}
	if len(s.Features) == 0 {
		return errors.New("at least one feature is required")
	}
	variants := map[string]bool{}
	for _, v := range s.Variants {
		if v.ID == "" {
			return errors.New("variant has empty id")
		}
		if variants[v.ID] {
			return fmt.Errorf("variant %q declared twice", v.ID)
		}
		variants[v.ID] = true
	}
	checkApplies := func(scope, id string, applies []string) error {
		for _, a := range applies {
			if !variants[a] {
				return fmt.Errorf("%s %q applies_to: unknown variant %q", scope, id, a)
			}
		}
		return nil
	}
	cfgKeys := map[string]bool{}
	for _, c := range s.Config {
		if c.Key == "" {
			return errors.New("config key has empty name")
		}
		if cfgKeys[c.Key] {
			return fmt.Errorf("config key %q declared twice", c.Key)
		}
		cfgKeys[c.Key] = true
		if err := checkApplies("config", c.Key, c.AppliesTo); err != nil {
			return err
		}
	}
	featureIDs := map[string]bool{}
	for _, f := range s.Features {
		if f.ID == "" {
			return errors.New("feature has empty id")
		}
		if featureIDs[f.ID] {
			return fmt.Errorf("feature %q declared twice", f.ID)
		}
		featureIDs[f.ID] = true
		if err := checkApplies("feature", f.ID, f.AppliesTo); err != nil {
			return err
		}
		for k := range f.Config {
			if !cfgKeys[k] {
				return fmt.Errorf("feature %q: config key %q not declared in spec.config", f.ID, k)
			}
		}
		if len(f.Examples) == 0 {
			return fmt.Errorf("feature %q: at least one example is required", f.ID)
		}
		exNames := map[string]bool{}
		for _, ex := range f.Examples {
			if ex.Name == "" {
				return fmt.Errorf("feature %q: example has empty name", f.ID)
			}
			if exNames[ex.Name] {
				return fmt.Errorf("feature %q: example %q declared twice", f.ID, ex.Name)
			}
			exNames[ex.Name] = true
			inputs := 0
			if ex.InputFile != "" {
				inputs++
			}
			if ex.InputXML != "" {
				inputs++
			}
			if len(ex.InputBytes) > 0 {
				inputs++
			}
			if inputs != 1 {
				return fmt.Errorf("feature %q example %q: exactly one of input_file/input_xml/input_bytes is required (got %d)", f.ID, ex.Name, inputs)
			}
			if ex.Variant != "" && !variants[ex.Variant] {
				return fmt.Errorf("feature %q example %q: unknown variant %q", f.ID, ex.Name, ex.Variant)
			}
			if len(s.Variants) > 0 && ex.InputXML != "" && ex.Variant == "" {
				return fmt.Errorf("feature %q example %q: variant required for inline input_xml under multi-variant spec", f.ID, ex.Name)
			}
			for k := range ex.Config {
				if !cfgKeys[k] {
					return fmt.Errorf("feature %q example %q: config key %q not declared in spec.config", f.ID, ex.Name, k)
				}
			}
		}
	}
	// Case-level meta-schema (format-spec-cases.md §8): id/class/cite/view
	// shape. Additive — no-op for legacy examples that omit the new fields.
	if err := s.validateCases(); err != nil {
		return err
	}
	// Parity-runner config (tikal / bridge config-preset, #852). Additive —
	// a no-op for specs that declare no tikal/parity/bridge block.
	if err := s.validateParity(); err != nil {
		return err
	}
	return nil
}

// Dir is the directory of the spec file (used to resolve InputFile).
// Empty if the spec was constructed in-memory rather than loaded.
func (s *Spec) Dir() string { return s.dir }
