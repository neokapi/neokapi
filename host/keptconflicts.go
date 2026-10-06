package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/workhome"
)

// The conflicts kapi status lists are decided by a person, block by block,
// through the change service. A surface that shows them (Kapi Desktop) reads
// each with both wordings: what the edition's home holds now, which an
// operation names by its revision, and the wording that did not land.
//
//   - An edit a merge left unapplied: the workspace home holds one wording
//     and the other machine's edit holds another. A set_content of either
//     wording, or of a new one, guarded by the held revision, decides the
//     block: the workspace home records a person keeping the held wording
//     as a write too (change.ContestedSession).
//   - Wording the workspace keeps that the translation's file, which exists
//     now, does not hold: the file is the edition's home. A set_content to
//     the file writes the kept wording or a new one into it, and
//     ReleaseKeptWording then drops the workspace's copy; keeping the file's
//     wording is the release alone.

// Conflict kinds.
const (
	// ConflictEdit is an edit two machines made from one version of a kept
	// draft that a pull merged and that could not land.
	ConflictEdit = "edit"
	// ConflictFile is wording a person or an agent wrote into a kept draft
	// whose file has appeared since without it.
	ConflictFile = "file"
)

// A third kind, ConflictDocument, is a write to a whole document a KPZ in the
// project carries that did not land (kpzdivergence.go). Until a person
// rebases it, it lists no blocks: Rebased is false, and the choice is to
// rebase the write onto the document's head or to discard it. A rebase lists
// the blocks the head changed too, each decided as an edit is.

// KeptConflict is one conflict of a project, with both wordings of each
// block concerned.
type KeptConflict struct {
	// Kind is ConflictEdit, ConflictFile or ConflictDocument.
	Kind string `json:"kind"`
	// Doc is the source document and Locale the translation's language.
	Doc    string `json:"doc"`
	Locale string `json:"locale"`
	// Edit is the recorded edit that did not land, for ConflictEdit and
	// ConflictDocument; File the translation's file, for ConflictFile.
	Edit string `json:"edit,omitempty"`
	File string `json:"file,omitempty"`
	// Rebased says a rebase has carried a ConflictDocument write over onto
	// the document's head, leaving Blocks for a person to decide.
	Rebased bool                `json:"rebased,omitempty"`
	Blocks  []KeptConflictBlock `json:"blocks"`
}

// KeptConflictBlock is one block of a conflict.
type KeptConflictBlock struct {
	// Block is the block's key; an operation names it with Doc and Locale.
	Block string `json:"block"`
	// Edition, for a ConflictDocument block, is the edition of the block
	// that is contested, empty for the document's own; an operation names it
	// in place of Locale.
	Edition string `json:"edition,omitempty"`
	// Source is the block's own text, for context.
	Source string `json:"source"`
	// Held is the wording the edition's home holds now, with the revision an
	// operation that decides the block names; Absent when it holds none.
	Held KeptWording `json:"held"`
	// Other is the wording that did not land: the other machine's edit, or
	// what the workspace keeps that the file does not hold.
	Other KeptWording `json:"other"`
}

// KeptWording is one wording of a contested block, as edit text with the
// placeholders a read shows.
type KeptWording struct {
	Text   string `json:"text"`
	Rev    string `json:"rev"`
	Absent bool   `json:"absent,omitempty"`
}

// KeptConflicts lists the conflicts of the project at recipe with both
// wordings of every block concerned: what kapi status lists, for a surface
// where a person decides them.
func (a *App) KeptConflicts(ctx context.Context, recipe string) ([]KeptConflict, error) {
	root := filepath.Dir(recipe)
	documents, err := a.kpzDocumentConflicts(ctx, recipe)
	if err != nil {
		return nil, err
	}
	h, err := a.keptEditions(root).open(ctx, false)
	if err != nil || h == nil {
		return documents, err
	}
	out := documents
	conflicts, err := h.Conflicts(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range conflicts {
		key, err := model.ParseEditionKey(c.Edition)
		if err != nil || key.Locale == "" {
			continue
		}
		doc := c.Path
		if doc == "" {
			doc = c.Doc
		}
		other, err := divergentWording(ctx, h, c)
		if err != nil {
			return nil, err
		}
		kc := KeptConflict{Kind: ConflictEdit, Doc: doc, Locale: string(key.Locale), Edit: c.Op}
		if kc.Blocks, err = a.conflictBlocks(ctx, recipe, doc, key, c.Blocks, other); err != nil {
			return nil, err
		}
		out = append(out, kc)
	}
	beside, err := a.keptBesideFiles(ctx, recipe)
	if err != nil {
		return nil, err
	}
	for _, bf := range beside {
		if len(bf.kept) == 0 {
			continue
		}
		other := map[string]KeptWording{}
		for _, block := range bf.kept {
			r := bf.held.Rows[block]
			ed, err := h.EditionOf(ctx, r)
			if err != nil {
				return nil, err
			}
			other[block] = KeptWording{Text: model.RunsEditText(ed.Runs), Rev: r.Rev}
		}
		kc := KeptConflict{Kind: ConflictFile, Doc: bf.path, Locale: string(bf.edition.Locale), File: bf.file}
		if kc.Blocks, err = a.conflictBlocks(ctx, recipe, bf.path, bf.edition, bf.kept, other); err != nil {
			return nil, err
		}
		out = append(out, kc)
	}
	return out, nil
}

// divergentWording reads, from the log, the wording a write that did not land
// gave each block it changed.
func divergentWording(ctx context.Context, h *workhome.Home, c workhome.Conflict) (map[string]KeptWording, error) {
	writes, err := h.Log.Writes(ctx, c.Doc, c.Edition)
	if err != nil {
		return nil, err
	}
	at := slices.IndexFunc(writes, func(w workhome.Write) bool { return w.Op == c.Op })
	out := map[string]KeptWording{}
	if at < 0 {
		return out, nil
	}
	for _, b := range writes[at].Blocks {
		if b.After == model.AbsentRevision {
			out[b.Block] = KeptWording{Rev: model.AbsentRevision, Absent: true}
			continue
		}
		data := b.Runs
		if data == nil && b.Ref != "" {
			if data, err = h.Log.Blob(ctx, b.Ref); err != nil {
				return nil, fmt.Errorf("read the edit %s: %w", c.Op, err)
			}
		}
		var runs []model.Run
		if len(data) > 0 {
			if err := json.Unmarshal(data, &runs); err != nil {
				return nil, fmt.Errorf("read the edit %s: %w", c.Op, err)
			}
		}
		out[b.Block] = KeptWording{Text: model.RunsEditText(runs), Rev: b.After}
	}
	return out, nil
}

// conflictBlocks reads each block of a conflict as the edition's home holds
// it now, through the change service, beside the wording that did not land.
func (a *App) conflictBlocks(ctx context.Context, recipe, doc string, key model.EditionKey, blocks []string, other map[string]KeptWording) ([]KeptConflictBlock, error) {
	svc, err := a.ChangeService(ctx, ChangeServiceOptions{Project: recipe, Origin: "status", TargetLocale: key.Locale})
	if err != nil {
		return nil, err
	}
	page, err := svc.Read(ctx, change.ReadRequest{Doc: doc, Blocks: blocks, Editions: []model.EditionKey{key}, Limit: change.MaxReadLimit})
	if err != nil {
		return nil, err
	}
	text, _ := key.MarshalText()
	read := map[string]change.BlockRead{}
	for _, b := range page.Blocks {
		read[b.Ref.Block] = b
	}
	out := make([]KeptConflictBlock, 0, len(blocks))
	for _, block := range blocks {
		b := read[block]
		kb := KeptConflictBlock{Block: block, Source: b.Text, Other: other[block]}
		if ed, ok := b.Editions[string(text)]; ok {
			kb.Held = KeptWording{Text: ed.Text, Rev: ed.Rev}
		} else {
			kb.Held = KeptWording{Rev: model.AbsentRevision, Absent: true}
		}
		out = append(out, kb)
	}
	return out, nil
}

// ErrNotKeptBesideFile is what ReleaseKeptWording returns for a block whose
// translation's file does not exist, or which the workspace does not keep:
// the workspace home is then the edition's home, and an edit decides it.
var ErrNotKeptBesideFile = errors.New("the workspace keeps no wording of that block beside its translation's file")

// ReleaseKeptWording drops what the workspace keeps of blocks of the locale
// translation of doc, once the translation's file exists and a person has
// decided what the file holds: the file is the edition's home, so the
// workspace's copy is released as a person's write through origin.
func (a *App) ReleaseKeptWording(ctx context.Context, recipe, doc string, locale model.LocaleID, blocks []string, origin string) error {
	beside, err := a.keptBesideFiles(ctx, recipe)
	if err != nil {
		return err
	}
	for _, bf := range beside {
		if bf.path != doc || bf.edition.Locale != locale {
			continue
		}
		var release []string
		for _, block := range blocks {
			if _, ok := bf.held.Rows[block]; ok {
				release = append(release, block)
			}
		}
		if len(release) == 0 {
			break
		}
		return a.releaseKept(ctx, filepath.Dir(recipe), bf.path, bf.edition, bf.held, release, change.Actor{Kind: change.ActorPerson}, origin)
	}
	return ErrNotKeptBesideFile
}
