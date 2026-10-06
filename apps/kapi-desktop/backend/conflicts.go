package backend

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host"
)

// Kapi Desktop shows the conflicts `kapi status` lists and lets a person
// decide them, block by block. GetKeptConflicts reads each with both
// wordings; a decision is a change set sent through Apply, guarded by the
// revision the conflict shows as held:
//
//   - for an edit a merge left unapplied, a set_content of the held wording,
//     the other wording, or a new one, to the translation of the source
//     document; the workspace home records each as the person's decision;
//   - for wording the workspace keeps that the translation's file does not
//     hold, a set_content to the file when the person takes the kept wording
//     or writes a new one, followed by ReleaseKeptWording, which drops the
//     workspace's copy; keeping the file's wording is the release alone;
//   - for a write to a whole document of a KPZ in the project that did not
//     land, RebaseKeptDocument, which carries the write's changes over onto
//     the document's head and leaves the blocks the head changed too, each
//     decided as an edit is, or DiscardKeptDocument, which keeps the head.

// KeptConflict is one conflict of a project, as the desktop shows it.
type KeptConflict struct {
	// Kind is "edit" (an edit another machine made that did not land),
	// "file" (wording the translation's file does not hold) or "document"
	// (a version of a KPZ's document, written from an older head, that did
	// not land).
	Kind string `json:"kind"`
	// Doc is the source document an operation names, Locale the
	// translation's language.
	Doc    string `json:"doc"`
	Locale string `json:"locale"`
	// Edit is the edit that did not land; File the translation's file.
	Edit string `json:"edit,omitempty"`
	File string `json:"file,omitempty"`
	// Rebased says a "document" write has been rebased onto the document's
	// head; until then it lists no blocks.
	Rebased bool                `json:"rebased,omitempty"`
	Blocks  []KeptConflictBlock `json:"blocks"`
}

// KeptConflictBlock is one block of a conflict with both wordings.
type KeptConflictBlock struct {
	Block string `json:"block"`
	// Edition, for a "document" block, is the contested edition, empty for
	// the document's own; a decision names it in place of the locale.
	Edition string `json:"edition,omitempty"`
	Source  string `json:"source"`
	// Held is what the translation's home holds now, with the revision a
	// decision names; Other the wording that did not land.
	Held  KeptWording `json:"held"`
	Other KeptWording `json:"other"`
}

// KeptWording is one wording of a contested block, in placeholder form.
type KeptWording struct {
	Text   string `json:"text"`
	Rev    string `json:"rev"`
	Absent bool   `json:"absent,omitempty"`
}

// GetKeptConflicts lists the conflicts of the project a tab has open, with
// both wordings of each block concerned.
func (a *App) GetKeptConflicts(tabID string) ([]KeptConflict, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil || op.Path == "" {
		return []KeptConflict{}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), changeTimeout)
	defer cancel()
	found, err := a.hostEngine().KeptConflicts(ctx, op.Path)
	if err != nil {
		return nil, err
	}
	return keptConflictsFrom(found), nil
}

// keptConflictsFrom is the host's conflicts as the desktop shows them.
func keptConflictsFrom(found []host.KeptConflict) []KeptConflict {
	out := make([]KeptConflict, 0, len(found))
	for _, c := range found {
		kc := KeptConflict{Kind: c.Kind, Doc: c.Doc, Locale: c.Locale, Edit: c.Edit, File: c.File, Rebased: c.Rebased,
			Blocks: make([]KeptConflictBlock, 0, len(c.Blocks))}
		for _, b := range c.Blocks {
			kc.Blocks = append(kc.Blocks, KeptConflictBlock{Block: b.Block, Edition: b.Edition, Source: b.Source,
				Held:  KeptWording{Text: b.Held.Text, Rev: b.Held.Rev, Absent: b.Held.Absent},
				Other: KeptWording{Text: b.Other.Text, Rev: b.Other.Rev, Absent: b.Other.Absent}})
		}
		out = append(out, kc)
	}
	return out
}

// ReleaseKeptWording drops what the workspace keeps of blocks of the locale
// translation of doc, once a person has decided what the translation's file
// holds. The file is the translation's home, so the workspace keeps no copy
// beside it.
func (a *App) ReleaseKeptWording(tabID, doc, locale string, blocks []string) error {
	op := a.getOpenProject(tabID)
	if op == nil {
		return fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil || op.Path == "" {
		return errors.New("project has no recipe loaded")
	}
	ctx, cancel := context.WithTimeout(context.Background(), changeTimeout)
	defer cancel()
	return a.hostEngine().ReleaseKeptWording(ctx, op.Path, doc, model.LocaleID(locale), blocks, "desktop")
}

// DocumentRebase is what a rebase of a document's divergent write did.
type DocumentRebase struct {
	// Carried counts the changes applied to the document's head, Contested
	// the blocks the head changed too, which the conflict now lists.
	Carried   int `json:"carried"`
	Contested int `json:"contested"`
	// Refused says why the change service refused the changes; nothing was
	// settled then.
	Refused string `json:"refused,omitempty"`
}

// RebaseKeptDocument carries the write edit to doc, a document of a KPZ in
// the project, over onto the document's head through the change service,
// and leaves the blocks the head changed too as the conflict's blocks.
func (a *App) RebaseKeptDocument(tabID, doc, edit string) (DocumentRebase, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return DocumentRebase{}, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil || op.Path == "" {
		return DocumentRebase{}, errors.New("project has no recipe loaded")
	}
	ctx, cancel := context.WithTimeout(context.Background(), changeTimeout)
	defer cancel()
	rb, err := a.hostEngine().RebaseKeptDocument(ctx, op.Path, doc, edit, "desktop")
	if err != nil {
		return DocumentRebase{}, err
	}
	return DocumentRebase{Carried: rb.Carried, Contested: rb.Contested, Refused: rb.Refused}, nil
}

// DiscardKeptDocument drops the write edit to doc, a document of a KPZ in
// the project, and keeps the document as its head holds it.
func (a *App) DiscardKeptDocument(tabID, doc, edit string) error {
	op := a.getOpenProject(tabID)
	if op == nil {
		return fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil || op.Path == "" {
		return errors.New("project has no recipe loaded")
	}
	ctx, cancel := context.WithTimeout(context.Background(), changeTimeout)
	defer cancel()
	return a.hostEngine().DiscardKeptDocument(ctx, op.Path, doc, edit, "desktop")
}

// The workspace home lists the same "document" conflicts for every .kpz on
// this machine whose working cache holds edits, with no project open. The
// caches are this user's and keyed by each .kpz's absolute path, so each
// document is named by that path (/work/guide.kpz!guide.md), and the change
// service that settles it is the one of the project the .kpz sits in, or of
// its directory when it sits in none.

// GetWorkspaceDocumentConflicts lists the versions of a .kpz's document that
// did not land, for every .kpz on this machine.
func (a *App) GetWorkspaceDocumentConflicts() ([]KeptConflict, error) {
	ctx, cancel := context.WithTimeout(context.Background(), changeTimeout)
	defer cancel()
	found, err := a.hostEngine().WorkspaceDocumentConflicts(ctx)
	if err != nil {
		return nil, err
	}
	return keptConflictsFrom(found), nil
}

// RebaseWorkspaceDocument rebases the write edit to doc, a .kpz's document
// named by the .kpz's absolute path, onto the document's head.
func (a *App) RebaseWorkspaceDocument(doc, edit string) (DocumentRebase, error) {
	ctx, cancel := context.WithTimeout(context.Background(), changeTimeout)
	defer cancel()
	rb, err := a.hostEngine().RebaseKeptDocument(ctx, "", doc, edit, "desktop")
	if err != nil {
		return DocumentRebase{}, err
	}
	return DocumentRebase{Carried: rb.Carried, Contested: rb.Contested, Refused: rb.Refused}, nil
}

// DiscardWorkspaceDocument drops the write edit to doc, a .kpz's document
// named by the .kpz's absolute path, and keeps the document as it stands.
func (a *App) DiscardWorkspaceDocument(doc, edit string) error {
	ctx, cancel := context.WithTimeout(context.Background(), changeTimeout)
	defer cancel()
	return a.hostEngine().DiscardKeptDocument(ctx, "", doc, edit, "desktop")
}

// ApplyWorkspaceDocument applies a change set to one .kpz's document named
// by the .kpz's absolute path, as the person using the desktop, and returns
// the result (kapi.change-result/v1): how the workspace home decides a block a
// rebase left. Every content operation names that one document.
func (a *App) ApplyWorkspaceDocument(changeSet string) (string, error) {
	set, err := change.Decode(strings.NewReader(changeSet))
	if err != nil {
		if ce, ok := errors.AsType[*change.Error](err); ok && ce != nil {
			return encodeChange(change.ErrorResult(ce))
		}
		return encodeChange(change.ErrorResult(&change.Error{Code: change.CodeInvalid, Message: err.Error()}))
	}
	doc := ""
	for _, o := range set.Ops {
		switch {
		case doc == "":
			doc = o.At.Doc
		case o.At.Doc != doc:
			return encodeChange(change.ErrorResult(&change.Error{Code: change.CodeInvalid, Field: "ops/at/doc",
				Message: "a change set from the workspace home edits one document of a .kpz"}))
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), changeTimeout)
	defer cancel()
	svc, err := a.hostEngine().KpzDocumentService(ctx, doc, "desktop")
	if errors.Is(err, host.ErrNotKpzDocument) {
		return encodeChange(change.ErrorResult(&change.Error{Code: change.CodeNotFound, Field: "ops/at/doc", Message: err.Error()}))
	}
	if err != nil {
		return "", err
	}
	res, err := svc.Apply(ctx, set, desktopActor)
	if err != nil {
		if ce, ok := errors.AsType[*change.Error](err); ok && ce != nil {
			return encodeChange(change.ErrorResult(ce))
		}
		return "", err
	}
	return encodeChange(res)
}
