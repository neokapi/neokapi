package profile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
	"gopkg.in/yaml.v3"
)

const (
	ConstraintProhibitedPattern = "prohibited_pattern"
	ConstraintGuidance          = "guidance"
)

// Constraint is shared across a selected profile's presentation overrides.
// Guidance is supplied to writers but is not a deterministic factual check.
type Constraint struct {
	ID         string                `json:"id" yaml:"id"`
	Version    int                   `json:"version" yaml:"version"`
	Source     string                `json:"source" yaml:"source"`
	Statement  string                `json:"statement" yaml:"statement"`
	Kind       string                `json:"kind" yaml:"kind"`
	Regex      string                `json:"regex,omitempty" yaml:"regex,omitempty"`
	Scope      ConstraintScope       `json:"scope,omitzero" yaml:"scope,omitempty"`
	Exceptions []ConstraintException `json:"exceptions,omitempty" yaml:"exceptions,omitempty"`
}

// ConstraintScope matches every populated coordinate exactly. Locale spelling
// is normalized, but a language never implicitly includes its regional variants.
type ConstraintScope struct {
	Locale  model.LocaleID `json:"locale,omitempty" yaml:"locale,omitempty"`
	Channel string         `json:"channel,omitempty" yaml:"channel,omitempty"`
	Persona string         `json:"persona,omitempty" yaml:"persona,omitempty"`
}

// ConstraintException records the profile author's asserted approval provenance.
// These local fields do not authenticate the named reviewer.
type ConstraintException struct {
	Scope       ConstraintScope `json:"scope" yaml:"scope"`
	Reason      string          `json:"reason" yaml:"reason"`
	ApprovedBy  string          `json:"approved_by" yaml:"approved_by"`
	ApprovalRef string          `json:"approval_ref" yaml:"approval_ref"`
}

// ConstraintResolution explains why a constraint applies at the resolved point.
type ConstraintResolution struct {
	Constraint Constraint            `json:"constraint"`
	Status     string                `json:"status"` // applicable, out_of_scope, excepted
	Exceptions []ConstraintException `json:"exceptions,omitempty"`
}

// UnmarshalJSON rejects unknown keys throughout the new constraint record.
func (c *Constraint) UnmarshalJSON(data []byte) error {
	type plain Constraint
	var decoded plain
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&decoded); err != nil {
		return err
	}
	*c = Constraint(decoded)
	return nil
}

// UnmarshalYAML keeps constraint keys strict even inside a lenient profile load.
func (c *Constraint) UnmarshalYAML(node *yaml.Node) error {
	type plain Constraint
	data, err := yaml.Marshal(node)
	if err != nil {
		return err
	}
	var decoded plain
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&decoded); err != nil {
		return err
	}
	*c = Constraint(decoded)
	return nil
}

func cloneConstraints(in []Constraint) []Constraint {
	if in == nil {
		return nil // Preserve omitted fields in legacy profile serialization.
	}
	out := append([]Constraint{}, in...)
	for i := range out {
		out[i].Exceptions = append([]ConstraintException(nil), in[i].Exceptions...)
	}
	return out
}

// ConstraintResolutions returns the constraints at the point last selected by
// ResolveProfile. An unresolved profile uses the default, unbound point.
func ConstraintResolutions(p *VoiceProfile) []ConstraintResolution {
	out := []ConstraintResolution{}
	if p == nil {
		return out
	}
	for _, c := range cloneConstraints(p.Constraints) {
		r := ConstraintResolution{Constraint: c, Status: "applicable"}
		if !c.Scope.matches(p.constraintScope) {
			r.Status = "out_of_scope"
			out = append(out, r)
			continue
		}
		for _, exception := range c.Exceptions {
			if exception.Scope.matches(p.constraintScope) {
				r.Status = "excepted"
				r.Exceptions = append(r.Exceptions, exception)
			}
		}
		out = append(out, r)
	}
	return out
}

func (s ConstraintScope) matches(point ConstraintScope) bool {
	if s.Locale != "" && locale.Normalize(s.Locale) != locale.Normalize(point.Locale) {
		return false
	}
	if s.Channel != "" && s.Channel != point.Channel {
		return false
	}
	return s.Persona == "" || s.Persona == point.Persona
}

func validateConstraints(p *VoiceProfile) []ProfileProblem {
	problems := []ProfileProblem{}
	if p == nil {
		return problems
	}
	seen := map[string]bool{}
	for i, c := range p.Constraints {
		field := fmt.Sprintf("constraints[%d]", i)
		add := func(message string) {
			problems = append(problems, ProfileProblem{Field: field, Message: message})
		}
		for _, value := range []string{c.ID, c.Source, c.Statement} {
			if strings.TrimSpace(value) == "" {
				add("id, source and statement are required")
				break
			}
		}
		if c.Version < 1 {
			add("version must be positive")
		}
		if seen[c.ID] {
			add("duplicate constraint id " + c.ID)
		}
		seen[c.ID] = true
		validateScope := func(scope ConstraintScope, path string) {
			if scope.Locale != "" {
				if _, err := locale.Canonical(string(scope.Locale)); err != nil {
					add(path + ".locale: " + err.Error())
				}
			}
			for _, coordinate := range []struct{ name, value string }{
				{name: "channel", value: scope.Channel},
				{name: "persona", value: scope.Persona},
			} {
				if coordinate.value != "" && strings.TrimSpace(coordinate.value) == "" {
					add(path + "." + coordinate.name + " cannot contain only whitespace")
				}
			}
		}
		validateScope(c.Scope, "scope")
		switch c.Kind {
		case ConstraintGuidance:
			if c.Regex != "" {
				add("guidance cannot declare a regex")
			}
		case ConstraintProhibitedPattern:
			re := compilePattern(c.Regex)
			if re == nil || strings.TrimSpace(c.Regex) == "" {
				add("prohibited_pattern requires a valid, nonempty regex")
			} else if re.MatchString("") {
				add("prohibited_pattern cannot match empty text")
			}
		default:
			add("kind must be prohibited_pattern or guidance")
		}
		for j, exception := range c.Exceptions {
			validateScope(exception.Scope, fmt.Sprintf("exceptions[%d].scope", j))
			if exception.Scope == (ConstraintScope{}) {
				add("exception requires a nonempty scope")
			}
			for _, value := range []string{exception.Reason, exception.ApprovedBy, exception.ApprovalRef} {
				if strings.TrimSpace(value) == "" {
					add("exception requires reason, approved_by and approval_ref")
					break
				}
			}
		}
	}
	return problems
}

func constraintError(p *VoiceProfile) error {
	problems := validateConstraints(p)
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("invalid voice constraint: %s: %s", problems[0].Field, problems[0].Message)
}

func constraintFindings(p *VoiceProfile, text string, runs []model.Run) []VoiceFinding {
	findings := []VoiceFinding{}
	if err := constraintError(p); err != nil {
		return append(findings, VoiceFinding{
			Category: "style", Severity: SeverityCritical, Message: err.Error(),
			Metadata: map[string]string{"constraint_error": "true"},
		})
	}
	for _, r := range ConstraintResolutions(p) {
		if r.Status != "applicable" || r.Constraint.Kind != ConstraintProhibitedPattern {
			continue
		}
		c := r.Constraint
		rules := &VoiceProfile{Style: StyleRules{ProhibitedPatterns: []Pattern{{
			Regex: c.Regex, Description: c.Statement, Severity: "critical",
		}}}}
		for _, finding := range PatternHitsToFindings(MatchPatterns(rules, text), text, runs) {
			finding.Metadata["constraint_id"] = c.ID
			finding.Metadata["constraint_version"] = strconv.Itoa(c.Version)
			finding.Metadata["constraint_source"] = c.Source
			findings = append(findings, finding)
		}
	}
	return findings
}

func constraintPatternCount(p *VoiceProfile) int {
	count := 0
	for _, r := range ConstraintResolutions(p) {
		if r.Status == "applicable" && r.Constraint.Kind == ConstraintProhibitedPattern {
			count++
		}
	}
	return count
}

func constraintGuide(p *VoiceProfile) string {
	lines := []string{}
	for _, r := range ConstraintResolutions(p) {
		c := r.Constraint
		label := fmt.Sprintf("%s v%d (%s)", c.ID, c.Version, c.Source)
		switch r.Status {
		case "out_of_scope":
			continue
		case "excepted":
			for _, e := range r.Exceptions {
				lines = append(lines, fmt.Sprintf("%s: excepted here: %s (asserted approval: %s; %s).",
					label, e.Reason, e.ApprovedBy, e.ApprovalRef))
			}
		default:
			coverage := "mandatory critical pattern: " + c.Regex
			if c.Kind == ConstraintGuidance {
				coverage = "guidance; semantic verification unsupported by deterministic checks"
			}
			lines = append(lines, fmt.Sprintf("%s: %s [%s]", label, c.Statement, coverage))
		}
	}
	return strings.Join(lines, "\n")
}
