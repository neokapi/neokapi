package rulesview

import (
	"path"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/axis"
	"github.com/neokapi/neokapi/core/contextop"
)

// Places are the points of one project a rule's scope is read against, to
// say in a person's words where the rule holds.
type Places struct {
	// Space is the project's axis space, which says whether a scope covers a
	// point's coordinates.
	Space axis.Space
	// Root is the coordinates of the project's own point, where a rule that
	// holds across the project answers.
	Root map[string]string
	// Collections are the project's named collections and the coordinates
	// of each one's point, in recipe order.
	Collections []Place
	// Files are the project's source files and the coordinates of the point
	// each sits at, for a project whose collections have no names. Name is
	// the file's project-relative, slash-separated path.
	Files []Place
}

// Place is one named point of a project: a collection, or a source file.
type Place struct {
	Name        string
	Coordinates map[string]string
}

// Applies says where a rule at scope holds, and whether that is less than the
// whole project: "the whole project", the collections it covers ("docs,
// help"), or the folders of the files it covers when no collection has a
// name.
func (p Places) Applies(scope contextop.Scope) (string, bool) {
	if scope.Level == contextop.LevelWorkspace {
		return "every project in the workspace", false
	}
	if scope.AllProfiles && len(scope.Coordinates) == 0 {
		return "the whole project", false
	}
	if len(p.Collections) > 0 {
		var in []string
		for _, c := range p.Collections {
			if scope.CoversIn(p.Space, c.Coordinates) {
				in = append(in, c.Name)
			}
		}
		switch {
		case len(in) == len(p.Collections) && scope.CoversIn(p.Space, p.Root):
			return "the whole project", false
		case len(in) == 0:
			return "no collection of this project", true
		}
		return strings.Join(in, ", "), true
	}
	if scope.CoversIn(p.Space, p.Root) {
		return "the whole project", false
	}
	if folders := p.coveredFolders(scope); len(folders) > 0 {
		return strings.Join(folders, ", "), true
	}
	return "no file of this project", true
}

// coveredFolders names the folders whose source files sit where scope holds,
// for a project that names no collection to say it with.
func (p Places) coveredFolders(scope contextop.Scope) []string {
	var out []string
	for _, f := range p.Files {
		if !scope.CoversIn(p.Space, f.Coordinates) {
			continue
		}
		dir := path.Dir(f.Name)
		if dir == "." {
			dir = "the project's root folder"
		}
		if !slices.Contains(out, dir) {
			out = append(out, dir)
		}
	}
	slices.Sort(out)
	return out
}
