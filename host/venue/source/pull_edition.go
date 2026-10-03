package source

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
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

// pullEdition gives the translation of itemName into locale the runs targets
// holds for its blocks, through the change service, and reports whether it
// wrote any. media are the locale variants of the document's media, which the
// writer substitutes as it writes.
func (c *BowrainSourceConnector) pullEdition(ctx context.Context, itemName, locale string, targets map[string][]model.Run, media []MediaReplacement) (bool, error) {
	opts := host.ChangeServiceOptions{
		Project: c.project.RecipePath(), Origin: "pull", TargetLocale: model.LocaleID(locale), Materialize: true,
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
	svc, err := c.app.ChangeService(ctx, opts)
	if err != nil {
		return false, err
	}
	key := model.EditionKey{Locale: model.LocaleID(locale)}
	var ops []change.Op
	_, err = svc.ReadEach(ctx, change.ReadRequest{Doc: itemName, Editions: []model.EditionKey{key}}, func(b *model.Block, r change.BlockRead) error {
		runs, ok := targets[targetMatchKey(b.Name, b.SourceText())]
		if !ok {
			return nil
		}
		ops = append(ops, change.Op{
			Kind: change.KindSetContent, At: change.Ref{Doc: itemName, Block: r.Ref.Block, Edition: key},
			IfMatch: model.EditionRevision(b, key),
			Body:    &change.SetContent{Content: change.Content{Runs: runs}},
		})
		return nil
	})
	if err != nil || len(ops) == 0 {
		return false, err
	}
	res, err := svc.Apply(ctx, change.Set{Gate: change.GateReport, Note: "pull " + locale, Ops: ops}, pullActor)
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
