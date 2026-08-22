package host

import (
	"path/filepath"
	"strings"

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
	// and terms. The zero point is the project's default one — either because
	// nothing claimed these files, or because what did resolves to exactly the
	// defaults.
	Point project.GovernancePoint
	// Inputs are the group's source files, in the order they were given.
	Inputs []string
	// bindings is the resolved governance, filled in by resolveGroupBindings.
	// nil is valid: the project carries no bindings.
	bindings *ProjectBindings
}

// groupInputsByBinding partitions inputPaths by the governance that applies to
// each path, resolved per file through project.ResolveGovernanceFor — so a
// content item's own `channel:` partitions its files out of the rest of its
// collection, and a profile outside its validity window governs nothing.
// Paths outside the project root, and paths no content pattern claims, sit at
// the project's default point.
//
// Grouping is by what the point *resolves to*, not by the reference that
// selected it: every file resolving to the same profile, voice, terms and
// channel — two collections on one profile's channel, say — stays in one group
// and shares one tool chain. When nothing declares a channel at all, the whole
// input set is one group at the default point, which resolves the project-wide
// bindings exactly as an ungrouped run always did.
//
// The error is an unresolvable channel reference; a recipe that loaded cleanly
// cannot produce one.
func (a *App) groupInputsByBinding(cmd Command, proj *project.KapiProject, projectDir string, inputPaths []string) ([]bindingGroup, error) {
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
	index := make(map[string]int, 2)
	for _, in := range inputPaths {
		point := a.GovernancePointFor("", "")
		if rel, ok := projectRelPath(projectDir, in); ok {
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
// somewhere in the context space — a collection's channel, or a content item's
// own. Nothing to split on when nothing does, which is the answer that keeps an
// ordinary recipe on its single, ungrouped run.
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
// The key is what the point resolves to — the profile, the voice, the terms and
// the channel — not the reference that selected them. Channel counts because it
// selects an override inside the profile, so one voice on two channels is two
// voices in practice. The profile counts because a profile is answered by its
// own directory: two profiles binding nothing in the recipe still resolve
// different `.kapi/profiles/<name>/terms.json`. Two collections on one profile's
// channel share a profile name, so nothing splits for nothing.
func bindingKey(rc *project.ResolvedGovernance) string {
	var b strings.Builder
	b.WriteString(rc.Profile)
	b.WriteByte(0)
	b.WriteString(rc.Channel)
	b.WriteByte(0)
	if rc.Voice != nil {
		b.WriteString(rc.Voice.ProfileFile)
		b.WriteByte(0)
		b.WriteString(rc.Voice.Profile)
		b.WriteByte(0)
		b.WriteString(rc.Voice.Pack)
	}
	b.WriteByte(0)
	b.WriteString(rc.TermStore)
	return b.String()
}

// resolveGroupBindings fills in each group's governance. Bindings are resolved
// once per group, before the locale passes run, so a grouped run costs one
// resolution per distinct point rather than one per file — and a single
// default-point group costs the one project-wide resolution it always did.
func (a *App) resolveGroupBindings(cmd Command, proj *project.KapiProject, projectPath string, groups []bindingGroup) error {
	for i := range groups {
		b, err := a.resolveProjectBindings(cmd, proj, projectPath, groups[i].Point)
		if err != nil {
			return err
		}
		groups[i].bindings = b
	}
	return nil
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
