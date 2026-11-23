package host

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/project"
)

// bindingGroup is one partition of a run's input set: the files governed
// identically, and so sharing one voice and one terms store.
//
// A project flow resolves its governance once and bakes it into the tool chain
// before any content is seen (buildFlowTools → applyBindings), so a run cannot
// switch voice per file. Splitting the input set instead is what makes the
// coordinates real: one run per (binding group, locale pass), each with its own
// bindings and its own tools. A recipe whose collections are all governed the
// same way produces exactly one group, so the common case keeps the single run,
// the single tool chain, and the same event stream.
type bindingGroup struct {
	// Point is the place in the context space this group's files sit at, as one
	// representative of them: every file in the group resolves to the same
	// governance, so re-resolving through any of them yields the group's voice
	// and terms. The zero point is the project's default one, either because
	// nothing claimed these files or because what did resolves to exactly the
	// defaults.
	Point project.GovernancePoint
	// Inputs are the group's source files, in the order they were given.
	Inputs []string
	// Redaction is the redaction policy that governs every file of the group
	// (project.RedactionForPath). A flow's redact step that names no rules of
	// its own redacts under it (stepToolConfig).
	Redaction project.ResolvedRedaction
}

// groupInputsByBinding partitions inputPaths by the governance that applies to
// each path (groupInputsByGovernance), then splits each group by the redaction
// policy that governs its files (splitByRedaction), so a flow's redact step
// redacts every file under that file's own policy.
//
// The error is an unresolvable channel reference; a recipe that loaded cleanly
// cannot produce one.
func (a *App) groupInputsByBinding(cmd Command, proj *project.KapiProject, projectDir string, inputPaths []string) ([]bindingGroup, error) {
	groups, err := a.groupInputsByGovernance(cmd, proj, projectDir, inputPaths)
	if err != nil {
		return nil, err
	}
	return splitByRedaction(proj, projectDir, groups), nil
}

// splitByRedaction splits each group by the redaction policy that governs its
// files, keeping the order of the files within each part. A group with no
// files keeps the policy `defaults.redaction` declares.
func splitByRedaction(proj *project.KapiProject, projectDir string, groups []bindingGroup) []bindingGroup {
	if proj == nil {
		return groups
	}
	var out []bindingGroup
	for _, g := range groups {
		if len(g.Inputs) == 0 {
			g.Redaction = proj.RedactionForPath("")
			out = append(out, g)
			continue
		}
		index := map[string]int{}
		for _, in := range g.Inputs {
			resolved := proj.RedactionForPath(redactionPath(projectDir, in))
			// Files no policy governs share one part, whichever declaration
			// (a disabled one, or none) resolved them.
			key := resolved.At
			if resolved.Spec == nil {
				key = ""
			}
			i, seen := index[key]
			if !seen {
				i = len(out)
				index[key] = i
				out = append(out, bindingGroup{Point: g.Point, Redaction: resolved})
			}
			out[i].Inputs = append(out[i].Inputs, in)
		}
	}
	return out
}

// groupInputsByGovernance partitions inputPaths by the governance that
// applies to each path, resolved per file through
// project.ResolveGovernanceFor. A content item's own `channel:` partitions its
// files out of the rest of its collection, and a profile outside its validity
// window governs nothing. Paths outside the project root, and paths no content
// pattern claims, sit at the project's default point.
//
// Grouping is by what the point *resolves to*, not by the reference that
// selected it. Every file resolving to the same profile, voice, terms and
// channel (two collections on one profile's channel, say) stays in one group
// and shares one tool chain. When nothing declares a channel at all, the whole
// input set is one group at the default point, which resolves the project-wide
// bindings.
func (a *App) groupInputsByGovernance(cmd Command, proj *project.KapiProject, projectDir string, inputPaths []string) ([]bindingGroup, error) {
	whole := []bindingGroup{{Inputs: inputPaths}}
	if proj == nil || projectDir == "" || len(inputPaths) == 0 || !hasCollectionContext(proj) {
		return whole, nil
	}

	defaultRC, err := a.ResolveGovernanceAtPoint(cmd, proj, a.GovernancePointFor("", ""))
	if err != nil {
		return nil, err
	}
	defaults := bindingKey(defaultRC)

	var groups []bindingGroup
	index := make(map[[5]string]int, 2)
	for _, in := range inputPaths {
		point := a.GovernancePointFor("", "")
		if rel, ok := projectRelPath(projectDir, in); ok && !ProjectIgnores(projectDir, rel) {
			point = a.GovernancePointFor("", rel)
		}
		rc, rerr := a.ResolveGovernanceAtPoint(cmd, proj, point)
		if rerr != nil {
			return nil, rerr
		}
		key := bindingKey(rc)
		if key == defaults {
			// This file is governed exactly as the project is: carry the empty
			// point, so the group resolves through the identical project-wide
			// path.
			point = a.GovernancePointFor("", "")
		}
		i, seen := index[key]
		if !seen {
			i = len(groups)
			index[key] = i
			groups = append(groups, bindingGroup{Point: point})
		}
		groups[i].Inputs = append(groups[i].Inputs, in)
	}
	return groups, nil
}

// hasCollectionContext reports whether anything in the recipe places content
// somewhere in the context space: a collection's channel, or a content item's
// own. When nothing does, there is nothing to split on, and an ordinary recipe
// keeps its single, ungrouped run.
func hasCollectionContext(proj *project.KapiProject) bool {
	for i := range proj.Collections {
		if proj.Collections[i].Channel != "" {
			return true
		}
		for _, item := range proj.Collections[i].EffectiveItems() {
			if item.Channel != "" {
				return true
			}
		}
	}
	return false
}

// bindingKey is the identity of resolved governance: two files with equal keys
// are governed identically and belong in one group.
//
// The key is what the point resolves to (the profile, the voice, the terms and
// the channel), not the reference that selected them. Channel counts because it
// selects an override inside the profile, so one voice on two channels is two
// voices in practice. The profile counts because the store answers a profile by
// its name: two profiles binding nothing in the recipe still resolve different
// stored voices and different profile-scoped concepts. Two collections on one
// profile's channel share a profile name, so nothing splits for nothing.
func bindingKey(rc *project.ResolvedGovernance) [5]string {
	k := [5]string{rc.Profile, rc.Channel, "", "", rc.TermStore}
	if rc.Voice != nil {
		k[2], k[3] = rc.Voice.Profile, rc.Voice.Pack
	}
	return k
}

// localeBindings resolves the governance in force at a point for one target
// locale, and remembers each answer for the length of a run.
//
// A run that fans out over locales resolves at (point, locale) rather than once
// for the whole run, because a term rule pairs a source term with the wording
// approved for THIS target (terms.Concept.PreferredTerm). Resolved before the
// fan-out, with no locale to ask about, a set would hold only the
// do-not-translate concepts. The staleness gate resolves per locale
// (host/verify_staleness.go), so resolving the same way here lets the gate
// recompute a context fingerprint over the rules the producer had.
//
// The cache is keyed on the point and the locale, so a run with many groups and
// many locales pays one resolution per distinct pair. at is safe for concurrent
// use, since a convergence pass fans its locales out over workers.
type localeBindings struct {
	app         *App
	cmd         Command
	proj        *project.KapiProject
	projectPath string

	mu    sync.Mutex
	cache map[[4]string]*ProjectBindings
}

// newLocaleBindings builds the resolver a run keeps for its whole fan-out.
func (a *App) newLocaleBindings(cmd Command, proj *project.KapiProject, projectPath string) *localeBindings {
	return &localeBindings{
		app:         a,
		cmd:         cmd,
		proj:        proj,
		projectPath: projectPath,
		cache:       map[[4]string]*ProjectBindings{},
	}
}

// at returns the bindings governing point for targetLang, resolving them on
// first ask. A nil result is an answer and is cached with the rest: the project
// carries no bindings at all.
func (l *localeBindings) at(point project.GovernancePoint, targetLang string) (*ProjectBindings, error) {
	key := [4]string{point.Profile, point.Collection, point.Path, targetLang}
	l.mu.Lock()
	defer l.mu.Unlock()
	if b, ok := l.cache[key]; ok {
		return b, nil
	}
	b, err := l.app.resolveBindingsFor(l.cmd, l.proj, l.projectPath, point, targetLang)
	if err != nil {
		return nil, err
	}
	l.cache[key] = b
	return b, nil
}

// projectRelPath returns path relative to the project root, slash-separated,
// and whether it is inside the root at all. Relative inputs are resolved
// against the working directory first.
func projectRelPath(root, path string) (string, bool) {
	abs := path
	if !filepath.IsAbs(abs) {
		if r, err := filepath.Abs(abs); err == nil {
			abs = r
		}
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", false
	}
	relSlash := filepath.ToSlash(rel)
	if relSlash == ".." || strings.HasPrefix(relSlash, "../") {
		return "", false
	}
	return relSlash, true
}
