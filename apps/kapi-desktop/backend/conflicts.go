package backend

import (
	"context"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
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
//     workspace's copy; keeping the file's wording is the release alone.

// KeptConflict is one conflict of a project, as the desktop shows it.
type KeptConflict struct {
	// Kind is "edit" (an edit another machine made that did not land) or
	// "file" (wording the translation's file does not hold).
	Kind string `json:"kind"`
	// Doc is the source document an operation names, Locale the
	// translation's language.
	Doc    string `json:"doc"`
	Locale string `json:"locale"`
	// Edit is the edit that did not land; File the translation's file.
	Edit   string              `json:"edit,omitempty"`
	File   string              `json:"file,omitempty"`
	Blocks []KeptConflictBlock `json:"blocks"`
}

// KeptConflictBlock is one block of a conflict with both wordings.
type KeptConflictBlock struct {
	Block  string `json:"block"`
	Source string `json:"source"`
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
	out := make([]KeptConflict, 0, len(found))
	for _, c := range found {
		kc := KeptConflict{Kind: c.Kind, Doc: c.Doc, Locale: c.Locale, Edit: c.Edit, File: c.File,
			Blocks: make([]KeptConflictBlock, 0, len(c.Blocks))}
		for _, b := range c.Blocks {
			kc.Blocks = append(kc.Blocks, KeptConflictBlock{Block: b.Block, Source: b.Source,
				Held:  KeptWording{Text: b.Held.Text, Rev: b.Held.Rev, Absent: b.Held.Absent},
				Other: KeptWording{Text: b.Other.Text, Rev: b.Other.Rev, Absent: b.Other.Absent}})
		}
		out = append(out, kc)
	}
	return out, nil
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
