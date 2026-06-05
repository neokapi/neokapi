package backend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	brandstore "github.com/neokapi/neokapi/cli/storage/brand"
	"github.com/neokapi/neokapi/core/brand"
	brandpacks "github.com/neokapi/neokapi/core/brand/packs"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/storage"
	coretools "github.com/neokapi/neokapi/core/tools"
)

// DesktopFinding is one content-check finding, flattened for the React Checks
// panel. It mirrors core/check.Finding but adds the fields the panel needs to
// locate the offending block and (when safe) offer a one-click fix.
type DesktopFinding struct {
	Category     string `json:"category"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	Suggestion   string `json:"suggestion,omitempty"`
	OriginalText string `json:"original_text,omitempty"`
	// BlockID identifies the block the finding applies to (the format's stable
	// block ID), so ApplyCheckFix can re-find it.
	BlockID string `json:"block_id,omitempty"`
	// Field is which side of the block the offending text lives on:
	// "source" or "target".
	Field string `json:"field,omitempty"`
	// Replacement is the structured fix text (e.g. a brand profile's preferred
	// term). Empty when there is no safe automatic replacement.
	Replacement string `json:"replacement,omitempty"`
	// Fixable reports whether the panel may show an "Apply fix" button: a
	// replacement and a block to target both exist.
	Fixable bool `json:"fixable"`
}

// CheckFileResult groups the findings for a single content file.
type CheckFileResult struct {
	Path     string           `json:"path"`
	Findings []DesktopFinding `json:"findings"`
}

// CheckRunResult is the structured result of a RunChecks pass over a project,
// the unit the Checks panel renders and an assistant fix-loop acts on.
type CheckRunResult struct {
	Pass  bool              `json:"pass"`
	Score int               `json:"score"`
	Files []CheckFileResult `json:"files"`
}

// RunChecks runs the project's content checks (placeholder + do-not-translate
// when a target exists, brand vocabulary on the source when a brand profile is
// bound) over every matched content file and returns structured findings plus a
// pass/fail and roll-up score. It mirrors the CLI `kapi check` semantics
// (cli/check.go): the gate fails on any critical finding.
func (a *App) RunChecks(tabID string, targetLang string) (*CheckRunResult, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("tab %q not found", tabID)
	}

	pctx := project.NewProjectContext(op.Project, op.Path)
	resolved, err := pctx.ResolveContent(a.formatReg)
	if err != nil {
		return nil, fmt.Errorf("resolve content: %w", err)
	}

	sourceLang := string(pctx.SourceLocale)
	targetLoc := model.LocaleID(targetLang)

	// Resolve standing project context once: a bound brand profile (for the
	// source-side vocabulary check) and do-not-translate terms (from the bound
	// termbase). Both are optional — when absent the corresponding check is
	// simply skipped, matching the CLI's flag-free behavior.
	profile := a.resolveProjectBrandProfile(op)
	dntTerms := a.resolveProjectDNTTerms(op, sourceLang)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var allFindings []check.Finding
	files := make([]CheckFileResult, 0, len(resolved))

	for _, rf := range resolved {
		sourceBlocks, rerr := a.readBlocksForChecks(ctx, rf.Path, rf.Format, sourceLang)
		if rerr != nil {
			// Surface the read failure as a finding rather than aborting the
			// whole run: one unreadable file should not hide the rest.
			files = append(files, CheckFileResult{
				Path: rf.Path,
				Findings: []DesktopFinding{{
					Category: "io",
					Severity: string(check.SeverityMajor),
					Message:  rerr.Error(),
				}},
			})
			continue
		}

		// Bilingual: overlay the target file's text onto the source blocks so
		// the target-comparing checks can run.
		hasTarget := false
		if targetLang != "" {
			if tgtPath := a.resolveTargetPath(rf, op, targetLang); tgtPath != "" {
				if _, serr := os.Stat(tgtPath); serr == nil {
					targetBlocks, terr := a.readBlocksForChecks(ctx, tgtPath, "", sourceLang)
					if terr == nil {
						overlayTargets(sourceBlocks, targetBlocks, targetLoc)
						hasTarget = true
					}
				}
			}
		}

		var fileFindings []DesktopFinding

		if hasTarget {
			// Placeholder integrity (always, when a target exists).
			placeholder := coretools.NewPlaceholderCheckTool(coretools.NewPlaceholderCheckConfig(targetLoc))
			for _, b := range sourceBlocks {
				runCheckToolOnBlock(ctx, placeholder, b)
				for _, f := range findingsFromCheckBlock(b) {
					fileFindings = append(fileFindings, toDesktopFinding(f, b, "target"))
					allFindings = append(allFindings, f)
				}
			}

			// Do-not-translate: only when terms are configured.
			if len(dntTerms) > 0 {
				dntCfg := coretools.NewDNTCheckConfig(targetLoc)
				dntCfg.Terms = dntTerms
				dnt := coretools.NewDNTCheckTool(dntCfg)
				for _, b := range sourceBlocks {
					runCheckToolOnBlock(ctx, dnt, b)
					for _, f := range findingsFromCheckBlock(b) {
						fileFindings = append(fileFindings, toDesktopFinding(f, b, "target"))
						allFindings = append(allFindings, f)
					}
				}
			}
		}

		// Brand vocabulary — source-side, when a profile is bound.
		if profile != nil {
			vocab := coretools.NewBrandVocabCheckTool(profile, nil)
			for _, b := range sourceBlocks {
				runCheckToolOnBlock(ctx, vocab, b)
				if ann, ok := b.Annotations["brand-voice"].(*brand.BrandVoiceAnnotation); ok {
					for _, f := range ann.Findings {
						fileFindings = append(fileFindings, toDesktopFinding(f, b, "source"))
						allFindings = append(allFindings, f)
					}
				}
			}
		}

		sortDesktopFindings(fileFindings)
		files = append(files, CheckFileResult{Path: rf.Path, Findings: fileFindings})
	}

	score := check.CalculateScore(allFindings).Overall
	critical := 0
	for _, f := range allFindings {
		if f.Severity == check.SeverityCritical {
			critical++
		}
	}

	return &CheckRunResult{
		Pass:  critical == 0,
		Score: score,
		Files: files,
	}, nil
}

// ApplyCheckFix applies a single finding's structured replacement to a block in
// a content file — the Checks panel's one-click fix. It reads the file through
// its format reader, finds the block by ID, replaces the first occurrence of
// original with replacement in the requested field (source or target), and
// writes the file back through the format writer.
//
// Safety: the edit is only applied when the field's content is a single plain
// text run (no inline markup / multiple runs). A plain substring replace over
// runs that carry placeholders or paired codes could silently corrupt the
// markup, so in that case the fix is refused with a clear error and the file is
// left untouched.
func (a *App) ApplyCheckFix(tabID, filePath, blockID, field, original, replacement string) error {
	op := a.getOpenProject(tabID)
	if op == nil {
		return fmt.Errorf("tab %q not found", tabID)
	}
	if blockID == "" {
		return errors.New("a block id is required to apply a fix")
	}
	if original == "" || replacement == "" {
		return errors.New("both the original text and its replacement are required")
	}
	if field != "source" && field != "target" {
		return fmt.Errorf("field must be %q or %q, got %q", "source", "target", field)
	}

	pctx := project.NewProjectContext(op.Project, op.Path)
	sourceLang := string(pctx.SourceLocale)

	fmtName := pctx.DetectFormat(a.formatReg, filePath)
	if fmtName == "" {
		return fmt.Errorf("could not detect a format for %q", filepath.Base(filePath))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The fix is in-place: read parts, mutate the one matching block, write the
	// same parts back. We mutate the live *model.Block carried by the part so
	// the writer reproduces the file faithfully (skeleton, ordering, untouched
	// blocks all preserved).
	//
	// A content file is read monolingually here, so its text — whether it is the
	// source file (field "source") or a translated target file (field "target")
	// — lands in the block's source runs. The fix therefore always replaces in
	// the block's own text runs; `field` records which side the finding was on
	// (it determines which file path the panel passes), not a different setter.
	applied := false
	var applyErr error
	transform := func(b *model.Block) {
		if b.ID != blockID {
			return
		}
		runs := b.SourceRuns()
		if !isSinglePlainTextRun(runs) {
			applyErr = fmt.Errorf("manual fix needed (formatted content): block %q has inline markup or multiple runs, so an automatic replace could corrupt it", blockID)
			return
		}
		text := model.RunsText(runs)
		if !strings.Contains(text, original) {
			applyErr = fmt.Errorf("the original text %q is no longer present in block %q (it may already be fixed)", original, blockID)
			return
		}
		b.SetSourceText(strings.Replace(text, original, replacement, 1))
		applied = true
	}

	if err := a.rewriteFile(ctx, filePath, fmtName, sourceLang, transform); err != nil {
		return err
	}
	if applyErr != nil {
		return applyErr
	}
	if !applied {
		return fmt.Errorf("block %q not found in %q", blockID, filepath.Base(filePath))
	}
	return nil
}

// rewriteFile reads filePath through its format reader, runs transform over
// every block (in stream order), then writes the parts back through the format
// writer atomically (temp file + rename). Non-block parts pass through
// unchanged.
func (a *App) rewriteFile(ctx context.Context, filePath, fmtName, sourceLang string, transform func(*model.Block)) error {
	reader, err := a.formatReg.NewReader(registry.FormatID(fmtName))
	if err != nil {
		return fmt.Errorf("no reader for %q: %w", fmtName, err)
	}
	defer reader.Close()

	writer, err := a.formatReg.NewWriter(registry.FormatID(fmtName))
	if err != nil {
		return fmt.Errorf("no writer for %q: %w", fmtName, err)
	}
	defer writer.Close()

	// Wire skeleton store when both sides support it so the writer reproduces
	// the original structure (whitespace, key order, untouched values).
	var skeletonStore *format.SkeletonStore
	if emitter, ok := reader.(format.SkeletonStoreEmitter); ok {
		if consumer, ok := writer.(format.SkeletonStoreConsumer); ok {
			if store, serr := format.NewSkeletonStore(); serr == nil {
				skeletonStore = store
				emitter.SetSkeletonStore(store)
				consumer.SetSkeletonStore(store)
				defer skeletonStore.Close()
			}
		}
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read %s: %w", filepath.Base(filePath), err)
	}

	doc := &model.RawDocument{
		URI:          filePath,
		SourceLocale: model.LocaleID(sourceLang),
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}
	if err := reader.Open(ctx, doc); err != nil {
		return fmt.Errorf("open %q: %w", filepath.Base(filePath), err)
	}

	var parts []*model.Part
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			return fmt.Errorf("read %q: %w", filepath.Base(filePath), pr.Error)
		}
		if pr.Part != nil && pr.Part.Type == model.PartBlock {
			if b, ok := pr.Part.Resource.(*model.Block); ok {
				transform(b)
			}
		}
		parts = append(parts, pr.Part)
	}
	reader.Close()

	// Write back atomically.
	tmp, err := os.CreateTemp(filepath.Dir(filePath), ".kapi-fix-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}

	if err := writer.SetOutputWriter(tmp); err != nil {
		cleanup()
		return fmt.Errorf("set output: %w", err)
	}
	// Skeleton-driven writers (OpenXML) need the original bytes to rebuild.
	if ocs, ok := writer.(format.OriginalContentSetter); ok {
		ocs.SetOriginalContent(content)
	}

	in := make(chan *model.Part, len(parts)+1)
	for _, p := range parts {
		in <- p
	}
	close(in)
	if err := writer.Write(ctx, in); err != nil {
		cleanup()
		return fmt.Errorf("write %q: %w", filepath.Base(filePath), err)
	}
	if err := writer.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close writer: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("finalize %q: %w", filepath.Base(filePath), err)
	}
	return nil
}

// --- helpers ---------------------------------------------------------------

// readBlocksForChecks reads a file through its format reader and returns the
// blocks. fmtName may be empty to auto-detect by extension.
func (a *App) readBlocksForChecks(ctx context.Context, path, fmtName, sourceLang string) ([]*model.Block, error) {
	if fmtName == "" {
		ext := filepath.Ext(path)
		detected, err := a.formatReg.DetectByExtension(ext)
		if err != nil {
			return nil, fmt.Errorf("detect format for %q: %w", filepath.Base(path), err)
		}
		fmtName = string(detected)
	}
	reader, err := a.formatReg.NewReader(registry.FormatID(fmtName))
	if err != nil {
		return nil, fmt.Errorf("no reader for %q: %w", fmtName, err)
	}
	defer reader.Close()

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

	var blocks []*model.Block
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			return nil, fmt.Errorf("read %q: %w", filepath.Base(path), pr.Error)
		}
		if pr.Part != nil && pr.Part.Type == model.PartBlock {
			if b, ok := pr.Part.Resource.(*model.Block); ok {
				blocks = append(blocks, b)
			}
		}
	}
	return blocks, nil
}

// resolveTargetPath derives the on-disk path of the translated file for a
// source file and target language, using the content item's Target template
// (e.g. "locales/{lang}.json" or "output/{lang}/*"). Returns "" when the item
// declares no target template.
func (a *App) resolveTargetPath(rf project.ResolvedFile, op *openProject, targetLang string) string {
	if rf.Item == nil || rf.Item.Target == "" {
		return ""
	}
	base := filepath.Dir(op.Path)
	target := strings.ReplaceAll(rf.Item.Target, "{lang}", targetLang)
	if strings.Contains(target, "*") {
		target = strings.ReplaceAll(target, "*", filepath.Base(rf.Relative))
	}
	return filepath.Join(base, target)
}

// overlayTargets pairs target-file blocks onto source blocks by their stable
// key (Name, else ID) and copies the target text/runs onto the source block as
// the given target locale, mirroring cli.bilingualBlocks.
func overlayTargets(sourceBlocks, targetBlocks []*model.Block, locale model.LocaleID) {
	byKey := make(map[string]*model.Block, len(targetBlocks))
	for _, tb := range targetBlocks {
		byKey[checkBlockKey(tb)] = tb
	}
	for _, sb := range sourceBlocks {
		tb, ok := byKey[checkBlockKey(sb)]
		if !ok {
			continue
		}
		if runs := tb.SourceRuns(); len(runs) > 0 {
			sb.SetTargetRuns(locale, runs)
		} else {
			sb.SetTargetText(locale, tb.SourceText())
		}
	}
}

// checkBlockKey returns the stable pairing key for a block: Name when set, else ID.
func checkBlockKey(b *model.Block) string {
	if b.Name != "" {
		return b.Name
	}
	return b.ID
}

// resolveProjectBrandProfile resolves the brand voice profile bound to the open
// project, mirroring cli.resolveProjectBrandProfile (minus the cobra plumbing):
//   - defaults.brand_voice → profile_file (relative to root) / pack / profile
//     (local brand store under the kapi config dir)
//   - convention files brand.yaml, .kapi/brand.yaml at the project root
//
// Returns nil when no binding is found — the vocabulary check is then skipped.
func (a *App) resolveProjectBrandProfile(op *openProject) *brand.VoiceProfile {
	if op.Path == "" {
		return nil
	}
	root := filepath.Dir(op.Path)

	if bv := op.Project.Defaults.BrandVoice; bv != nil {
		switch {
		case bv.ProfileFile != "":
			p := bv.ProfileFile
			if !filepath.IsAbs(p) {
				p = filepath.Join(root, p)
			}
			if prof := loadCheckProfileFile(p); prof != nil {
				return prof
			}
		case bv.Pack != "":
			if prof, err := brandpacks.Load(bv.Pack); err == nil {
				return prof
			}
		case bv.Profile != "":
			if prof := a.lookupBrandStoreProfile(bv.Profile); prof != nil {
				return prof
			}
		}
	}

	for _, conv := range []string{
		filepath.Join(root, "brand.yaml"),
		filepath.Join(root, project.StateDirName, "brand.yaml"),
	} {
		if prof := loadCheckProfileFile(conv); prof != nil {
			return prof
		}
	}
	return nil
}

// lookupBrandStoreProfile loads a profile by id (then slugged name) from the
// local SQLite brand store under the kapi config dir. Returns nil on any miss.
func (a *App) lookupBrandStoreProfile(name string) *brand.VoiceProfile {
	dbPath := filepath.Join(kapiConfigDir(), "brand.db")
	if _, err := os.Stat(dbPath); err != nil {
		return nil
	}
	db, err := storage.Open(dbPath)
	if err != nil {
		return nil
	}
	store, err := brandstore.NewSQLiteBrandStore(db)
	if err != nil {
		_ = db.Close()
		return nil
	}
	defer store.Close()
	ctx := context.Background()
	if p, gerr := store.GetProfile(ctx, name); gerr == nil {
		return p
	}
	return nil
}

// loadCheckProfileFile loads a VoiceProfile YAML, returning nil when the file
// does not exist or fails to parse (best-effort: the check is optional).
func loadCheckProfileFile(path string) *brand.VoiceProfile {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	p, err := brand.LoadProfileYAML(f)
	if err != nil {
		return nil
	}
	return p
}

// resolveProjectDNTTerms collects do-not-translate terms from the project's
// auto-opened termbase. A concept opts in via a `do_not_translate` property
// (the term must survive verbatim); its source-locale term texts are returned.
// Returns nil when there is no termbase or no opted-in concepts — the
// do-not-translate check is then skipped, matching the CLI's flag-driven default.
func (a *App) resolveProjectDNTTerms(op *openProject, sourceLang string) []string {
	if op.tbHandle == "" {
		return nil
	}
	tb, ok := a.tbHandles.Get(op.tbHandle)
	if !ok || tb == nil {
		return nil
	}
	srcLoc := model.LocaleID(sourceLang)
	seen := make(map[string]bool)
	var terms []string
	concepts, err := tb.Concepts(context.Background())
	if err != nil {
		return nil
	}
	for _, c := range concepts {
		if !dntConcept(c.Properties) {
			continue
		}
		for _, t := range c.Terms {
			if (srcLoc == "" || t.Locale == srcLoc) && t.Text != "" && !seen[t.Text] {
				seen[t.Text] = true
				terms = append(terms, t.Text)
			}
		}
	}
	sort.Strings(terms)
	return terms
}

// dntConcept reports whether a concept's properties mark it do-not-translate.
func dntConcept(props map[string]string) bool {
	if props == nil {
		return false
	}
	for _, k := range []string{"do_not_translate", "dnt", "no_translate"} {
		switch strings.ToLower(props[k]) {
		case "true", "1", "yes":
			return true
		}
	}
	return false
}

// runCheckToolOnBlock runs an annotate-only block tool over a single block in
// place, mirroring cli.runCheckTool.
func runCheckToolOnBlock(ctx context.Context, t interface {
	Process(context.Context, <-chan *model.Part, chan<- *model.Part) error
}, b *model.Block) {
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
	<-errc
}

// findingsFromCheckBlock reads (and clears) the unified check annotation off a
// block. Clearing lets the same block be run through a second checker without
// re-collecting the first checker's findings.
func findingsFromCheckBlock(b *model.Block) []check.Finding {
	ann, ok := b.Annotations[check.AnnotationKey].(*check.FindingsAnnotation)
	if !ok {
		return nil
	}
	delete(b.Annotations, check.AnnotationKey)
	return ann.Findings
}

// toDesktopFinding flattens a check.Finding for the panel, wiring the block ID,
// the field the offending text lives on, and a structured replacement (from
// Metadata["replacement"], set by the vocabulary checker) when one exists.
func toDesktopFinding(f check.Finding, b *model.Block, field string) DesktopFinding {
	replacement := ""
	if f.Metadata != nil {
		replacement = f.Metadata["replacement"]
	}
	df := DesktopFinding{
		Category:     f.Category,
		Severity:     string(f.Severity),
		Message:      f.Message,
		Suggestion:   f.Suggestion,
		OriginalText: f.OriginalText,
		BlockID:      b.ID,
		Field:        field,
		Replacement:  replacement,
	}
	df.Fixable = replacement != "" && b.ID != "" && f.OriginalText != ""
	return df
}

// isSinglePlainTextRun reports whether runs is exactly one TextRun — the only
// shape where a plain substring replace is structurally safe (no placeholders
// or paired inline codes to corrupt).
func isSinglePlainTextRun(runs []model.Run) bool {
	return len(runs) == 1 && runs[0].Text != nil
}

// checkSeverityRank orders findings critical → neutral for stable panel output.
var checkSeverityRank = map[string]int{"critical": 0, "major": 1, "minor": 2, "neutral": 3}

func sortDesktopFindings(fs []DesktopFinding) {
	sort.SliceStable(fs, func(i, j int) bool {
		if checkSeverityRank[fs[i].Severity] != checkSeverityRank[fs[j].Severity] {
			return checkSeverityRank[fs[i].Severity] < checkSeverityRank[fs[j].Severity]
		}
		return fs[i].Category < fs[j].Category
	})
}
