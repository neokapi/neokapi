package host

import (
	"context"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/container"
	"github.com/neokapi/neokapi/core/model"
)

// commandChanges is the change service a command line reads and edits the
// files it is given through. Inside a project a file under the project's root
// is a document of the project, named by its project-relative path and read
// with the format and configuration the recipe binds, so the references a
// read prints are the ones kapi apply resolves. Any other file is named by its
// path from the working directory, or by its absolute path when a project is
// in force, and read with the format detection finds; kapi apply in the
// project edits a document named by an absolute path outside the project the
// same way.
type commandChanges struct {
	app    *App
	cmd    Command
	recipe string
	opts   ChangeServiceOptions

	project, dir *commandService
}

// commandService is one service of a command line, with the home it edits.
type commandService struct {
	home changeHome
	svc  *change.Service
}

// newCommandChanges builds the services lazily; opts sets what every service
// shares (origin, format, backup, output). recipe names the project, "" none.
func (a *App) newCommandChanges(cmd Command, recipe string, opts ChangeServiceOptions) *commandChanges {
	return &commandChanges{app: a, cmd: cmd, recipe: recipe, opts: opts}
}

// For returns the service that holds the file at path, and the document
// reference that names it there. path may be a container!entry locator.
func (c *commandChanges) For(ctx context.Context, path string) (*change.Service, string, error) {
	cs, doc, _, err := c.forFile(ctx, path)
	if err != nil {
		return nil, "", err
	}
	return cs.svc, doc, nil
}

// forFile is For, reporting whether the file is a document of the project.
func (c *commandChanges) forFile(ctx context.Context, path string) (*commandService, string, bool, error) {
	file, member := splitLocator(path)
	abs, err := filepath.Abs(file)
	if err != nil {
		return nil, "", false, err
	}
	if c.recipe != "" {
		root := filepath.Dir(c.recipe)
		if rel, rerr := filepath.Rel(root, abs); rerr == nil && filepath.IsLocal(rel) {
			if c.project == nil {
				opts := c.opts
				opts.Project = c.recipe
				if c.project, err = c.build(ctx, opts); err != nil {
					return nil, "", false, err
				}
			}
			return c.project, withMember(filepath.ToSlash(rel), member), true, nil
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		return nil, "", false, err
	}
	// In a project, the files outside it are resolved from the project's
	// root, so none lies under the root and each keeps its absolute path as
	// its reference, whatever the working directory.
	root, ref := wd, relRef(wd, abs)
	if c.recipe != "" {
		root, ref = filepath.Dir(c.recipe), filepath.ToSlash(abs)
	}
	if c.dir == nil {
		opts := c.opts
		opts.Project, opts.Root, opts.AnyPath = "", root, true
		if c.dir, err = c.build(ctx, opts); err != nil {
			return nil, "", false, err
		}
	}
	return c.dir, withMember(ref, member), false, nil
}

func (c *commandChanges) build(ctx context.Context, opts ChangeServiceOptions) (*commandService, error) {
	h, err := c.app.changeHome(opts)
	if err != nil {
		return nil, err
	}
	svc, err := c.app.serviceOver(ctx, c.cmd, opts, h)
	if err != nil {
		return nil, err
	}
	return &commandService{home: h, svc: svc}, nil
}

// copyService returns a service that applies changes to the file at copy in
// place of the file path names (an archive for a container!entry locator),
// with every document located and read as For reads it, so a document is
// named there by the reference For gives it. A command that prints an edited
// document applies the change to a private copy this way and leaves the file
// as it is. The copy's locks live under lockDir, and the service runs no
// project's hooks: the copy belongs to the command alone.
func (c *commandChanges) copyService(ctx context.Context, path, copy, lockDir string) (*change.Service, error) {
	cs, _, _, err := c.forFile(ctx, path)
	if err != nil {
		return nil, err
	}
	file, _ := splitLocator(path)
	abs, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}
	opts := c.opts
	opts.Project, opts.BackupSuffix = "", ""
	h := changeHome{layout: copyLayout{inner: cs.home.layout, file: abs, copy: copy}, lockDir: lockDir}
	return c.app.serviceOver(ctx, c.cmd, opts, h)
}

// copyLayout locates documents as inner does, with the file at file read and
// written at copy in its place. Any other file a document reaches is read
// where it lies and never written: an edition kept in a file of its own other
// than file has no place here.
type copyLayout struct {
	inner      filehome.Layout
	file, copy string
}

func (l copyLayout) Locate(ctx context.Context, doc string) (filehome.Doc, error) {
	d, err := l.inner.Locate(ctx, doc)
	if err != nil {
		return d, err
	}
	if samePath(d.Path, l.file) {
		d.Path = l.copy
	}
	if edition := d.EditionFile; edition != nil {
		d.EditionFile = func(k model.EditionKey) (filehome.EditionFile, bool) {
			f, ok := edition(k)
			if !ok || !samePath(f.Path, l.file) {
				return filehome.EditionFile{}, false
			}
			// The copy holds the edition, whatever home the edition has in
			// the project: a preview writes nothing anywhere else.
			f.Path, f.Kept = l.copy, nil
			return f, true
		}
	}
	d.Derived = nil
	return d, nil
}

// samePath reports whether two paths name one file.
func samePath(a, b string) bool {
	x, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	y, err := filepath.Abs(b)
	if err != nil {
		return false
	}
	return x == y
}

// splitLocator splits a container!entry locator into the archive and the
// member; any other path is the file itself.
func splitLocator(path string) (file, member string) {
	if loc, ok := parseEntryLocator(path); ok {
		return loc.Archive, loc.Entry
	}
	return path, ""
}

func withMember(doc, member string) string {
	if member == "" {
		return doc
	}
	return doc + "!" + member
}

// stdinDocument copies standard input into a private directory, so a command
// that reads a document from a pipe reads it through the change service like
// any file. It returns the copy's path and a function that removes it.
func stdinDocument(ctx context.Context) (string, func(), error) {
	data, err := readContent(ctx, StdinName)
	if err != nil {
		return "", nil, err
	}
	dir, err := os.MkdirTemp("", "kapi-stdin-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	// The copy has no extension, so its format is the one --format names or
	// detection finds in its content, as it is for a pipe.
	path := filepath.Join(dir, "stdin")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		cleanup()
		return "", nil, err
	}
	return path, cleanup, nil
}

// isContainer reports whether path names a whole archive, as opposed to one
// of its members.
func isContainer(path string) bool {
	if _, ok := parseEntryLocator(path); ok {
		return false
	}
	return container.IsContainerPath(path)
}

// containerMembers lists the members of the archive at path that a format
// reads and writes, as container!entry locators, in the archive's order.
func (a *App) containerMembers(path string) ([]string, error) {
	_, entries, err := container.ListEntries(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if _, eligible := a.containerEntryFormat(e.Name); eligible {
			out = append(out, path+"!"+e.Name)
		}
	}
	return out, nil
}
