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
// reader, with formatName overriding detection (empty = detect by extension),
// formatConfig configuring that reader (nil = reader defaults) and sourceLang as
// the source locale. It is the check pipeline's read leg: when the project
// document cache is open (WithDocumentCache), unchanged files replay from the
// cache instead of re-parsing, exactly like the CLI's verify/status path.
//
// A gate must read a file the way the loop does, so both halves of the recipe's
// binding travel here: the format the item declares, and the config
// project.ProjectContext.FormatConfigFor merges for it. Reading under the
// extension's default format, or under reader defaults, judges content the
// project does not declare as content.
func (a *App) ReadBlocksForCheck(ctx context.Context, path, formatName string, formatConfig map[string]any, sourceLang string) ([]*model.Block, error) {
	ctx = ctxOrBackground(ctx)
	return a.readBlocksAs(ctx, path, formatName, formatConfig, sourceLang)
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
// locale. Source blocks with no matching target keep an empty target, so the
// checks flag them as untranslated. It is the pairing leg of the bilingual check
// pipeline (the core of the CLI's bilingualBlocks).
//
// # Why the key is not enough on its own
//
// A markdown block's key is a structural address whose segments are the slugs of
// the headings above it (core/formats/markdown/naming.go). Those headings are
// content, so a translated document addresses the very same paragraph as
// `hva-den-leser/p` where its source calls it `what-it-reads/p`, and a key match
// alone leaves every block under a translated heading unpaired. A fully
// translated docs page then read as one third translated, could not be reviewed
// unit by unit, and could never clear a ship gate.
//
// Pairing therefore runs three passes, strongest first.
//
// The key is first: two blocks that agree on it ARE the same unit.
//
// What the key leaves over pairs on the block's TRANSLATION-INVARIANT ADDRESS
// (convergence.BlockAddress) — the same structural trail with each heading
// written as its own identity rather than as its words, so it reads the same in
// both languages. This is an identity match, not a guess: it holds however far
// the two documents' block counts have drifted apart, and a block whose section
// genuinely no longer exists in the target still finds no partner.
//
// Only what neither pass claimed falls back to document position, and only when
// the two documents hold the same number of blocks. The guard is what makes that
// fallback exact rather than a guess: a target is materialized from the source's
// own skeleton, so equal counts mean the same sequence of blocks in the same
// order. It is the last resort for formats that compose no address at all —
// unequal counts there mean the target has genuinely diverged (edited by hand,
// or produced by a different reader configuration), and the honest answer is the
// key match alone.
func OverlayTargets(sourceBlocks, targetBlocks []*model.Block, locale model.LocaleID) {
	targetByKey := make(map[string]*model.Block, len(targetBlocks))
	targetByAddress := make(map[string]*model.Block, len(targetBlocks))
	for _, tb := range targetBlocks {
		targetByKey[convergence.BlockKey(tb)] = tb
		if addr := convergence.BlockAddress(tb); addr != "" {
			targetByAddress[addr] = tb
		}
	}
	positional := len(sourceBlocks) == len(targetBlocks)
	for i, sb := range sourceBlocks {
		tb, ok := targetByKey[convergence.BlockKey(sb)]
		if !ok {
			if addr := convergence.BlockAddress(sb); addr != "" {
				tb, ok = targetByAddress[addr]
			}
		}
		if !ok {
			if !positional {
				continue // no target → empty; the checks flag it as untranslated.
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
		// The overlay carries how the target was produced, not only what it
		// says. A reader that restores provenance loses it again here
		// otherwise, because the setters above build a fresh Target: every
		// caller downstream — the record absorber most of all — then reads an
		// answer with no governing context recorded, which is the same thing it
		// reads for an answer produced under no governance.
		if t := tb.Target(locale); t != nil {
			if st := sb.Target(locale); st != nil {
				status := st.Status
				if t.Status != "" {
					status = t.Status
				}
				sb.StampTargetProvenance(locale, status, t.Origin)
			}
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
