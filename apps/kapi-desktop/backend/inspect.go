package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/terms"
)

// InspectFile reads a content file through the project's format reader and
// returns the editor ContentTree as JSON — the same structure the docs site's
// PreviewKit (DocumentViewer / FormatPreview) renders. Format detection is
// content-aware and scoped to the project's declared plugins, and the project's
// per-format config overrides are applied to the reader so the tree reflects how
// the project actually parses the file.
//
// When committed target variants exist for the file's blocks (a project may have
// translated/merged targets into a sibling target file), they are overlaid onto
// the source blocks so BuildContentTree serializes `targets` and DocumentViewer's
// source↔target toggle works. Source-only files simply yield source-only nodes.
func (a *App) InspectFile(tabID, filePath string) (string, error) {
	return a.inspect(tabID, filePath, false)
}

// InspectFileAnnotated does everything InspectFile does, then runs the project's
// native, read-only annotators over the parsed blocks so the tree carries
// source-anchored stand-off overlays before serialization:
//
//   - term overlays (type "term") from the project's auto-opened terms
//     (LookupAll over each block's source text), carrying the matched surface
//     form, its preferred target translation and domain;
//   - voice-vocabulary overlays (type "qa", props.category="voice-vocabulary")
//     from the project's resolved voice profile (resolveProjectVoiceProfile via
//     coreprofile.MatchVocabulary);
//   - rule-based QA overlays (type "qa") from the shared source-only shape rules
//     (double spaces, doubled words — check.HygieneOverlay).
//
// These mirror the overlay shapes the docs "Anatomy" explorer produces in
// kapi/cmd/kapi-wasm-cli/lab_annotate.go, but use the project's real resources
// rather than a seeded in-memory terms / voice profile. DocumentViewer's
// annotations toggle highlights them on the rendered document.
func (a *App) InspectFileAnnotated(tabID, filePath string) (string, error) {
	return a.inspect(tabID, filePath, true)
}

// inspect is the shared body of InspectFile / InspectFileAnnotated.
func (a *App) inspect(tabID, filePath string, annotate bool) (string, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return "", fmt.Errorf("tab %q not found", tabID)
	}

	pctx := project.NewProjectContext(op.Project, op.Path)
	sourceLang := string(pctx.SourceLocale)
	if sourceLang == "" {
		sourceLang = "en"
	}

	fmtName := pctx.DetectFormat(a.formatReg, filePath)
	if fmtName == "" {
		return "", fmt.Errorf("could not detect a format for %q", filepath.Base(filePath))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	parts, err := a.readPartsForInspect(ctx, pctx, filePath, fmtName, sourceLang)
	if err != nil {
		return "", err
	}

	// Overlay any committed target variants from sibling target files so the
	// tree carries `targets` and the viewer's source↔target toggle works.
	a.overlayProjectTargets(ctx, op, pctx, filePath, fmtName, sourceLang, parts)

	if annotate {
		a.annotateParts(ctx, op, parts)
	}

	tree := editor.BuildContentTree(parts, fmtName)
	out, err := json.Marshal(tree)
	if err != nil {
		return "", fmt.Errorf("marshal content tree: %w", err)
	}
	return string(out), nil
}

// readPartsForInspect reads a file through its format reader, applying the
// project's per-format config overrides, and returns the full part stream (not
// just blocks) so BuildContentTree can reconstruct the layer/group hierarchy.
func (a *App) readPartsForInspect(ctx context.Context, pctx *project.ProjectContext, path, fmtName, sourceLang string) ([]*model.Part, error) {
	reader, err := a.formatReg.NewReader(registry.FormatID(fmtName))
	if err != nil {
		return nil, fmt.Errorf("no reader for %q: %w", fmtName, err)
	}
	defer reader.Close()

	// Apply the project's format config overrides (e.g. JSON/YAML key rules) so
	// the tree reflects the project's parsing, not bare defaults.
	if cfg, ok := reader.(project.Configurable); ok {
		if cerr := pctx.ConfigureReader(cfg, fmtName); cerr != nil {
			return nil, fmt.Errorf("configure reader for %q: %w", fmtName, cerr)
		}
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	doc := &model.RawDocument{
		URI:          path,
		SourceLocale: model.LocaleID(sourceLang),
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}
	if err := reader.Open(ctx, doc); err != nil {
		return nil, fmt.Errorf("open %q: %w", filepath.Base(path), err)
	}

	var parts []*model.Part
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			return nil, fmt.Errorf("read %q: %w", filepath.Base(path), pr.Error)
		}
		if pr.Part != nil {
			parts = append(parts, pr.Part)
		}
	}
	return parts, nil
}

// overlayProjectTargets looks for sibling target files for each of the project's
// target locales and, when present, overlays their text onto the matching source
// blocks (paired by Name/ID via the shared host.OverlayTargets). After this the source
// blocks carry committed targets, which BuildContentTree serializes.
//
// It resolves the file's content item from the project's resolved content so it
// can use the item's Target template; a file that is not part of the project
// content (or has no Target template) is left source-only.
func (a *App) overlayProjectTargets(ctx context.Context, op *openProject, pctx *project.ProjectContext, filePath, fmtName, sourceLang string, parts []*model.Part) {
	if len(pctx.TargetLocales) == 0 {
		return
	}
	rf := a.resolvedFileFor(pctx, filePath)
	if rf == nil || rf.Item == nil || rf.Item.Target == "" {
		return
	}

	sourceBlocks := blocksFromParts(parts)
	if len(sourceBlocks) == 0 {
		return
	}

	for _, loc := range pctx.TargetLocales {
		lang := string(loc)
		tgtPath := a.resolveTargetPath(*rf, op, lang)
		if tgtPath == "" {
			continue
		}
		if _, err := os.Stat(tgtPath); err != nil {
			continue
		}
		// A target is the translated rendering of the same item, so it is read
		// under the item's declared format and the recipe's config for it.
		targetBlocks, err := a.readBlocksForChecks(ctx, tgtPath, rf.Format,
			pctx.FormatConfigFor(rf.Format, rf.Item), sourceLang)
		if err != nil {
			continue
		}
		host.OverlayTargets(sourceBlocks, targetBlocks, loc)
	}
}

// resolvedFileFor returns the project's ResolvedFile for an absolute file path,
// or nil when the path is not part of the project's resolved content.
func (a *App) resolvedFileFor(pctx *project.ProjectContext, filePath string) *project.ResolvedFile {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		abs = filePath
	}
	resolved, err := pctx.ResolveContent(a.formatReg)
	if err != nil {
		return nil
	}
	for i := range resolved {
		if resolved[i].Path == abs {
			return &resolved[i]
		}
	}
	return nil
}

// blocksFromParts collects every translatable Block resource from a part stream.
func blocksFromParts(parts []*model.Part) []*model.Block {
	var blocks []*model.Block
	for _, p := range parts {
		if p == nil || p.Type != model.PartBlock {
			continue
		}
		if b, ok := p.Resource.(*model.Block); ok {
			blocks = append(blocks, b)
		}
	}
	return blocks
}

// --- native annotators ------------------------------------------------------

// annotateParts walks the part stream and writes source-anchored stand-off
// overlays onto every translatable Block, in place, using the project's real
// resources: its terms store (term overlays), its voice profile
// (voice-vocabulary QA overlays) and the shared source-only shape rules
// (check.HygieneOverlay). It mirrors
// the wasm "Anatomy" annotator (kapi/cmd/kapi-wasm-cli/lab_annotate.go) so the
// content tree's `overlays` view is populated identically, but sources its term
// and brand data from the open project rather than a seeded demo set.
func (a *App) annotateParts(ctx context.Context, op *openProject, parts []*model.Part) {
	pctx := project.NewProjectContext(op.Project, op.Path)
	sourceLoc := pctx.SourceLocale
	if sourceLoc == "" {
		sourceLoc = model.LocaleID("en")
	}
	var targetLoc model.LocaleID
	if len(pctx.TargetLocales) > 0 {
		targetLoc = pctx.TargetLocales[0]
	}

	var tb terms.Terminology
	if op.tbHandle != "" {
		if h, ok := a.tbHandles.Get(op.tbHandle); ok && h != nil {
			tb = h
		}
	}
	profile := a.resolveProjectVoiceProfile(ctx, op)

	for _, b := range blocksFromParts(parts) {
		if !b.Translatable {
			continue
		}
		source := b.SourceText()
		if strings.TrimSpace(source) == "" {
			continue
		}
		runs := b.SourceRuns()

		if tb != nil {
			if ov := termOverlay(ctx, tb, runs, source, sourceLoc, targetLoc); ov != nil {
				b.Overlays = append(b.Overlays, *ov)
			}
		}
		if profile != nil {
			if ov := voiceOverlay(profile, runs, source); ov != nil {
				b.Overlays = append(b.Overlays, *ov)
			}
		}
		// Shape QA (double spaces, doubled words) comes from the shared
		// check.HygieneOverlay, which judges the run-aware flattening and maps
		// its ranges back onto the runs. A preview highlight and a `kapi check`
		// finding therefore agree by construction rather than by coincidence.
		if ov := check.HygieneOverlay(runs); ov != nil {
			b.Overlays = append(b.Overlays, *ov)
		}
	}
}

// termOverlay builds an OverlayTerm over the source runs from the project's
// terms. Each matched term becomes a span carrying the matched
// surface form (term), its preferred target translation (target) and domain.
// Returns nil when nothing matches.
func termOverlay(ctx context.Context, tb terms.Terminology, runs []model.Run, source string, sourceLoc, targetLoc model.LocaleID) *model.Overlay {
	matches, err := tb.LookupAll(ctx, source, terms.LookupOptions{
		SourceLocale: sourceLoc,
		TargetLocale: targetLoc,
	})
	if err != nil || len(matches) == 0 {
		return nil
	}
	spans := make([]model.Span, 0, len(matches))
	for _, m := range matches {
		props := map[string]string{"term": m.Term.Text}
		// LookupAll's match carries only a partial concept (id/domain, no terms);
		// load the full concept to resolve the preferred target translation.
		domain := m.Concept.Domain
		if full, ok, err := tb.GetConcept(ctx, m.Concept.ID); err == nil && ok {
			if domain == "" {
				domain = full.Domain
			}
			if !targetLoc.IsEmpty() {
				if tgt := full.PreferredTerm(targetLoc); tgt != nil {
					props["target"] = tgt.Text
				}
			}
		}
		if domain != "" {
			props["domain"] = domain
		}
		spans = append(spans, model.Span{
			Range: model.RangeAnchorForBytes(runs, m.Position.Start, m.Position.End),
			Props: props,
		})
	}
	return &model.Overlay{Type: model.OverlayTerm, Spans: spans}
}

// voiceOverlay builds an OverlayQA over the source runs from the project's voice
// profile — both halves of its deterministic gate, vocabulary
// (coreprofile.MatchVocabulary) and prohibited style patterns
// (coreprofile.MatchPatterns). Findings ride on the QA overlay type (the model's
// overlay enum has no dedicated voice type) tagged with
// category="voice-vocabulary" or category="voice-pattern" plus the matched term
// or rule, severity, kind and any preferred replacement. Returns nil when
// nothing matches.
func voiceOverlay(profile *coreprofile.VoiceProfile, runs []model.Run, source string) *model.Overlay {
	hits := coreprofile.MatchVocabulary(profile, source)
	patterns := coreprofile.MatchPatterns(profile, source)
	if len(hits) == 0 && len(patterns) == 0 {
		return nil
	}
	spans := make([]model.Span, 0, len(hits)+len(patterns))
	for _, h := range hits {
		props := map[string]string{
			"category": "voice-vocabulary",
			"severity": string(h.Severity),
			"term":     h.Term,
		}
		switch h.Kind {
		case coreprofile.VocabCompetitor:
			props["kind"] = "competitor"
			props["message"] = fmt.Sprintf("Competitor term %q found", h.Term)
		default:
			props["kind"] = "forbidden"
			props["message"] = fmt.Sprintf("Forbidden term %q found", h.Term)
		}
		if h.Replacement != "" {
			props["replacement"] = h.Replacement
		}
		spans = append(spans, model.Span{
			Range: model.RangeAnchorForBytes(runs, h.Start, h.End),
			Props: props,
		})
	}
	for _, p := range patterns {
		spans = append(spans, model.Span{
			Range: model.RangeAnchorForBytes(runs, p.Start, p.End),
			Props: patternSpanProps(p),
		})
	}
	return &model.Overlay{Type: model.OverlayQA, Spans: spans}
}

// patternSpanProps renders one prohibited-pattern hit as overlay span props,
// keyed the same way the vocabulary spans are so a single panel renders both.
func patternSpanProps(p coreprofile.PatternHit) map[string]string {
	message := p.Description
	if strings.TrimSpace(message) == "" {
		message = fmt.Sprintf("Prohibited pattern %q matched", p.Regex)
	}
	return map[string]string{
		"category": "voice-pattern",
		"severity": string(p.Severity),
		"kind":     "pattern",
		"pattern":  p.Regex,
		"message":  message,
	}
}
