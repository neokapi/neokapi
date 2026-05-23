package redaction

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// RulesFile is the on-disk shape of a dedicated redaction rules file. It is
// kept separate from the committed .kapi recipe so the sensitive term list
// can be gitignored. Example:
//
//	version: v1
//	placeholder: "[REDACTED:{category}]"
//	detectors: [rules]
//	rules:
//	  - term: "Mr Bean"
//	    category: person
//	  - pattern: "PROJECT-[A-Z]+"
//	    category: product
//	    flags: [ignorecase]
type RulesFile struct {
	Version     string   `yaml:"version,omitempty"`
	Placeholder string   `yaml:"placeholder,omitempty"`
	Detectors   []string `yaml:"detectors,omitempty"`
	Rules       []Rule   `yaml:"rules"`
}

// RulesFileVersion is the current schema version written by Save.
const RulesFileVersion = "v1"

// LoadRulesFile reads and parses a redaction rules file.
func LoadRulesFile(path string) (*RulesFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("redaction: read rules %s: %w", path, err)
	}
	var rf RulesFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return nil, fmt.Errorf("redaction: parse rules %s: %w", path, err)
	}
	return &rf, nil
}

// Detector compiles the file's rules into a [RuleDetector].
func (rf *RulesFile) Detector() (*RuleDetector, error) {
	return NewRuleDetector(rf.Rules)
}

// Save writes the rules file as YAML, creating parent directories as needed.
// Version defaults to the current schema version when unset.
func (rf *RulesFile) Save(path string) error {
	if rf.Version == "" {
		rf.Version = RulesFileVersion
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("redaction: create rules dir: %w", err)
	}
	data, err := yaml.Marshal(rf)
	if err != nil {
		return fmt.Errorf("redaction: encode rules: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("redaction: write rules %s: %w", path, err)
	}
	return nil
}
