package backend

import (
	"errors"
	"path/filepath"

	"github.com/neokapi/neokapi/core/project"
)

// StateToVouch is the state of a project the desktop could not open because
// kapi on this machine did not write it: the entries, relative to the project
// root, and where the state appears to come from.
type StateToVouch struct {
	Path    string   `json:"path"`
	Entries []string `json:"entries"`
	Origin  string   `json:"origin"`
}

// ProjectStateToVouch reports the state a person is asked to vouch for before
// the project at path opens, or nil when there is none: no state yet, or state
// this machine wrote. The frontend asks it when opening a project fails; an
// empty path names the project OpenProject refused last.
func (a *App) ProjectStateToVouch(path string) (*StateToVouch, error) {
	if path == "" {
		last, _ := a.lastStateRefusal.Load().(string)
		if last == "" {
			return nil, nil
		}
		path = last
	}
	target, err := resolveOpenTarget(path)
	if err != nil {
		return nil, err
	}
	layout := project.LayoutAt(filepath.Dir(target.recipe))
	origin, err := project.DescribeState(layout)
	if err != nil {
		return nil, err
	}
	if origin.Verified || len(origin.Entries) == 0 {
		return nil, nil
	}
	return &StateToVouch{Path: target.recipe, Entries: origin.Entries, Origin: origin.Origin}, nil
}

// VouchForProjectState records that the person vouches for the state of the
// project at path, after the dialog showed it (ProjectStateToVouch), so kapi
// uses it as this machine's own. State git tracks stays refused: untracking it
// is the remedy for that.
func (a *App) VouchForProjectState(path string) error {
	target, err := resolveOpenTarget(path)
	if err != nil {
		return err
	}
	if target.recipe == "" {
		return errors.New("name a project to vouch for")
	}
	return project.AdoptState(project.LayoutAt(filepath.Dir(target.recipe)))
}
