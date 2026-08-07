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
	// Collection names the content collection whose governance this group
	// carries. Empty means the project's default point — either because no
	// collection claimed these files, or because the collection that did
	// resolves to exactly the defaults.
	Collection string
	// Inputs are the group's source files, in the order they were given.
	Inputs []string
	// bindings is the resolved governance, filled in by resolveGroupBindings.
	// nil is valid: the project carries no bindings.
	bindings *ProjectBindings
}

// groupInputsByBinding partitions inputPaths by the governance that applies to
// each path, found through the collection that owns it
// (project.CollectionForPath, the same glob matching used to resolve a file's
// target). Paths outside the project root, and paths no content pattern claims,
// sit at the project's default point.
//
// Grouping is by what the channel reference *resolves to*, not by the reference
// itself: every collection resolving to the same voice, terms and channel — two
// collections on one profile's channel, say — stays in one group and shares one
// tool chain. When no collection declares a channel at all, the whole input set
// is one group naming no collection, which resolves the project-wide bindings
// exactly as an ungrouped run always did.
//
// The error is an unresolvable channel reference (project.ResolveGovernance); a
// recipe that loaded cleanly cannot produce one.
func groupInputsByBinding(proj *project.KapiProject, projectDir string, inputPaths []string) ([]bindingGroup, error) {
	whole := []bindingGroup{{Inputs: inputPaths}}
	if proj == nil || projectDir == "" || len(inputPaths) == 0 || !hasCollectionContext(proj) {
		return whole, nil
	}

	defaultRC, err := proj.ResolveGovernance("")
	if err != nil {
		return nil, err
	}
	defaults := bindingKey(defaultRC)

	var groups []bindingGroup
	index := make(map[string]int, 2)
	for _, in := range inputPaths {
		collection := ""
		if rel, ok := projectRelPath(projectDir, in); ok {
			collection = proj.CollectionForPath(rel)
		}
		rc, rerr := proj.ResolveGovernance(collection)
		if rerr != nil {
			return nil, rerr
		}
		key := bindingKey(rc)
		if key == defaults {
			// The collection is governed exactly as the project is: name no
			// collection, so the group resolves through the identical
			// project-wide path.
			collection = ""
		}
		i, seen := index[key]
		if !seen {
			i = len(groups)
			index[key] = i
			groups = append(groups, bindingGroup{Collection: collection})
		}
		groups[i].Inputs = append(groups[i].Inputs, in)
	}
	return groups, nil
}

// hasCollectionContext reports whether any collection places itself somewhere
// in the context space. Nothing to split on when none does — the answer that
// keeps an ordinary recipe on its single, ungrouped run.
func hasCollectionContext(proj *project.KapiProject) bool {
	for i := range proj.Collections {
		if proj.Collections[i].Channel != "" {
			return true
		}
	}
	return false
}

// bindingKey is the identity of resolved governance: two collections with equal
// keys are governed identically and belong in one group.
//
// The key is what the channel reference resolves to — the voice, the terms, and
// the channel — not the reference that selected them. Channel counts because it
// selects an override inside the profile, so one voice on two channels is two
// voices in practice. The profile name does not: two collections on one
// profile's channel produce the same run, and keying on the label would split it
// for nothing.
func bindingKey(rc *project.ResolvedGovernance) string {
	var b strings.Builder
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
	b.WriteString(rc.Terms)
	return b.String()
}

// resolveGroupBindings fills in each group's governance. Bindings are resolved
// once per group, before the locale passes run, so a grouped run costs one
// resolution per distinct point rather than one per file — and a single
// default-point group costs the one project-wide resolution it always did.
func (a *App) resolveGroupBindings(cmd Command, proj *project.KapiProject, projectPath string, groups []bindingGroup) error {
	for i := range groups {
		b, err := a.resolveProjectBindings(cmd, proj, projectPath, groups[i].Collection)
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
