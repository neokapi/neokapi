package kpz

import (
	"fmt"
	"path"
	"strings"

	"github.com/neokapi/neokapi/core/safeio"
)

// A workspace package carries a workspace that has no other home: the files
// of a file system, and the context of every project among them, each as a
// context package of its own (KindContext). The browser engine writes one, so
// a person can keep what a page holds somewhere other than the browser that
// holds it, and reads one back into a page.
//
// The contexts are the operation logs, never the stores: a project's stores
// are projections of its log, and reading the package back rebuilds them by
// merging each log the way `kapi context import` does.

// FilesDir is the archive directory holding a workspace package's files, at
// their paths relative to the root the package was written from.
const FilesDir = "files/"

// ContextsDir is the archive directory holding a workspace package's context
// packages, one per project.
const ContextsDir = "contexts/"

// FileDoc is one file of a workspace package.
type FileDoc struct {
	// Path is the archive path under files/, e.g. "files/project/kapi.yaml".
	Path string
	// Content is the file's bytes, carried as a reference.
	Content Content
}

// ContextDoc is one project's context in a workspace package.
type ContextDoc struct {
	// Path is the archive path under contexts/, e.g. "contexts/1.kpz".
	Path string
	// Project is the project's root, relative to the root the package was
	// written from, "." for the root itself.
	Project string
	// Data is the project's context package (KindContext), as its bytes.
	Data []byte
}

// WorkspaceProject is one project a workspace package carries a context for,
// as the manifest records it.
type WorkspaceProject struct {
	// Root is the project's root, relative to the package's root.
	Root string `json:"root"`
	// Context is the archive path of the project's context package.
	Context string `json:"context"`
}

// FilePath returns the archive path of the file at rel, a slash-separated
// path relative to the package's root.
func FilePath(rel string) string { return FilesDir + strings.TrimPrefix(path.Clean(rel), "/") }

// FileRel returns the path of a file member relative to the package's root.
func FileRel(member string) string { return strings.TrimPrefix(member, FilesDir) }

// underDir reports whether a member path is a clean path below dir, and so
// stays inside the archive and inside dir.
func underDir(p, dir string) bool {
	return safeio.IsLocalPath(p) && path.Clean(p) == p && strings.HasPrefix(p, dir) && len(p) > len(dir)
}

// workspaceProjects lists the projects of a package's contexts for its
// manifest, refusing a context outside contexts/ or a root outside the
// package.
func workspaceProjects(contexts []ContextDoc) ([]WorkspaceProject, error) {
	var out []WorkspaceProject
	for _, c := range contexts {
		if !underDir(c.Path, ContextsDir) {
			return nil, fmt.Errorf("kpz: %q is not a path under %s", c.Path, ContextsDir)
		}
		if !safeio.IsLocalPath(c.Project) {
			return nil, fmt.Errorf("kpz: project root %q is not a path inside the workspace", c.Project)
		}
		out = append(out, WorkspaceProject{Root: c.Project, Context: c.Path})
	}
	return out, nil
}

// contextMember reads one context member of a workspace package, with the
// project root the manifest records for it.
func contextMember(member string, data []byte, roots map[string]string) (ContextDoc, error) {
	if !underDir(member, ContextsDir) {
		return ContextDoc{}, fmt.Errorf("kpz: %q is not a path under %s", member, ContextsDir)
	}
	root, ok := roots[member]
	if !ok {
		return ContextDoc{}, fmt.Errorf("kpz: the manifest names no project for %q", member)
	}
	return ContextDoc{Path: member, Project: root, Data: data}, nil
}
