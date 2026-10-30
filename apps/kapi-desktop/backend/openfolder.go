package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/host"
	"gopkg.in/yaml.v3"
)

// A folder opens whether or not it holds a recipe. One without a kapi.yaml
// opens on a recipe held in memory, the one `kapi init` would write there.
// ensureRecipe writes it the first time a person changes something it holds or
// acts on the project: keeping a suggestion, connecting an assistant, saving a
// setting, running a flow. Until then the folder holds no kapi.yaml and the
// workspace has no record of it. Every surface that reads the recipe from disk
// answers as for a project with nothing in it.

// openTarget is what a path handed to OpenProject names: the recipe path the
// tab is keyed on, and whether that recipe is on disk.
type openTarget struct {
	recipe string
	exists bool
}

// resolveOpenTarget turns a path a person chose, dropped or typed into the
// recipe path of the project it names. A folder names the kapi.yaml inside it.
// A recipe file names itself. Any other file names its parent folder, which is
// how a single file opens. A folder without a kapi.yaml inside a kapi project
// names that project's recipe, found by the upward walk the CLI makes.
func resolveOpenTarget(path string) (openTarget, error) {
	if path == "" {
		return openTarget{}, errors.New("name a folder to open")
	}
	fi, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// The path itself is gone. A recipe path is resolved against its
			// folder so the caller can tell a lost recipe from a lost folder.
			if isRecipeFile(path) {
				return openTarget{recipe: path}, nil
			}
			return openTarget{}, fmt.Errorf("%s no longer exists", path)
		}
		return openTarget{}, err
	}
	if !fi.IsDir() && isRecipeFile(path) {
		return openTarget{recipe: path, exists: true}, nil
	}
	dir := path
	if !fi.IsDir() {
		dir = filepath.Dir(path)
	}
	recipe := filepath.Join(dir, project.RecipeFileName)
	if _, serr := os.Stat(recipe); serr == nil {
		return openTarget{recipe: recipe, exists: true}, nil
	}
	if layout, lerr := project.ResolveLayout(dir); lerr == nil {
		if _, serr := os.Stat(layout.RecipePath); serr == nil {
			return openTarget{recipe: layout.RecipePath, exists: true}, nil
		}
	}
	return openTarget{recipe: recipe}, nil
}

// isRecipeFile reports whether a file path names a recipe rather than content:
// a kapi.yaml, or a file with the .kapi extension an older desktop wrote.
func isRecipeFile(path string) bool {
	return filepath.Base(path) == project.RecipeFileName || filepath.Ext(path) == ".kapi"
}

// broadFolder reports whether a folder is one no project is: the root of the
// file system, or the person's home directory. Such a folder opens, but nothing
// walks it. Scanning a home directory for content, polling it for changes or
// building a block cache inside it would read every file the account owns.
func broadFolder(dir string) bool {
	clean := filepath.Clean(dir)
	if filepath.Dir(clean) == clean {
		return true
	}
	if home, err := userHomeDir(); err == nil && sameDir(clean, home) {
		return true
	}
	return false
}

// sameDir compares two directory paths, following symlinks where they resolve.
func sameDir(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ra, erra := filepath.EvalSymlinks(a)
	rb, errb := filepath.EvalSymlinks(b)
	return erra == nil && errb == nil && ra == rb
}

// draftRecipe builds the recipe `kapi init` would write for root, carrying id,
// as the bytes it would write and as the project they parse to. The project name is the
// folder's, the source language is sourceLocale (en when empty), and the
// collections are the ones kapi init proposes from the files in the tree. A
// broad folder gets no collections, since its tree is not scanned.
func draftRecipe(root, id, sourceLocale string, formats *registry.FormatRegistry) (*project.KapiProject, []byte, error) {
	if sourceLocale == "" {
		sourceLocale = "en"
	}
	var proposals []host.ProposedCollection
	if !broadFolder(root) {
		var err error
		proposals, err = host.ProposeCollections(root, host.ProposeOptions{
			Formats:      formats,
			SourceLocale: sourceLocale,
		})
		if err != nil {
			return nil, nil, err
		}
	}
	data := host.ScaffoldRecipe(yamlName(filepath.Base(root)), id, sourceLocale, nil, proposals)
	var proj project.KapiProject
	if err := yaml.Unmarshal(data, &proj); err != nil {
		return nil, nil, fmt.Errorf("read the recipe for %s: %w", root, err)
	}
	if err := proj.Validate(); err != nil {
		return nil, nil, fmt.Errorf("the recipe for %s: %w", root, err)
	}
	return &proj, data, nil
}

// yamlName renders a folder name as a YAML scalar. ScaffoldRecipe writes the
// name as given, and a folder may be called anything, "Docs: 2026" included.
func yamlName(name string) string {
	out, err := yaml.Marshal(name)
	if err != nil {
		return name
	}
	return strings.TrimSpace(string(out))
}

// draftSourceLocale is the source language a recipe drafted here declares:
// the language the app's interface is in, en when that is unset or does not
// name a language.
func (a *App) draftSourceLocale() string {
	tag := string(a.T().Locale())
	if tag == "" {
		return "en"
	}
	id, err := locale.Canonical(tag)
	if err != nil || id == "" {
		return "en"
	}
	return string(id)
}

// initFormats is the format registry a drafted recipe's collections are
// proposed from: the built-in formats, as `kapi init` proposes them, so a
// recipe never names a format that only a plugin installed here can read.
func (a *App) initFormats() *registry.FormatRegistry {
	a.initFormatsOnce.Do(func() {
		e := &host.App{}
		e.InitRegistries()
		a.initFormatsReg = e.FormatReg
	})
	return a.initFormatsReg
}

// registeredCheckout reports whether the workspace records a checkout at dir:
// a folder some kapi surface opened or ran in while it held a recipe.
func (a *App) registeredCheckout(dir string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), workspaceHomeTimeout)
	defer cancel()
	ws, err := a.hostEngine().Workspace(ctx)
	if err != nil {
		return false
	}
	regs, err := ws.Projects(ctx)
	if err != nil {
		return false
	}
	want := host.NormalizeCheckoutPath(dir)
	for _, reg := range regs {
		for _, c := range reg.Checkouts {
			if c == want || sameDir(c, dir) {
				return true
			}
		}
	}
	return false
}

// onDisk reports whether the tab's recipe is a file a reader can load.
func (op *openProject) onDisk() bool {
	return op != nil && op.Path != "" && !op.unsaved.Load()
}

// draftID is the project id a folder opened without a recipe carries: the one
// minted the first time the folder was opened, kept in the draft's state
// directory under the data root, so context written before the recipe exists
// is found again when the folder is reopened.
func draftID(stateDir string) string {
	path := filepath.Join(stateDir, "id")
	if data, err := os.ReadFile(path); err == nil {
		id := strings.TrimSpace(string(data))
		if id != "" && project.ValidateID(id) == nil {
			return id
		}
	}
	id := project.NewID()
	if err := os.MkdirAll(stateDir, 0o755); err == nil {
		_ = os.WriteFile(path, []byte(id+"\n"), 0o644)
	}
	return id
}

// errBroadFolder refuses a recipe for a home directory or the root of the
// disk, which open for reading and are never a project.
func errBroadFolder(dir string) error {
	return fmt.Errorf("%s is not a project folder. Open the folder that holds your content before you keep rules or connect an assistant", dir)
}

// ensureRecipe writes the tab's recipe into its folder, if it is not there
// yet, and registers the project in the workspace. It is idempotent: a tab
// whose recipe is on disk is left alone. The caller must not hold a.mu; one
// that does calls ensureRecipeLocked.
//
// The file written is the draft `kapi init` would have written, comments
// included, with whatever the person has changed in memory since the folder
// opened saved over it. A kapi.yaml that appeared in the folder meanwhile (a
// `kapi init` in a terminal) is kept as it is.
func (a *App) ensureRecipe(op *openProject) error {
	if op == nil || !op.unsaved.Load() {
		return nil
	}
	a.mu.RLock()
	written, err := a.writeDraft(op)
	a.mu.RUnlock()
	if err != nil || !written {
		return err
	}
	a.adoptRecipe(op, filepath.Dir(op.Path))
	return nil
}

// ensureRecipeLocked is ensureRecipe for a caller holding a.mu.
func (a *App) ensureRecipeLocked(op *openProject) error {
	if op == nil || !op.unsaved.Load() {
		return nil
	}
	written, err := a.writeDraft(op)
	if err != nil || !written {
		return err
	}
	a.adoptRecipe(op, filepath.Dir(op.Path))
	return nil
}

// writeDraft puts an unsaved tab's recipe on disk and reports whether this call
// did. The caller holds a.mu, for reading at least, so the recipe it saves is
// not replaced underneath it.
func (a *App) writeDraft(op *openProject) (bool, error) {
	if op.contextOnly || op.Path == "" {
		return false, nil
	}
	op.recipeMu.Lock()
	defer op.recipeMu.Unlock()
	if !op.unsaved.Load() {
		return false, nil
	}
	dir := filepath.Dir(op.Path)
	if broadFolder(dir) {
		return false, errBroadFolder(dir)
	}
	f, err := os.OpenFile(op.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	switch {
	case errors.Is(err, fs.ErrExist):
		// Someone else wrote one. It is theirs, so it is not overwritten.
	case err != nil:
		return false, fmt.Errorf("write %s: %w", project.RecipeFileName, err)
	default:
		_, werr := f.Write(op.draft)
		cerr := f.Close()
		if werr = errors.Join(werr, cerr); werr != nil {
			_ = os.Remove(op.Path)
			return false, fmt.Errorf("write %s: %w", project.RecipeFileName, werr)
		}
		if op.Project != nil {
			if err := project.Save(op.Path, op.Project); err != nil {
				return false, err
			}
		}
	}
	if err := project.EnsureLayout(project.LayoutAt(dir)); err != nil {
		a.logger.Printf("create %s for %s: %v", project.StateDirName, op.Path, err)
	}
	op.unsaved.Store(false)
	op.draft = nil
	return true, nil
}

// adoptRecipe moves a tab whose recipe has just been written from its draft
// store to the store in the folder. draftRoot is the folder the draft was
// opened on, which a save to another folder leaves behind.
//
// The draft's block cache is dropped and rebuilt in the folder's `.kapi/`. The
// context is kept: the draft bound the workspace context under the id the
// recipe now carries, and the store opened here binds the same one. Opening it
// also registers the checkout, under that id and no other.
func (a *App) adoptRecipe(op *openProject, draftRoot string) {
	if op.memoryHandle != "" {
		a.memoryHandles.Close(op.memoryHandle)
		op.memoryHandle = ""
	}
	if op.tbHandle != "" {
		a.tbHandles.Close(op.tbHandle)
		op.tbHandle = ""
	}
	if err := a.hostEngine().UnbindDraftProject(draftRoot); err != nil {
		a.logger.Printf("close the draft store for %s: %v", draftRoot, err)
	}
	root := filepath.Dir(op.Path)
	if _, err := a.hostEngine().ProjectDB(context.Background(), root); err != nil {
		a.logger.Printf("open the project store for %s: %v", root, err)
	}
	a.autoOpenProjectResources(op)
	a.registerInWorkspace(op)
	a.emitEvent("project:recipe-written", map[string]any{"tabID": op.ID, "path": op.Path})
}

// saveRecipe writes a tab's recipe, first giving an unsaved folder its
// kapi.yaml. The recipe-editing methods call it in place of project.Save, with
// a.mu held.
func (a *App) saveRecipe(op *openProject) error {
	if err := a.ensureRecipeLocked(op); err != nil {
		return err
	}
	return project.Save(op.Path, op.Project)
}

// ensureRecipeForKey writes the recipe of every open tab of one workspace
// project that has none on disk yet. The context operations name a project by
// its workspace key rather than by tab.
func (a *App) ensureRecipeForKey(key workspace.ProjectKey) error {
	a.mu.RLock()
	var pending []*openProject
	for _, op := range a.projects {
		if op.workspaceKey == key && op.unsaved.Load() {
			pending = append(pending, op)
		}
	}
	a.mu.RUnlock()
	var errs []error
	for _, op := range pending {
		errs = append(errs, a.ensureRecipe(op))
	}
	return errors.Join(errs...)
}

// tabInfoOf describes an open tab to the frontend.
func tabInfoOf(op *openProject) TabInfo {
	if op.contextOnly {
		return TabInfo{ID: op.ID, Name: op.contextName, ContextOnly: true, RecipeWritten: true}
	}
	return TabInfo{
		ID:            op.ID,
		Name:          projectDisplayName(op.Project, op.Path),
		Path:          op.Path,
		RecipeWritten: !op.unsaved.Load(),
	}
}

// sameRecipe reports whether two recipes say the same thing, as the frontend
// sees them. UpdateProject receives the recipe back through JSON, so the
// comparison is made in that form.
func sameRecipe(a, b *project.KapiProject) bool {
	ja, erra := json.Marshal(a)
	jb, errb := json.Marshal(b)
	return erra == nil && errb == nil && bytes.Equal(ja, jb)
}
