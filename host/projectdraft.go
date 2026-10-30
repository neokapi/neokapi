package host

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
)

// A draft project is a folder an embedding host has opened on a recipe that
// is not on disk yet. The desktop opens any folder and writes its kapi.yaml
// when a person first changes something the recipe holds; until then the
// folder is left as it was found. A draft's store keeps its state outside the
// folder, and the workspace registers nothing for it: a registered checkout
// without a recipe reads everywhere as a project whose recipe was deleted.
// The CLI never binds a draft.

// draftProject is where the store of a draft keeps its state, and the key its
// context is kept under in the workspace.
type draftProject struct {
	stateDir string
	key      workspace.ProjectKey
}

// DraftStateDir is the directory the state of a draft project rooted at root
// is kept in: under the data root, named for the checkout, so the same folder
// opened again finds the cache it left.
func DraftStateDir(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	key := workspace.KeyForCheckout(NormalizeCheckoutPath(abs))
	return filepath.Join(DataDir(), "drafts", workspace.FileNameFor(key))
}

// BindDraftProject declares the folder at root a draft: the next ProjectDB for
// root keeps its state in stateDir, binds the context the workspace keeps under
// key, and registers nothing. Binding a root whose store is already open is an
// error, since the open handle would keep the place it was opened in.
func (a *App) BindDraftProject(root string, key workspace.ProjectKey, stateDir string) error {
	if root == "" || stateDir == "" || key == "" {
		return errors.New("draft project: a root, a state directory and a key are required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("draft project: resolve root %q: %w", root, err)
	}
	s := a.ensureProjectStores()
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, open := s.dbs[abs]; open {
		if d, ok := s.drafts[abs]; ok && d.stateDir == stateDir && d.key == key {
			return nil
		}
		return fmt.Errorf("draft project: the store of %s is already open", abs)
	}
	if s.drafts == nil {
		s.drafts = map[string]draftProject{}
	}
	s.drafts[abs] = draftProject{stateDir: stateDir, key: key}
	return nil
}

// UnbindDraftProject ends a folder's draft state, once its recipe is on disk:
// the store opened for the draft is closed, so the next ProjectDB opens the
// store in the folder's own `.kapi/`, registers the project and rebuilds the
// cache there. An unknown root is not an error.
func (a *App) UnbindDraftProject(root string) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("draft project: resolve root %q: %w", root, err)
	}
	s := a.ensureProjectStores()
	s.mu.Lock()
	_, bound := s.drafts[abs]
	delete(s.drafts, abs)
	s.mu.Unlock()
	if !bound {
		return nil
	}
	return a.CloseProjectDB(abs)
}

// ProjectStorePath is the file the store of the project rooted at root keeps
// its block cache and working set in: the folder's `.kapi/work/store.db`, or
// the draft's state directory for a folder bound as a draft.
func (a *App) ProjectStorePath(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	s := a.ensureProjectStores()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.layoutFor(abs).StorePath()
}

// layoutFor is the layout the store of the project rooted at abs opens in. The
// caller holds s.mu.
func (s *projectStores) layoutFor(abs string) project.Layout {
	if d, ok := s.drafts[abs]; ok {
		return project.Layout{
			Root:       abs,
			RecipePath: filepath.Join(abs, project.RecipeFileName),
			StateDir:   d.stateDir,
		}
	}
	return projectLayoutAt(abs)
}
