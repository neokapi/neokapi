package backend

import (
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/project"
)

// SaveProjectFlow writes a flow's steps (and their inline options) to the
// recipe on disk. It is the persist behind the linear flow editor's save.
func (a *App) SaveProjectFlow(tabID, name string, spec *flow.StepsSpec) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	op := a.projects[tabID]
	if op == nil {
		return fmt.Errorf("tab %q not found", tabID)
	}
	if op.Project == nil {
		return errors.New("the tab has no project recipe")
	}
	if name == "" {
		return errors.New("a flow needs a name")
	}
	if spec == nil {
		spec = &flow.StepsSpec{}
	}
	if op.Project.Flows == nil {
		op.Project.Flows = map[string]*flow.StepsSpec{}
	}
	op.Project.Flows[name] = spec
	if op.Path != "" {
		return project.Save(op.Path, op.Project)
	}
	return nil
}

// SetDefaultFlow sets the project's defaults.flow, or clears it with an empty
// name, and writes the recipe. It refuses a name no flow carries.
func (a *App) SetDefaultFlow(tabID, name string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	op := a.projects[tabID]
	if op == nil {
		return fmt.Errorf("tab %q not found", tabID)
	}
	if op.Project == nil {
		return errors.New("the tab has no project recipe")
	}
	if name != "" {
		if _, ok := op.Project.Flows[name]; !ok {
			return fmt.Errorf("no flow named %q", name)
		}
	}
	op.Project.Defaults.Flow = name
	if op.Path != "" {
		return project.Save(op.Path, op.Project)
	}
	return nil
}

// RenameProjectFlow renames a flow, moving defaults.flow with it so the recipe
// stays consistent, and writes it. It refuses an unknown source or a name
// already taken.
func (a *App) RenameProjectFlow(tabID, oldName, newName string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	op := a.projects[tabID]
	if op == nil {
		return fmt.Errorf("tab %q not found", tabID)
	}
	if op.Project == nil {
		return errors.New("the tab has no project recipe")
	}
	if newName == "" {
		return errors.New("a flow needs a name")
	}
	if oldName == newName {
		return nil
	}
	spec, ok := op.Project.Flows[oldName]
	if !ok {
		return fmt.Errorf("no flow named %q", oldName)
	}
	if _, exists := op.Project.Flows[newName]; exists {
		return fmt.Errorf("a flow named %q already exists", newName)
	}
	delete(op.Project.Flows, oldName)
	op.Project.Flows[newName] = spec
	if op.Project.Defaults.Flow == oldName {
		op.Project.Defaults.Flow = newName
	}
	if op.Path != "" {
		return project.Save(op.Path, op.Project)
	}
	return nil
}
