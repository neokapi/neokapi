package host

import (
	"path/filepath"
	"sync"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/model"
)

// withFix gives d the change operation that applies its finding's replacement
// to b, the block of file the finding was raised on (check.Fix), so `kapi
// check --json` hands kapi apply an operation it carries as it is. The fix
// addresses the block's own edition, written in source, under the revision the
// check read, and names the document as docs says the change service resolves
// it. A code comment is written through the comment path, which takes no such
// operation, and standard input is no document a change set can name; neither
// gets a fix, and nor does a check with no docs (the commit check, which
// reports findings and offers no fix).
func withFix(d check.Diagnostic, f check.Finding, b *model.Block, file string, source model.LocaleID, docs *fixDocs) check.Diagnostic {
	if docs == nil || file == "" || file == StdinName || comment.IsBlock(b) {
		return d
	}
	op := check.Fix(f, b.SourceRuns(), change.Ref{Block: blockKey(b)}, check.SourceRevision(b, source))
	if op == nil {
		return d
	}
	doc, ok := docs.doc(file)
	if !ok {
		return d
	}
	op.At.Doc = doc
	d.Fix = op
	return d
}

// fixDocs names the document a check's fix addresses, the way the change
// service that applies the fix resolves one. Inside a project that is the
// file's path relative to the project root, whichever directory the check ran
// in, and a file outside the project is named by its absolute path, as kapi
// apply edits one. Outside a project it is the path the check was given, which
// kapi apply resolves from the same working directory.
//
// The file of a translation gets no fix. The change service reads it as that
// translation of its source, while the check held its words to the rules of
// the source language and read them as source; a replacement from those rules
// is no fix for a translation, and a revision of the source edition never
// matches the translation it would guard.
type fixDocs struct {
	app    *App
	recipe string

	once   sync.Once
	layout *projectChangeLayout
	root   string
	// kinds caches what each project-relative path is: true for a file a fix
	// names, false for the file of a translation.
	mu    sync.Mutex
	kinds map[string]bool
}

// newFixDocs names documents for the fixes of a check in the project at
// recipe, or outside a project when recipe is empty.
func (a *App) newFixDocs(recipe string) *fixDocs {
	return &fixDocs{app: a, recipe: recipe, kinds: map[string]bool{}}
}

// doc is the reference a fix names file by, and false for a file no fix
// names.
func (d *fixDocs) doc(file string) (string, bool) {
	if d.recipe == "" {
		return DisplayName(file), true
	}
	d.once.Do(func() {
		l, err := d.app.newProjectLayout(ChangeServiceOptions{Project: d.recipe})
		if err != nil {
			return
		}
		d.layout, d.root = l, l.root
	})
	if d.layout == nil {
		return "", false
	}
	rel, inside := projectRelPath(d.root, file)
	if !inside {
		abs, err := filepath.Abs(file)
		if err != nil {
			return "", false
		}
		return filepath.ToSlash(abs), true
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	named, seen := d.kinds[rel]
	if !seen {
		named = d.names(rel)
		d.kinds[rel] = named
	}
	return rel, named
}

// names reports whether a fix names rel, a project-relative path: a file the
// recipe claims as a source does, the file of a translation does not, and any
// other file does, read as the change service reads a file the recipe does
// not claim.
func (d *fixDocs) names(rel string) bool {
	if _, ok := d.layout.sourceFile(rel); ok {
		return true
	}
	_, translation, err := d.layout.translationFile(rel)
	return err == nil && !translation
}
