package host

import (
	"context"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/container"
)

// commandChanges is the change service a command line reads and edits the
// files it is given through. Inside a project a file under the project's root
// is a document of the project, named by its project-relative path and read
// with the format and configuration the recipe binds, so the references a
// read prints are the ones kapi apply resolves. Any other file, and every file
// outside a project, is named by its path from the working directory and read
// with the format detection finds.
type commandChanges struct {
	app    *App
	cmd    Command
	recipe string
	opts   ChangeServiceOptions

	project, dir *change.Service
}

// newCommandChanges builds the services lazily; opts sets what every service
// shares (origin, format, backup, output). recipe names the project, "" none.
func (a *App) newCommandChanges(cmd Command, recipe string, opts ChangeServiceOptions) *commandChanges {
	return &commandChanges{app: a, cmd: cmd, recipe: recipe, opts: opts}
}

// For returns the service that holds the file at path, and the document
// reference that names it there. path may be a container!entry locator.
func (c *commandChanges) For(ctx context.Context, path string) (*change.Service, string, error) {
	svc, doc, _, err := c.forFile(ctx, path)
	return svc, doc, err
}

// forFile is For, reporting whether the file is a document of the project.
func (c *commandChanges) forFile(ctx context.Context, path string) (*change.Service, string, bool, error) {
	file, member := path, ""
	if loc, ok := parseEntryLocator(path); ok {
		file, member = loc.Archive, loc.Entry
	}
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
				if c.project, err = c.app.changeService(ctx, c.cmd, opts); err != nil {
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
	if c.dir == nil {
		opts := c.opts
		opts.Project, opts.Root, opts.AnyPath = "", wd, true
		if c.dir, err = c.app.changeService(ctx, c.cmd, opts); err != nil {
			return nil, "", false, err
		}
	}
	return c.dir, withMember(relRef(wd, abs), member), false, nil
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
