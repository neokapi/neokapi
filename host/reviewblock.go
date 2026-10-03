package host

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// The review picture of one block, addressed the way the edit contract
// addresses content: {doc, block, edition}, where doc is the source document,
// block its block key and edition the translation under review (none for the
// source itself). The picture carries the edition's revision, which is what a
// decide operation names as if_match, so a pre-review an agent records binds
// to the wording it read.

// ReviewBlock is the review picture of one edition of one block.
type ReviewBlock struct {
	// Ref is the edition under review, in canonical form: the source
	// document, the block key, and the translation's edition, which is empty
	// for the source.
	Ref change.Ref `json:"ref"`
	// Rev is the edition's revision: the if_match a decide operation about it
	// sends.
	Rev string `json:"rev"`
	// Unit is the review queue's picture of the block, with the context the
	// decision is made in.
	Unit *ReviewUnitInfo `json:"unit"`
}

// ReviewBlockAt reads the review picture of the edition at names, in the
// project at recipe. at may name the source document with an edition, or the
// file of a translation (the edition is then the file's), as a read reports
// and as a review queue row's ref carries. An edition in the project's source
// language is the source itself.
func (a *App) ReviewBlockAt(ctx context.Context, recipe string, at change.Ref) (*ReviewBlock, error) {
	if at.Doc == "" || at.Block == "" {
		return nil, &change.Error{Code: change.CodeInvalid, Field: "at", Message: "name the document and the block to review"}
	}
	a.InitRegistries()
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return nil, fmt.Errorf("load project: %w", err)
	}
	sourceLang := ResolveSourceLocale("", proj.Defaults.SourceLanguage)
	edition := at.Edition.Canonical()
	if isLanguageEdition(edition, sourceLang) {
		edition = model.EditionKey{}
	}
	if edition.Tone != "" || edition.Channel != "" {
		return nil, &change.Error{Code: change.CodeUnsupported, Field: "at/edition", Capability: "review",
			Message: "the review queue holds a block's translations by language; " + at.EditionText() + " names a tone or a channel"}
	}

	svc, err := a.ChangeService(ctx, ChangeServiceOptions{Project: recipe, Origin: "review", SourceLocale: model.LocaleID(sourceLang)})
	if err != nil {
		return nil, err
	}
	req := change.ReadRequest{Doc: at.Doc, Blocks: []string{at.Block}, Limit: 1}
	if !edition.IsZero() {
		req.Editions = []model.EditionKey{edition}
	}
	page, err := svc.Read(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(page.Blocks) == 0 {
		return nil, &change.Error{Code: change.CodeNotFound, Field: "at/block",
			Message: fmt.Sprintf("%s has no block %s", page.Doc, at.Block)}
	}
	b := page.Blocks[0]
	out := &ReviewBlock{Ref: b.Ref, Rev: b.Rev}
	if !edition.IsZero() && out.Ref.Edition.Canonical() != edition {
		text := change.Ref{Edition: edition}.EditionText()
		ed, ok := b.Editions[text]
		if !ok {
			return nil, &change.Error{Code: change.CodeNotFound, Field: "at/edition",
				Message: fmt.Sprintf("block %s of %s has no %s edition", out.Ref.Block, out.Ref.Doc, text)}
		}
		out.Ref.Edition = edition
		out.Rev = ed.Rev
	}
	if isLanguageEdition(out.Ref.Edition, sourceLang) {
		out.Ref.Edition = model.EditionKey{}
	}

	unit := ReviewUnitRef{File: filepath.FromSlash(out.Ref.Doc), Key: out.Ref.Block, Locale: sourceLang}
	if !out.Ref.Edition.IsZero() {
		if unit, err = a.reviewTargetUnit(proj, recipe, out.Ref); err != nil {
			return nil, err
		}
	}
	info, err := a.ReviewUnitWithContext(ctx, recipe, "", unit)
	if err != nil {
		return nil, err
	}
	out.Unit = info
	return out, nil
}

// reviewTargetUnit is the review-queue address of a translation: the file the
// recipe writes the edition of ref's document to, and the locale it is
// declared under. ref's document is a source file, which resolves on its own.
func (a *App) reviewTargetUnit(proj *project.KapiProject, recipe string, ref change.Ref) (ReviewUnitRef, error) {
	pctx := project.NewProjectContext(proj, recipe)
	want := model.NormalizeLocale(ref.Edition.Locale)
	if rf, ok := claimedSource(a.FormatReg, pctx, ref.Doc); ok {
		for _, u := range a.unitsOfFile(proj, pctx.ProjectDir, rf, "") {
			if model.NormalizeLocale(model.LocaleID(u.Locale)) == want {
				return ReviewUnitRef{File: u.DisplayPath, Key: ref.Block, Locale: u.Locale}, nil
			}
		}
	}
	return ReviewUnitRef{}, &change.Error{Code: change.CodeNotFound, Field: "at/edition",
		Message: fmt.Sprintf("the project declares no %s translation of %s", ref.EditionText(), ref.Doc)}
}

// isLanguageEdition reports whether k is the plain edition of language lang:
// no tone, no channel.
func isLanguageEdition(k model.EditionKey, lang string) bool {
	return !k.IsZero() && k.Tone == "" && k.Channel == "" &&
		model.NormalizeLocale(k.Locale) == model.NormalizeLocale(model.LocaleID(lang))
}

// ReviewQueueRef is the reference a review queue row addresses its block by:
// the source document, the block key, and the translation's language, with
// no edition for a row in the source language.
func ReviewQueueRef(item ReviewQueueItem) change.Ref {
	doc := item.Relative
	if doc == "" {
		doc = item.File
	}
	ref := change.Ref{Doc: filepath.ToSlash(doc), Block: item.Key}
	if item.IsSource {
		return ref
	}
	if k, err := model.ParseEditionKey(item.Locale); err == nil {
		ref.Edition = k
	} else {
		ref.Edition = model.EditionKey{Locale: model.LocaleID(item.Locale)}
	}
	return ref
}
