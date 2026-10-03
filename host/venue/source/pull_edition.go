package source

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/history"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/host"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
)

// A pulled translation lands in a local file one of two ways. When the recipe
// claims the item and its target template keeps the source's format, the
// translation is an edition of the source: the pull compiles the runs the
// server holds into set_content operations, each naming the revision the
// local read found, and the change service writes the translation's file from
// the source's skeleton. Any other target (one in another format, or an item
// no collection claims) is a projection the target's own writer writes
// (writeTranslatedFile).

// pullActor is who a pull sends its change sets as: the tool that brings the
// server's translations down.
var pullActor = change.Actor{Kind: change.ActorTool, Name: "pull"}

// pulledTargets maps each pulled block's match key (targetMatchKey) to its
// translation into locale, as runs.
func pulledTargets(blocks []apiclient.SyncBlock, locale string) map[string][]model.Run {
	out := map[string][]model.Run{}
	for _, sb := range blocks {
		if _, ok := sb.Targets[locale]; !ok {
			continue
		}
		t := apiclient.SyncBlockToBlock(sb).Target(model.LocaleID(locale))
		if t == nil || !hasText(t.Runs) {
			continue
		}
		out[targetMatchKey(sb.Name, sb.SourceText)] = t.Runs
	}
	return out
}

// pulledBases maps each pulled block's match key (targetMatchKey) to the
// source its translation into locale was made from, as the venue's record of
// that translation says: the basis of a decision on it, or of a draft the
// venue made. records is the venue's ledger as the pull carries it. A
// translation the venue's record does not describe, and one whose source the
// venue does not know (a person wrote it there), are left out.
func pulledBases(blocks []apiclient.SyncBlock, locale string, records []venue.UnitDecision) map[string]string {
	if len(records) == 0 {
		return nil
	}
	type unitAt struct{ item, unit, variant string }
	byUnit := make(map[unitAt]venue.UnitDecision, len(records))
	for _, d := range records {
		byUnit[unitAt{d.ItemName, d.Unit, d.Variant}] = d
	}
	out := map[string]string{}
	for _, sb := range blocks {
		unit := sb.Unit
		if unit == "" {
			unit = sb.Name
		}
		d, ok := byUnit[unitAt{sb.ItemName, unit, locale}]
		if !ok || d.ContentHash == "" {
			continue
		}
		t := apiclient.SyncBlockToBlock(sb).Target(model.LocaleID(locale))
		if t == nil || d.TargetHash != state.TargetHash(model.RunsText(t.Runs)) {
			continue
		}
		out[targetMatchKey(sb.Name, sb.SourceText)] = d.ContentHash
	}
	return out
}

// hasText reports whether runs hold any content: text, or an inline code or
// structure.
func hasText(runs []model.Run) bool {
	for _, r := range runs {
		if r.Text == nil || r.Text.Text != "" {
			return true
		}
	}
	return false
}

// pullsAnEdition reports whether the translation of itemName at outPath is an
// edition of its source the change service writes: a collection claims the
// item with a target template, and the target keeps the source's extension.
func (c *BowrainSourceConnector) pullsAnEdition(itemName, outPath string) bool {
	recipe := c.project.Recipe
	for _, it := range recipe.IterateContent() {
		entryLang := string(it.Item.ResolvedSourceLanguage(it.Collection, recipe.Defaults))
		if coreproj.MatchGlob(coreproj.ResolvePathPattern(it.Item.Path, entryLang), itemName) {
			return it.Item.Target != "" && strings.EqualFold(filepath.Ext(itemName), filepath.Ext(outPath))
		}
	}
	return false
}

// pullServices holds, for one pull, the change service each language's
// translations are written through.
type pullServices map[string]*change.Service

// pullEdition gives the translation of itemName into locale the runs targets
// holds for its blocks, through the change service, and reports whether it
// wrote any. media are the locale variants of the document's media, which the
// writer substitutes as it writes; a document with none shares its
// language's service in services.
//
// bases (pulledBases) says which source the venue made each translation
// from. A translation made from the source this checkout holds is recorded
// with that source as its basis, so it reads stale here once the source
// moves. Any other is recorded with none (host.WithStatedBases): the venue
// made it from wording this checkout does not hold, and the source the
// checkout holds would claim it current.
func (c *BowrainSourceConnector) pullEdition(ctx context.Context, services pullServices, itemName, locale string, targets map[string][]model.Run, bases map[string]string, media []MediaReplacement) (bool, error) {
	svc := services[locale]
	if svc == nil || len(media) > 0 {
		opts := host.ChangeServiceOptions{
			Project: c.project.RecipePath(), Origin: history.OriginPull, TargetLocale: model.LocaleID(locale), Materialize: true,
		}
		if len(media) > 0 {
			opts.WriterHook = func(w format.DataFormatWriter) {
				if mrs, ok := w.(MediaReplacementSetter); ok {
					for _, mr := range media {
						mrs.SetMediaReplacement(mr.ZipPath, mr.Media)
					}
				}
			}
		}
		var err error
		if svc, err = c.app.ChangeService(ctx, opts); err != nil {
			return false, err
		}
		if len(media) == 0 {
			services[locale] = svc
		}
	}
	key := model.EditionKey{Locale: model.LocaleID(locale)}
	var ops []change.Op
	_, err := svc.ReadEach(ctx, change.ReadRequest{Doc: itemName, Editions: []model.EditionKey{key}}, func(b *model.Block, r change.BlockRead) error {
		match := targetMatchKey(b.Name, b.SourceText())
		runs, ok := targets[match]
		if !ok {
			return nil
		}
		op := change.Op{
			Kind: change.KindSetContent, At: change.Ref{Doc: itemName, Block: r.Ref.Block, Edition: key},
			IfMatch: model.EditionRevision(b, key),
			Body:    &change.SetContent{Runs: runs},
		}
		if basis, known := bases[match]; known && basis == state.SourceHash(b.SourceText()) {
			op.Basis = r.Rev
		}
		ops = append(ops, op)
		return nil
	})
	if err != nil || len(ops) == 0 {
		return false, err
	}
	set := change.Set{Gate: change.GateReport, Note: "pull " + locale, Ops: ops}
	res, err := svc.Apply(host.WithStatedBases(ctx, set), set, pullActor)
	if err != nil {
		return false, err
	}
	if res.Status != change.SetApplied {
		for _, r := range res.Ops {
			if r.Status == change.OpRefused && r.Error != nil && r.At != nil {
				return false, fmt.Errorf("block %s: %w", r.At.Block, r.Error)
			}
		}
		return false, fmt.Errorf("the change set was %s", res.Status)
	}
	return true, nil
}
