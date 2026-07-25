package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/kmb"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
	"gopkg.in/yaml.v3"
)

// applyAssetEntry lands one asset change (a glossary term, content memory pair, brand rule,
// or recipe field). It implements decision B of the apply design: an asset edit
// is written into the asset's COMMITTED SOURCE artifact — the git-tracked file
// the recipe points at — and the EXISTING import/compile then refreshes the
// gitignored SQLite cache from that source. The backing store is therefore
// written by exactly one path (the importer), `git diff` is the uniform review
// surface, and apply is idempotent: a term/pair/rule already present with the
// same value is a "skipped" no-op, safe to re-run in a check→fix loop.
//
// No AI provider or credential is touched. Every asset kind requires a .kapi
// project (the committed source + recipe live there); without one the result is
// a precise error so the caller exits on the gate code and the fix loop can act.
func (a *App) applyAssetEntry(ctx context.Context, cmd Command, e changeEntry) assetResult {
	res := assetResult{Kind: e.Kind, Op: e.Op}
	if err := ctx.Err(); err != nil {
		res.Status = "error"
		res.Detail = err.Error()
		return res
	}

	switch e.Kind {
	case kindTerm:
		return a.applyTermEntry(ctx, cmd, e)
	case kindMemory:
		return a.applyMemoryEntry(ctx, cmd, e)
	case kindBrand:
		return a.applyBrandEntry(ctx, cmd, e)
	case kindRecipe:
		return a.applyRecipeEntry(cmd, e)
	default:
		res.Status = "error"
		res.Detail = fmt.Sprintf("unsupported asset kind %q", e.Kind)
		return res
	}
}

// resolveProjectRoot resolves the .kapi project recipe and its root directory.
// Every asset kind requires a project; a missing one is a precise error.
func (a *App) resolveProjectRoot(cmd Command) (recipePath, root string, err error) {
	recipePath, err = ResolveProjectPath(cmd)
	if err != nil {
		return "", "", err
	}
	if recipePath == "" {
		return "", "", errors.New("no kapi project")
	}
	return recipePath, filepath.Dir(recipePath), nil
}

// ---------------------------------------------------------------------------
// term → committed .terms.json source → terms import compile into .kapi/termbase.db
// ---------------------------------------------------------------------------

// applyTermEntry upserts a glossary term. It edits the committed .terms.json source
// the recipe binds (creating l10n/terms.json and binding it when none
// exists), then re-imports the whole .terms.json into the project terms store (.db)
// cache so the SQLite store reflects the committed source — one write path.
func (a *App) applyTermEntry(ctx context.Context, cmd Command, e changeEntry) assetResult {
	res := assetResult{Kind: e.Kind, Op: e.Op, Target: e.Term}

	if e.Op != "" && e.Op != "upsert" {
		return errResult(res, fmt.Sprintf("term: unsupported op %q (want \"upsert\")", e.Op))
	}
	if strings.TrimSpace(e.Term) == "" {
		return errResult(res, "term: empty term")
	}

	recipePath, root, err := a.resolveProjectRoot(cmd)
	if err != nil {
		return errResult(res, err.Error())
	}

	srcPath, err := a.ensureTermsSourceBinding(recipePath, root)
	if err != nil {
		return errResult(res, err.Error())
	}

	concepts, err := loadKTBConcepts(srcPath)
	if err != nil {
		return errResult(res, err.Error())
	}

	locale := e.Locale
	if locale == "" {
		locale = "en"
	}
	status := model.TermStatus(e.Status)
	if status == "" {
		status = model.TermPreferred
	}

	concepts, changed := upsertTerm(concepts, e.Term, model.LocaleID(locale), status, e.Replacement)
	if !changed {
		res.Status = "skipped"
		res.Detail = "already present"
		return res
	}

	if err := writeKTB(srcPath, concepts); err != nil {
		return errResult(res, err.Error())
	}
	if err := a.compileTermsSource(ctx, root, srcPath); err != nil {
		return errResult(res, err.Error())
	}

	res.Status = "applied"
	res.Detail = filepath.Base(srcPath)
	return res
}

// ensureTermsSourceBinding returns the committed .terms.json source path the
// recipe binds via defaults.termbase_source, creating a default
// (l10n/terms.json) and writing the binding into the recipe when none is
// bound — so future runs are consistent.
func (a *App) ensureTermsSourceBinding(recipePath, root string) (string, error) {
	proj, err := project.LoadWithOptions(recipePath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return "", fmt.Errorf("load project: %w", err)
	}
	if bound := proj.Defaults.TermsSource; bound != "" {
		return resolveUnder(root, bound), nil
	}
	rel := filepath.Join("l10n", ktb.ConventionalName)
	proj.Defaults.TermsSource = rel
	// Bind the compiled cache too, so term enforcement (resolveProjectTermsPath)
	// reads the .db this source compiles into rather than an unrelated default.
	if proj.Defaults.Terms == "" {
		proj.Defaults.Terms = filepath.Join(project.StateDirName, "termbase.db")
	}
	if err := project.Save(recipePath, proj); err != nil {
		return "", fmt.Errorf("bind terms source: %w", err)
	}
	return resolveUnder(root, rel), nil
}

// compileTermsSource re-imports the committed .terms.json into the project
// terms (.db) cache — the single store-write path. The cache is the recipe's
// bound terms, else the .kapi/termbase.db convention.
func (a *App) compileTermsSource(ctx context.Context, root, srcPath string) error {
	dbPath := filepath.Join(root, project.StateDirName, "termbase.db")
	if proj, err := a.loadRecipeForRoot(root); err == nil && proj.Defaults.Terms != "" {
		dbPath = resolveUnder(root, proj.Defaults.Terms)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return fmt.Errorf("create terms dir: %w", err)
	}

	tb, err := terms.NewSQLiteStore(dbPath)
	if err != nil {
		return fmt.Errorf("open terms cache: %w", err)
	}
	defer tb.Close()

	f, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open terms source: %w", err)
	}
	defer f.Close()
	if _, err := ImportKTBFile(ctx, tb, f); err != nil {
		return fmt.Errorf("compile terms: %w", err)
	}
	return nil
}

// loadKTBConcepts reads the concepts from a .terms.json source, returning an empty
// slice when the file does not exist yet (the first term creates it).
func loadKTBConcepts(path string) ([]terms.Concept, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open terms source: %w", err)
	}
	defer f.Close()
	file, err := ktb.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("parse terms source: %w", err)
	}
	return file.Concepts, nil
}

// writeKTB serializes concepts to a deterministic .terms.json document, creating
// parent directories. The deterministic marshal keeps `git diff` minimal.
func writeKTB(path string, concepts []terms.Concept) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create source dir: %w", err)
	}
	data, err := ktb.Marshal(ktb.FromConcepts(concepts))
	if err != nil {
		return fmt.Errorf("marshal terms source: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write terms source: %w", err)
	}
	return nil
}

// upsertTerm inserts or updates a single-locale term within the concept set.
// It is idempotent: when a term with the same text/locale already carries the
// requested status (and the replacement note matches), it returns changed=false
// so apply reports a skipped no-op. A term is matched case-insensitively on its
// text within its locale; a new term creates its own concept (one concept per
// term text), keyed by a stable id so re-seeding is reproducible.
func upsertTerm(concepts []terms.Concept, text string, locale model.LocaleID, status model.TermStatus, replacement string) ([]terms.Concept, bool) {
	now := time.Now().UTC()
	for ci := range concepts {
		c := &concepts[ci]
		for ti := range c.Terms {
			t := &c.Terms[ti]
			if t.Locale != locale || !strings.EqualFold(t.Text, text) {
				continue
			}
			noteWant := replacementNote(replacement)
			if t.Status == status && t.Text == text && t.Note == noteWant {
				return concepts, false
			}
			t.Status = status
			t.Text = text
			if noteWant != "" {
				t.Note = noteWant
			}
			c.UpdatedAt = now
			return concepts, true
		}
	}
	concepts = append(concepts, terms.Concept{
		ID:     conceptID(text, locale),
		Source: terms.TermSourceTerminology,
		Terms: []terms.Term{{
			Text:   text,
			Locale: locale,
			Status: status,
			Note:   replacementNote(replacement),
		}},
		CreatedAt: now,
		UpdatedAt: now,
	})
	return concepts, true
}

// replacementNote folds a forbidden term's suggested replacement into the
// term note (ktb's Term has no dedicated replacement field), so the guidance
// survives the round-trip.
func replacementNote(replacement string) string {
	if replacement == "" {
		return ""
	}
	return "use: " + replacement
}

// conceptID derives a stable, filesystem-safe concept id from the term text and
// locale so re-applying the same term re-seeds the same concept.
func conceptID(text string, locale model.LocaleID) string {
	return "term:" + string(locale) + ":" + slugify(text)
}

// ---------------------------------------------------------------------------
// tm → committed .memory.json source → tm import compile into .kapi/tm.db
// ---------------------------------------------------------------------------

// applyReviewEntry records a review decision in the project STATE store via the
// shared ApproveReviewUnit path — the CLI counterpart of the desktop "approve"
// action and the write side of `kapi status --review`. The unit is addressed by
// (file, id, locale) exactly as the review queue lists it; `status` is "reviewed"
// (default) or "signed-off". This is distinct from a `kind:"tm"` entry: a content memory
// correction is recycle leverage, not a review decision.
func (a *App) applyReviewEntry(ctx context.Context, cmd Command, e changeEntry) assetResult {
	res := assetResult{Kind: e.Kind, Op: e.Op, Target: e.ID}
	if e.Op != "" && e.Op != "add" {
		return errResult(res, fmt.Sprintf("review: unsupported op %q (want \"add\")", e.Op))
	}
	if strings.TrimSpace(e.File) == "" || strings.TrimSpace(e.ID) == "" || strings.TrimSpace(e.Locale) == "" {
		return errResult(res, "review: file, id, and locale are required (as listed by `kapi status --review`)")
	}
	recipePath, _, err := a.resolveProjectRoot(cmd)
	if err != nil {
		return errResult(res, err.Error())
	}
	reviewState := strings.TrimSpace(e.Status)
	if reviewState == "" {
		reviewState = string(model.TargetStatusReviewed)
	}
	changed, aerr := a.ApproveReviewUnit(ctx, recipePath, a.SourceLang, e.Locale, e.File, e.ID, reviewState)
	if aerr != nil {
		return errResult(res, aerr.Error())
	}
	if !changed {
		res.Status = "skipped"
		res.Detail = "already at this review state"
		return res
	}
	res.Status = "applied"
	res.Detail = fmt.Sprintf("%s %s → %s", e.Locale, e.ID, reviewState)
	return res
}

// applyMemoryEntry adds a source→target content memory pair. It edits the committed .memory.json
// source the recipe binds (creating l10n/tm.memory.json and binding it when none
// exists), then re-imports the .memory.json into the project content memory (.kapi/tm.db) cache.
func (a *App) applyMemoryEntry(ctx context.Context, cmd Command, e changeEntry) assetResult {
	res := assetResult{Kind: e.Kind, Op: e.Op, Target: e.Source}

	if e.Op != "" && e.Op != "add" {
		return errResult(res, fmt.Sprintf("tm: unsupported op %q (want \"add\")", e.Op))
	}
	if strings.TrimSpace(e.Source) == "" || strings.TrimSpace(e.Target) == "" {
		return errResult(res, "tm: source and target are both required")
	}

	recipePath, root, err := a.resolveProjectRoot(cmd)
	if err != nil {
		return errResult(res, err.Error())
	}

	srcPath, err := a.ensureMemorySourceBinding(recipePath, root)
	if err != nil {
		return errResult(res, err.Error())
	}

	entries, err := loadKMBEntries(srcPath)
	if err != nil {
		return errResult(res, err.Error())
	}

	srcLocale := e.SourceLocale
	if srcLocale == "" {
		srcLocale = "en"
	}
	tgtLocale := e.TargetLocale
	if tgtLocale == "" {
		return errResult(res, "tm: target_locale is required")
	}

	// For a tm correction, `status` carries the review state: empty/`reviewed`
	// records a reviewed correction, `signed-off` the final sign-off.
	reviewState := e.Status
	switch reviewState {
	case "", string(model.TargetStatusReviewed), string(model.TargetStatusSignedOff):
	default:
		return errResult(res, fmt.Sprintf("tm: status must be empty, %q, or %q", model.TargetStatusReviewed, model.TargetStatusSignedOff))
	}

	entries, changed := upsertMemoryPair(entries, e.Source, e.Target, model.LocaleID(srcLocale), model.LocaleID(tgtLocale), reviewState)
	if !changed {
		res.Status = "skipped"
		res.Detail = "already present"
		return res
	}

	if err := writeKMB(srcPath, entries); err != nil {
		return errResult(res, err.Error())
	}
	if err := a.compileMemorySource(ctx, root, srcPath); err != nil {
		return errResult(res, err.Error())
	}

	res.Status = "applied"
	res.Detail = filepath.Base(srcPath)
	return res
}

// ensureMemorySourceBinding returns the committed bundle path bound via
// defaults.tm_source, scaffolding and binding l10n/memory.json when none is
// bound and the project holds no bundles yet.
//
// A project may keep many content-memory bundles — this repository commits one
// per content surface — and nothing in an unbound recipe says which of them
// reviewed edits belong in. Picking one silently would write approved content
// into the wrong surface, so an unbound project that already has bundles is an
// error naming the candidates rather than a guess.
func (a *App) ensureMemorySourceBinding(recipePath, root string) (string, error) {
	proj, err := project.LoadWithOptions(recipePath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return "", fmt.Errorf("load project: %w", err)
	}
	if bound := proj.Defaults.MemorySource; bound != "" {
		return resolveUnder(root, bound), nil
	}
	if existing := findMemoryBundles(root); len(existing) > 0 {
		return "", fmt.Errorf(
			"no defaults.tm_source is bound and this project already has %d content-memory %s (%s); "+
				"bind the one that reviewed edits belong in",
			len(existing), pluralBundles(len(existing)), strings.Join(existing, ", "))
	}
	rel := filepath.Join("l10n", kmb.ConventionalName)
	proj.Defaults.MemorySource = rel
	if err := project.Save(recipePath, proj); err != nil {
		return "", fmt.Errorf("bind tm source: %w", err)
	}
	return resolveUnder(root, rel), nil
}

// findMemoryBundles returns the project-relative paths of every content-memory
// bundle committed under root, skipping kapi's own state directory.
func findMemoryBundles(root string) []string {
	var out []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable subtree is not a binding conflict
		}
		if d.IsDir() {
			if d.Name() == project.StateDirName || d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if kmb.IsBundlePath(path) {
			if rel, rerr := filepath.Rel(root, path); rerr == nil {
				out = append(out, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	sort.Strings(out)
	return out
}

func pluralBundles(n int) string {
	if n == 1 {
		return "bundle"
	}
	return "bundles"
}

// compileMemorySource re-imports the committed .memory.json into the project content memory cache
// (the conventional .kapi/tm.db, the same file kapi extract/merge use).
func (a *App) compileMemorySource(ctx context.Context, root, srcPath string) error {
	dbPath := filepath.Join(root, project.StateDirName, "tm.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return fmt.Errorf("create tm dir: %w", err)
	}

	tm, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		return fmt.Errorf("open tm cache: %w", err)
	}
	defer tm.Close()

	if _, err := ImportKMBFile(ctx, tm, srcPath); err != nil {
		return fmt.Errorf("compile tm: %w", err)
	}
	a.RebuildMemorySearchIndexes(tm)
	return nil
}

// loadKMBEntries reads the entries from a .memory.json source, returning an empty
// slice when the file does not exist yet.
func loadKMBEntries(path string) ([]memory.Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open tm source: %w", err)
	}
	defer f.Close()
	file, err := kmb.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("parse tm source: %w", err)
	}
	return file.ModelEntries(), nil
}

// writeKMB serializes entries to a deterministic .memory.json document.
func writeKMB(path string, entries []memory.Entry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create source dir: %w", err)
	}
	data, err := kmb.Marshal(kmb.FromModel(entries, nil))
	if err != nil {
		return fmt.Errorf("marshal tm source: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write tm source: %w", err)
	}
	return nil
}

// upsertMemoryPair adds a source→target pair as a bilingual entry, keyed by a
// stable id so re-applying the same pair is idempotent. When the entry already
// holds the same target text for the target locale, it returns changed=false.
// upsertMemoryPair adds or updates a source→target correction in the .memory.json entry
// list. reviewState, when non-empty, is recorded on the entry's `review` property
// (the carrier that distinguishes `reviewed` from `signed-off`); an empty
// reviewState leaves the entry at the `reviewed` baseline. It returns changed =
// true when the target text OR the review state changed, so promoting an
// already-present translation to signed-off is not mistaken for a no-op.
func upsertMemoryPair(entries []memory.Entry, source, target string, srcLocale, tgtLocale model.LocaleID, reviewState string) ([]memory.Entry, bool) {
	id := memoryEntryID(source, srcLocale, tgtLocale)
	for i := range entries {
		if entries[i].ID != id {
			continue
		}
		sameTarget := entries[i].VariantText(tgtLocale) == target
		reviewChanged := setReviewProperty(&entries[i], reviewState)
		if sameTarget && !reviewChanged {
			return entries, false
		}
		if entries[i].Variants == nil {
			entries[i].Variants = map[model.LocaleID][]model.Run{}
		}
		entries[i].Variants[srcLocale] = []model.Run{{Text: &model.TextRun{Text: source}}}
		entries[i].Variants[tgtLocale] = []model.Run{{Text: &model.TextRun{Text: target}}}
		entries[i].UpdatedAt = time.Now().UTC()
		return entries, true
	}
	now := time.Now().UTC()
	e := memory.Entry{
		ID:          id,
		HintSrcLang: srcLocale,
		Variants: map[model.LocaleID][]model.Run{
			srcLocale: {{Text: &model.TextRun{Text: source}}},
			tgtLocale: {{Text: &model.TextRun{Text: target}}},
		},
		Origins: []memory.Origin{{
			Source:  "apply",
			AddedAt: now,
			AddedBy: "kapi-apply",
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	setReviewProperty(&e, reviewState)
	entries = append(entries, e)
	return entries, true
}

// setReviewProperty records the review state (signed-off; reviewed is the
// property-absent baseline) on a content-memory entry, returning whether it changed. An empty
// or `reviewed` state clears the property so the entry round-trips minimally.
// reviewPropertyKey is the .memory.json entry property `kapi apply` uses to tag a
// correction's review state in the content memory corpus. (Project review STATE now lives in
// the state store, core/state; this is the content memory-side tag the apply path still sets.)
const reviewPropertyKey = "review"

func setReviewProperty(e *memory.Entry, reviewState string) bool {
	want := reviewState
	if want == string(model.TargetStatusReviewed) {
		want = "" // reviewed is the property-absent baseline
	}
	if e.Properties[reviewPropertyKey] == want {
		return false
	}
	if want == "" {
		delete(e.Properties, reviewPropertyKey)
		return true
	}
	if e.Properties == nil {
		e.Properties = map[string]string{}
	}
	e.Properties[reviewPropertyKey] = want
	return true
}

// memoryEntryID derives a stable id for a source/locale-pair content-memory entry.
func memoryEntryID(source string, srcLocale, tgtLocale model.LocaleID) string {
	return fmt.Sprintf("apply:%s:%s:%s", srcLocale, tgtLocale, model.ComputeContentHash(source))
}

// ---------------------------------------------------------------------------
// brand → committed brand.yaml (VoiceProfile) → brand store compile
// ---------------------------------------------------------------------------

// applyBrandEntry adds a vocabulary rule to the committed brand voice profile
// YAML the recipe binds (creating l10n/brand-voice.yaml and binding it under
// defaults.brand_voice.profile_file when none exists), then re-imports the
// profile into the local brand store so the store reflects the committed source.
func (a *App) applyBrandEntry(ctx context.Context, cmd Command, e changeEntry) assetResult {
	res := assetResult{Kind: e.Kind, Op: e.Op, Target: e.Term}

	if e.Op != "" && e.Op != "add-rule" {
		return errResult(res, fmt.Sprintf("brand: unsupported op %q (want \"add-rule\")", e.Op))
	}
	if strings.TrimSpace(e.Term) == "" {
		return errResult(res, "brand: empty term")
	}
	if e.List != "forbidden" && e.List != "competitor" && e.List != "preferred" {
		return errResult(res, fmt.Sprintf("brand: unknown list %q (want forbidden, competitor, or preferred)", e.List))
	}

	recipePath, root, err := a.resolveProjectRoot(cmd)
	if err != nil {
		return errResult(res, err.Error())
	}

	profilePath, err := a.ensureBrandProfileBinding(recipePath, root)
	if err != nil {
		return errResult(res, err.Error())
	}

	profile, err := loadOrInitProfile(profilePath, root)
	if err != nil {
		return errResult(res, err.Error())
	}

	changed := upsertBrandRule(profile, e.List, e.Term, e.Replacement, e.Severity)
	if !changed {
		res.Status = "skipped"
		res.Detail = "already present"
		return res
	}

	if err := writeProfileYAML(profilePath, profile); err != nil {
		return errResult(res, err.Error())
	}
	if err := a.compileBrandProfile(ctx, cmd, profilePath); err != nil {
		return errResult(res, err.Error())
	}

	res.Status = "applied"
	res.Detail = filepath.Base(profilePath)
	return res
}

// ensureBrandProfileBinding returns the committed profile YAML path bound via
// defaults.brand_voice.profile_file, creating l10n/brand-voice.yaml and binding
// it when no profile_file is bound. A non-file binding (pack/store profile) is
// an error: apply edits a committed file, not a starter pack or a store row.
func (a *App) ensureBrandProfileBinding(recipePath, root string) (string, error) {
	proj, err := project.LoadWithOptions(recipePath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return "", fmt.Errorf("load project: %w", err)
	}
	if bv := proj.Defaults.BrandVoice; bv != nil {
		switch {
		case bv.ProfileFile != "":
			return resolveUnder(root, bv.ProfileFile), nil
		case bv.Pack != "" || bv.Profile != "":
			return "", errors.New("brand: defaults.brand_voice binds a pack/store profile, not a committed profile_file — bind a profile_file to apply rules")
		}
	}
	rel := filepath.Join("l10n", "brand-voice.yaml")
	proj.Defaults.BrandVoice = &project.BrandVoiceBinding{ProfileFile: rel}
	if err := project.Save(recipePath, proj); err != nil {
		return "", fmt.Errorf("bind brand voice profile: %w", err)
	}
	return resolveUnder(root, rel), nil
}

// loadOrInitProfile loads the committed profile YAML, or builds a minimal valid
// profile (named after the project root) when the file does not exist yet.
func loadOrInitProfile(path, root string) (*brand.VoiceProfile, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			name := filepath.Base(root)
			if name == "" || name == "." || name == string(filepath.Separator) {
				name = "Brand"
			}
			return &brand.VoiceProfile{Name: name}, nil
		}
		return nil, fmt.Errorf("open brand profile: %w", err)
	}
	defer f.Close()
	p, err := brand.LoadProfileYAML(f)
	if err != nil {
		return nil, fmt.Errorf("load brand profile: %w", err)
	}
	return p, nil
}

// upsertBrandRule adds a term rule to the named vocabulary list. It is
// idempotent: a rule with the same term, replacement, and severity already on
// the list returns changed=false.
func upsertBrandRule(profile *brand.VoiceProfile, list, term, replacement, severity string) bool {
	rule := brand.TermRule{Term: term, Replacement: replacement, Severity: severity}
	target := brandRuleList(profile, list)
	for _, existing := range *target {
		if existing.Term == term && existing.Replacement == replacement && existing.Severity == severity {
			return false
		}
	}
	// Replace an existing rule for the same term (different replacement/severity)
	// rather than appending a duplicate.
	for i := range *target {
		if (*target)[i].Term == term {
			(*target)[i] = rule
			return true
		}
	}
	*target = append(*target, rule)
	return true
}

// brandRuleList returns a pointer to the vocabulary slice named by list.
func brandRuleList(profile *brand.VoiceProfile, list string) *[]brand.TermRule {
	switch list {
	case "forbidden":
		return &profile.Vocabulary.ForbiddenTerms
	case "competitor":
		return &profile.Vocabulary.CompetitorTerms
	default: // "preferred"
		return &profile.Vocabulary.PreferredTerms
	}
}

// writeProfileYAML serializes a VoiceProfile to its committed YAML form.
func writeProfileYAML(path string, profile *brand.VoiceProfile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create profile dir: %w", err)
	}
	data, err := yaml.Marshal(profile)
	if err != nil {
		return fmt.Errorf("marshal brand profile: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write brand profile: %w", err)
	}
	return nil
}

// compileBrandProfile re-imports the committed profile YAML into the local
// brand store — the existing brand-import path (SaveProfileToStore's
// create/update by id) — so the store reflects the committed source.
func (a *App) compileBrandProfile(ctx context.Context, cmd Command, profilePath string) error {
	f, err := os.Open(profilePath)
	if err != nil {
		return fmt.Errorf("open brand profile: %w", err)
	}
	defer f.Close()
	profile, err := brand.LoadProfileYAML(f)
	if err != nil {
		return fmt.Errorf("load brand profile: %w", err)
	}

	store, _, err := a.OpenBrandStore(cmd)
	if err != nil {
		return fmt.Errorf("open brand store: %w", err)
	}
	defer store.Close()

	if profile.ID == "" {
		profile.ID = slugify(profile.Name)
	}
	profile.WorkspaceID = LocalWorkspace
	if _, gerr := store.GetProfile(ctx, profile.ID); gerr == nil {
		if uerr := store.UpdateProfile(ctx, profile); uerr != nil {
			return fmt.Errorf("update brand store: %w", uerr)
		}
		return nil
	}
	if cerr := store.CreateProfile(ctx, profile); cerr != nil {
		return fmt.Errorf("create brand store profile: %w", cerr)
	}
	return nil
}

// ---------------------------------------------------------------------------
// recipe → edit the .kapi recipe YAML in place via project load/save
// ---------------------------------------------------------------------------

// applyRecipeEntry sets a dotted recipe field to a JSON-decoded value, then
// saves the .kapi recipe. Only an allowlisted set of fields can be set; an
// unknown path or a value that does not decode into the field's type is an
// error, so a malformed change cannot silently corrupt the recipe.
func (a *App) applyRecipeEntry(cmd Command, e changeEntry) assetResult {
	res := assetResult{Kind: e.Kind, Op: e.Op, Target: e.Path}

	if e.Op != "" && e.Op != "set" {
		return errResult(res, fmt.Sprintf("recipe: unsupported op %q (want \"set\")", e.Op))
	}
	if e.Path == "" {
		return errResult(res, "recipe: empty path")
	}

	recipePath, _, err := a.resolveProjectRoot(cmd)
	if err != nil {
		return errResult(res, err.Error())
	}
	proj, err := project.LoadWithOptions(recipePath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return errResult(res, fmt.Sprintf("load project: %v", err))
	}

	changed, err := setRecipeField(proj, e.Path, e.Value)
	if err != nil {
		return errResult(res, err.Error())
	}
	if !changed {
		res.Status = "skipped"
		res.Detail = "already set"
		return res
	}
	if err := project.Save(recipePath, proj); err != nil {
		return errResult(res, fmt.Sprintf("save project: %v", err))
	}

	res.Status = "applied"
	return res
}

// setRecipeField sets one allowlisted dotted recipe field from a JSON value,
// reporting whether the value actually changed (for idempotency). The allowlist
// is deliberate: it bounds apply to the safe, structured recipe surface a fix
// loop legitimately edits and keeps every set type-checked.
func setRecipeField(proj *project.KapiProject, path string, raw json.RawMessage) (bool, error) {
	switch path {
	case "name":
		var v string
		if err := decodeRecipeValue(path, raw, &v); err != nil {
			return false, err
		}
		if proj.Name == v {
			return false, nil
		}
		proj.Name = v
		return true, nil

	case "defaults.source_language":
		var v string
		if err := decodeRecipeValue(path, raw, &v); err != nil {
			return false, err
		}
		if proj.Defaults.SourceLanguage == model.LocaleID(v) {
			return false, nil
		}
		proj.Defaults.SourceLanguage = model.LocaleID(v)
		return true, nil

	case "defaults.target_languages":
		var v []string
		if err := decodeRecipeValue(path, raw, &v); err != nil {
			return false, err
		}
		next := make([]model.LocaleID, len(v))
		for i, s := range v {
			next[i] = model.LocaleID(s)
		}
		if localesEqual(proj.Defaults.TargetLanguages, next) {
			return false, nil
		}
		proj.Defaults.TargetLanguages = next
		return true, nil

	case "defaults.encoding":
		var v string
		if err := decodeRecipeValue(path, raw, &v); err != nil {
			return false, err
		}
		if proj.Defaults.Encoding == v {
			return false, nil
		}
		proj.Defaults.Encoding = v
		return true, nil

	case "defaults.termbase":
		var v string
		if err := decodeRecipeValue(path, raw, &v); err != nil {
			return false, err
		}
		if proj.Defaults.Terms == v {
			return false, nil
		}
		proj.Defaults.Terms = v
		return true, nil

	case "defaults.termbase_source":
		var v string
		if err := decodeRecipeValue(path, raw, &v); err != nil {
			return false, err
		}
		if proj.Defaults.TermsSource == v {
			return false, nil
		}
		proj.Defaults.TermsSource = v
		return true, nil

	case "defaults.tm_source":
		var v string
		if err := decodeRecipeValue(path, raw, &v); err != nil {
			return false, err
		}
		if proj.Defaults.MemorySource == v {
			return false, nil
		}
		proj.Defaults.MemorySource = v
		return true, nil

	default:
		return false, fmt.Errorf("recipe: unknown or unsettable path %q", path)
	}
}

// decodeRecipeValue JSON-decodes a recipe value into dst, wrapping the error
// with the path so a type mismatch is actionable.
func decodeRecipeValue(path string, raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		return fmt.Errorf("recipe: %s has no value", path)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("recipe: %s: %w", path, err)
	}
	return nil
}

func localesEqual(a, b []model.LocaleID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// shared helpers
// ---------------------------------------------------------------------------

// errResult stamps an error status + detail onto a result.
func errResult(res assetResult, detail string) assetResult {
	res.Status = "error"
	res.Detail = detail
	return res
}

// resolveUnder resolves a recipe-relative path against the project root,
// leaving absolute paths untouched.
func resolveUnder(root, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(root, p)
}

// loadRecipeForRoot loads the recipe at <root>/<dir>.kapi-style layout via the
// resolved layout, used when only the root is in hand.
func (a *App) loadRecipeForRoot(root string) (*project.KapiProject, error) {
	layout, err := project.ResolveLayout(root)
	if err != nil {
		return nil, err
	}
	return project.LoadWithOptions(layout.RecipePath, project.LoadOptions{SkipRequiresCheck: true})
}
