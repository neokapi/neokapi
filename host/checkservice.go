package host

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
)

// This file is the exported, cobra-free surface of the bilingual check
// pipeline (read → pair → run → collect) — the same primitives `kapi check`
// and `kapi check --ship` are built from, callable by an embedding surface (the
// Kapi Desktop) so it cannot drift into a parallel reimplementation.

// ReadBlocksForCheck reads a file's translatable blocks through its format
// reader, with formatName overriding detection (empty = detect by extension)
// and sourceLang as the source locale. It is the check pipeline's read leg:
// when the project document cache is open (WithDocumentCache), unchanged
// files replay from the cache instead of re-parsing, exactly like the CLI's
// verify/status path.
func (a *App) ReadBlocksForCheck(ctx context.Context, path, formatName, sourceLang string) ([]*model.Block, error) {
	ctx = ctxOrBackground(ctx)
	return a.readBlocksAs(ctx, path, formatName, nil, sourceLang)
}

// WithDocumentCache opens the project document cache for the project rooted
// at root, runs fn, and closes the cache again. While it is open, the check
// and coverage read paths (ReadBlocksForCheck, the verify/status internals)
// serve unchanged files from the cache. A missing project layout or an
// unopenable cache degrades to direct parsing — fn always runs. When a cache
// is already open, fn runs against it and the outer owner keeps it.
func (a *App) WithDocumentCache(root string, fn func() error) error {
	return a.withParseCache(root, fn)
}

// OverlayTargets pairs target-file blocks onto source blocks by their stable
// unit key (Name when set, else ID — convergence.BlockKey) and copies each
// target block's text/runs onto the matching source block as the given target
// locale. Source blocks with no matching target keep an empty target, so QA
// flags them as untranslated. It is the pairing leg of the bilingual check
// pipeline (the core of the CLI's bilingualBlocks).
//
// # Why the key is not enough on its own
//
// A markdown block's key is a structural address whose segments are the slugs of
// the headings above it (core/formats/markdown/naming.go). Those headings are
// content, so a translated document addresses the very same paragraph as
// `hva-den-leser/p` where its source calls it `what-it-reads/p`, and every block
// under a translated heading fails to pair. A fully translated docs page
// therefore read as one third translated, could not be reviewed unit by unit,
// and could never clear a ship gate.
//
// So the key match is the first pass, and anything it leaves unpaired falls back
// to document position — but only when the two documents hold the same number of
// blocks. That guard is what makes the fallback exact rather than a guess: a
// target is materialized from the source's own skeleton, so equal counts mean
// the same sequence of blocks in the same order, and the nth block is the nth
// block's translation. Unequal counts mean the target has genuinely diverged
// (edited by hand, or produced by a different reader configuration), and there
// the honest answer is the key match alone.
func OverlayTargets(sourceBlocks, targetBlocks []*model.Block, locale model.LocaleID) {
	targetByKey := make(map[string]*model.Block, len(targetBlocks))
	for _, tb := range targetBlocks {
		targetByKey[convergence.BlockKey(tb)] = tb
	}
	positional := len(sourceBlocks) == len(targetBlocks)
	for i, sb := range sourceBlocks {
		tb, ok := targetByKey[convergence.BlockKey(sb)]
		if !ok {
			if !positional {
				continue // no target → empty; QA flags as untranslated.
			}
			tb = targetBlocks[i]
		}
		// Carry the translation onto the source block as the target locale so
		// checkers can compare inline codes structurally. A bilingual target
		// file (kapi's own .kbf.json interchange, which keeps the source in place and
		// the translation under targets.<locale>) carries the translation as the
		// block's target runs; prefer those. A monolingual target file (e.g.
		// fr-FR.json, whose content IS the translation) has none, so fall back to
		// the block's own source runs/text.
		if tr := tb.TargetRuns(locale); len(tr) > 0 {
			sb.SetTargetRuns(locale, tr)
		} else if tt := tb.TargetText(locale); tt != "" {
			sb.SetTargetText(locale, tt)
		} else if runs := tb.SourceRuns(); len(runs) > 0 {
			sb.SetTargetRuns(locale, runs)
		} else {
			sb.SetTargetText(locale, tb.SourceText())
		}
	}
}

// BlockProcessor is the minimal interface a check tool satisfies to run over
// a single block via RunCheckTool (the Process leg of tool.Tool).
type BlockProcessor interface {
	Process(context.Context, <-chan *model.Part, chan<- *model.Part) error
}

// RunCheckTool runs an annotate-only block tool (qa / term-check / brand
// vocabulary / placeholder / do-not-translate) over a single block in place.
// The tool records its findings on the block's annotations or properties;
// read them with FindingsFromBlock or the tool's own property keys.
//
// The tool's error is RETURNED, and every caller must act on it. A checker that
// could not run is NOT a checker that passed: the block carries no annotation
// either way, so a dropped error is indistinguishable from a clean result — and
// via computeLoopCheckExclusions that silently readmits a unit that fails a bound
// check into the shippable set. Checkers are not all local and deterministic
// (the voice checker dials the kapi-check plugin over gRPC), so this fires in
// practice, not only in theory.
func RunCheckTool(ctx context.Context, t BlockProcessor, b *model.Block) error {
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: b}
	close(in)
	errc := make(chan error, 1)
	go func() {
		defer close(out)
		errc <- t.Process(ctx, in, out)
	}()
	for range out { //nolint:revive // drain
	}
	if err := <-errc; err != nil {
		return fmt.Errorf("check %q: %w", blockKey(b), err)
	}
	return nil
}

// FindingsFromBlock reads the unified check annotation off a block. With
// clear, the annotation is removed after reading, so the same block can be
// run through a second checker without re-collecting the first checker's
// findings.
func FindingsFromBlock(b *model.Block, clear bool) []check.Finding {
	ann, ok := model.AnnoAs[*check.FindingsAnnotation](b, check.AnnotationKey)
	if !ok {
		return nil
	}
	if clear {
		b.DelAnno(check.AnnotationKey)
	}
	return ann.Findings
}
