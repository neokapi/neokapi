package graph

import "time"

// Validity represents temporal bounds and tag-based scoping for graph edges.
// The temporal model uses time + generic tags (no hardcoded dimensions).
// Workspace-configurable tag vocabulary determines the meaning of tags.
type Validity struct {
	ValidFrom *time.Time        `json:"valid_from,omitempty"`
	ValidTo   *time.Time        `json:"valid_to,omitempty"`
	Tags      map[string]string `json:"tags,omitempty"`
}

// Scope represents an evaluation point for validity checks.
// When querying the graph, a Scope determines which edges are "active".
type Scope struct {
	At   time.Time         `json:"at"`
	Tags map[string]string `json:"tags,omitempty"`
}

// Now returns a Scope at the current time with no tag constraints.
func Now() Scope {
	return Scope{At: time.Now()}
}

// ScopeAt returns a Scope at the given time with no tag constraints.
func ScopeAt(t time.Time) Scope {
	return Scope{At: t}
}

// ScopeWithTags returns a Scope at the current time with the given tag constraints.
func ScopeWithTags(tags map[string]string) Scope {
	return Scope{At: time.Now(), Tags: tags}
}

// Matches returns true if this Validity is active at the given Scope.
// An edge with nil Validity always matches (unbounded).
// Time matching: ValidFrom <= scope.At < ValidTo (half-open interval).
// Tag matching: all Scope tags must be present in Validity tags with matching values.
// Validity tags not in Scope are ignored (open-world assumption).
func (v *Validity) Matches(s Scope) bool {
	if v == nil {
		return true
	}
	if v.ValidFrom != nil && s.At.Before(*v.ValidFrom) {
		return false
	}
	if v.ValidTo != nil && !s.At.Before(*v.ValidTo) {
		return false
	}
	for k, sv := range s.Tags {
		vv, ok := v.Tags[k]
		if !ok || vv != sv {
			return false
		}
	}
	return true
}

// IsExpired returns true if the validity has a ValidTo in the past.
func (v *Validity) IsExpired() bool {
	if v == nil || v.ValidTo == nil {
		return false
	}
	return v.ValidTo.Before(time.Now())
}

// IsActive returns true if the validity is currently active (Matches with Now()).
func (v *Validity) IsActive() bool {
	if v == nil {
		return true
	}
	return v.Matches(Now())
}
