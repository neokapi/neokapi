package project

import "fmt"

// A recipe declares a redaction policy in three places: `defaults.redaction`
// for the project, `redaction:` on a collection for the files its items claim,
// and `redaction:` on a content item for the files that item claims. The most
// specific declaration governs a file, and it replaces the others as a whole:
// rule lists, detectors and placeholders are never merged across levels, so a
// collection that names its own rules file redacts with that file alone.

// ResolvedRedaction is the redaction policy that governs one file, with the
// recipe key that declares it.
type ResolvedRedaction struct {
	// Spec is the policy, or nil when the file is not redacted: nothing
	// declares a policy for it, or the declaration that governs it is
	// switched off (`enabled: false`).
	Spec *RedactionSpec
	// At is the recipe key of the governing declaration, such as
	// `defaults.redaction` or `collections[1].content[0].redaction`, and ""
	// when nothing declares one.
	At string
}

// RedactionForPath resolves the redaction policy for the file at relPath, a
// project-relative, slash-separated path: the redaction of the item that
// claims the file (ItemForPath's claim), else the redaction of that item's
// collection, else `defaults.redaction`. An empty relPath names content that
// sits in no file, which the defaults govern. Spec is a copy, so a caller
// cannot rewrite the recipe's policy through it.
func (p *KapiProject) RedactionForPath(relPath string) ResolvedRedaction {
	if p == nil {
		return ResolvedRedaction{}
	}
	if relPath != "" {
		if claim := p.claimsForPath(relPath, fixedNoReader(false)).item(); claim.ok {
			if claim.item.Redaction != nil {
				return resolvedRedaction(claim.item.Redaction, fmt.Sprintf("collections[%d].content[%d].redaction", claim.coll, claim.index))
			}
			if c := p.Collections[claim.coll]; c.Redaction != nil {
				return resolvedRedaction(c.Redaction, fmt.Sprintf("collections[%d].redaction", claim.coll))
			}
		}
	}
	if p.Defaults.Redaction != nil {
		return resolvedRedaction(p.Defaults.Redaction, "defaults.redaction")
	}
	return ResolvedRedaction{}
}

// DeclaresRedaction reports whether any declaration in the recipe switches a
// redaction policy on, for any file. A check that cannot name the file it
// guards, such as a flow built once for every file of a run, requires
// redaction when this is true.
func (p *KapiProject) DeclaresRedaction() bool {
	return len(p.DeclaredRedactions()) > 0
}

// DeclaredRedactions returns every policy the recipe switches on, in recipe
// order: `defaults.redaction`, then each collection's redaction followed by
// its items'. Content whose file is unknown is redacted under all of them.
func (p *KapiProject) DeclaredRedactions() []ResolvedRedaction {
	if p == nil {
		return nil
	}
	var out []ResolvedRedaction
	add := func(spec *RedactionSpec, at string) {
		if spec != nil && spec.Enabled {
			out = append(out, resolvedRedaction(spec, at))
		}
	}
	add(p.Defaults.Redaction, "defaults.redaction")
	for i, c := range p.Collections {
		add(c.Redaction, fmt.Sprintf("collections[%d].redaction", i))
		for j, item := range c.Content {
			add(item.Redaction, fmt.Sprintf("collections[%d].content[%d].redaction", i, j))
		}
	}
	return out
}

func resolvedRedaction(spec *RedactionSpec, at string) ResolvedRedaction {
	if !spec.Enabled {
		return ResolvedRedaction{At: at}
	}
	cp := *spec
	cp.Detectors = append([]string(nil), spec.Detectors...)
	return ResolvedRedaction{Spec: &cp, At: at}
}
