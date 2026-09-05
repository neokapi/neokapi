package host

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/yamledit"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/kmb"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
)

// applyAssetEntry lands one asset change (a term, content memory pair, voice rule,
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
	case kindVoice:
		return a.applyVoiceEntry(ctx, cmd, e)
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
// term → committed .terms.json source → terms import compile into the project store
// ---------------------------------------------------------------------------

// applyTermEntry upserts a term. It edits the committed .terms.json source
// the recipe binds (creating .kapi/terms.json and binding it when none
// exists), then re-imports the whole .terms.json into the project store's
// vocabulary so the store reflects the committed source — one write path.
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

	file, err := loadKTBFile(srcPath)
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

	concepts, changed := upsertTerm(file.Concepts, termDecision{
		Text:        e.Term,
		Locale:      model.LocaleID(locale),
		Status:      status,
		Replacement: e.Replacement,
		Replaces:    e.Replaces,
	})
	if !changed {
		res.Status = "skipped"
		res.Detail = "already present"
		return res
	}
	file.Concepts = concepts

	if _, err := writeKTB(srcPath, file); err != nil {
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
// recipe binds via defaults.terms_source, creating a default
// (.kapi/terms.json) and writing the binding into the recipe when none is
// bound — so future runs are consistent.
func (a *App) ensureTermsSourceBinding(recipePath, root string) (string, error) {
	proj, err := project.LoadWithOptions(recipePath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return "", fmt.Errorf("load project: %w", err)
	}
	if bound := proj.Defaults.TermsSource; bound != "" {
		return resolveUnder(root, bound), nil
	}
	rel := project.RelStatePath(ktb.ConventionalName)
	proj.Defaults.TermsSource = rel
	// Only the SOURCE is bound. The compiled vocabulary has one destination now
	// — the project's own store — so there is no second binding to keep pointed
	// at it, and no way for the two to disagree.
	if err := project.Save(recipePath, proj); err != nil {
		return "", fmt.Errorf("bind terms source: %w", err)
	}
	return resolveUnder(root, rel), nil
}

// compileTermsSource re-imports the committed .terms.json into the project
// store's vocabulary tables — the single store-write path. The destination is
// always the project's own store: a recipe no longer names a compiled cache, so
// there is nothing to override it with.
func (a *App) compileTermsSource(ctx context.Context, root, srcPath string) error {
	db, err := a.ProjectDB(ctx, root)
	if err != nil {
		return err
	}
	tb := db.Terms()
	if tb == nil {
		return fmt.Errorf("compile terms: %w", projectdb.ErrNoStore)
	}

	f, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open terms source: %w", err)
	}
	defer f.Close()
	if _, err := ImportKTBFile(ctx, tb, f); err != nil {
		return fmt.Errorf("compile terms: %w", err)
	}
	a.stampContextSource(ctx, root, srcPath)
	return nil
}

// loadKTBFile reads a .terms.json source, returning an empty bundle when the
// file does not exist yet (the first term creates it). The whole document is
// returned — concepts and relations — so a writer round-trips everything the
// file carried rather than only the part it edited.
func loadKTBFile(path string) (*ktb.File, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ktb.FromConcepts(nil), nil
		}
		return nil, fmt.Errorf("open terms source: %w", err)
	}
	defer f.Close()
	file, err := ktb.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("parse terms source: %w", err)
	}
	return file, nil
}

// writeKTB serializes a bundle to a deterministic .terms.json document, creating
// parent directories, and reports whether the bytes on disk actually moved.
//
// The write is skipped when the serialization is identical to what is already
// there. A projection that rewrote the file every run would put a change under
// `.kapi/` on every night of an automated sync — and a change under `.kapi/` is
// what the erasure gate reads as the decision backing a derived artifact, so a
// file that churns on its own manufactures backing for artifacts nothing
// decided.
func writeKTB(path string, file *ktb.File) (bool, error) {
	data, err := ktb.Marshal(file)
	if err != nil {
		return false, fmt.Errorf("marshal terms source: %w", err)
	}
	if existing, rerr := os.ReadFile(path); rerr == nil && bytes.Equal(existing, data) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create source dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return false, fmt.Errorf("write terms source: %w", err)
	}
	return true, nil
}

// termDecision is one term entry as apply reads it off the ledger.
type termDecision struct {
	Text        string
	Locale      model.LocaleID
	Status      model.TermStatus
	Replacement string
	// Replaces names the concept the term joins: a concept id, or the text of a
	// term that concept already declares.
	Replaces string
}

// upsertTerm lands a term decision in the concept set.
//
// The term joins a CONCEPT rather than getting one of its own wherever the
// entry says which: the concept that already declares it, else the one
// `replaces` names, else the one that declares the replacement. Only a decision
// that names nothing already in the graph opens a new concept. That is what
// makes the decision answerable later — "what should this say instead" is the
// preferred term of the concept the retired word sits in, so a term filed on
// its own island can never be answered, however clearly the decision was
// written. A replacement no concept declares yet is added to the joined concept
// as its preferred term, for the same reason.
//
// It is idempotent: an entry already recorded this way returns changed=false so
// apply reports a skipped no-op. Terms are matched case-insensitively on text
// within a locale; a new concept is keyed by a stable id so re-seeding is
// reproducible.
func upsertTerm(concepts []terms.Concept, d termDecision) ([]terms.Concept, bool) {
	now := time.Now().UTC()
	noteWant := terms.ReplacementNote(d.Replacement)

	target := indexOfTerm(concepts, d.Text, d.Locale)
	if target < 0 && d.Replaces != "" {
		target = indexOfConcept(concepts, d.Replaces, d.Locale)
	}
	if target < 0 && d.Replacement != "" {
		target = indexOfTerm(concepts, d.Replacement, d.Locale)
	}

	changed := false
	if target < 0 {
		concepts = append(concepts, terms.Concept{
			ID:        conceptID(d.Text, d.Locale),
			Source:    terms.TermSourceTerminology,
			CreatedAt: now,
			UpdatedAt: now,
		})
		target = len(concepts) - 1
		changed = true
	}

	c := &concepts[target]
	if ti := termIndex(c, d.Text, d.Locale); ti >= 0 {
		t := &c.Terms[ti]
		if t.Status != d.Status || t.Text != d.Text || (noteWant != "" && t.Note != noteWant) {
			t.Status = d.Status
			t.Text = d.Text
			if noteWant != "" {
				t.Note = noteWant
			}
			changed = true
		}
	} else {
		c.Terms = append(c.Terms, terms.Term{
			Text:   d.Text,
			Locale: d.Locale,
			Status: d.Status,
			Note:   noteWant,
		})
		changed = true
	}

	// The replacement becomes the concept's preferred term when the graph does
	// not have it yet — including under another concept, which would otherwise
	// end up declaring the same word twice. A term the entry itself declares
	// preferred is not retired in favour of anything, so its replacement, if it
	// named one, stays in the note rather than contradicting it.
	if d.Replacement != "" && d.Status.Discouraged() && indexOfTerm(concepts, d.Replacement, d.Locale) < 0 {
		c.Terms = append(c.Terms, terms.Term{
			Text:   d.Replacement,
			Locale: d.Locale,
			Status: model.TermPreferred,
		})
		changed = true
	}

	if changed {
		c.UpdatedAt = now
	}
	return concepts, changed
}

// indexOfTerm returns the index of the concept declaring text in locale, or -1.
func indexOfTerm(concepts []terms.Concept, text string, locale model.LocaleID) int {
	for ci := range concepts {
		if termIndex(&concepts[ci], text, locale) >= 0 {
			return ci
		}
	}
	return -1
}

// indexOfConcept resolves a join key — a concept id, or the text of a term the
// concept declares in locale — to a concept index, or -1.
func indexOfConcept(concepts []terms.Concept, key string, locale model.LocaleID) int {
	for ci := range concepts {
		if concepts[ci].ID == key {
			return ci
		}
	}
	return indexOfTerm(concepts, key, locale)
}

// termIndex returns the index of the concept's term with this text in this
// locale, or -1.
func termIndex(c *terms.Concept, text string, locale model.LocaleID) int {
	for ti := range c.Terms {
		if c.Terms[ti].Locale == locale && strings.EqualFold(c.Terms[ti].Text, text) {
			return ti
		}
	}
	return -1
}

// conceptID derives a stable, filesystem-safe concept id from the term text and
// locale so re-applying the same term re-seeds the same concept.
func conceptID(text string, locale model.LocaleID) string {
	return "term:" + string(locale) + ":" + slugify(text)
}

// ---------------------------------------------------------------------------
// memory → committed .memory.json source → import compile into the project store
// ---------------------------------------------------------------------------

// applyReviewEntry records a review decision in the project STATE store via the
// shared ApproveReviewUnit path — the CLI counterpart of the desktop "approve"
// action and the write side of `kapi status --review`. The unit is addressed by
// (file, id, locale) exactly as the review queue lists it; `status` is "reviewed"
// (default) or "signed-off". This is distinct from a `kind:"memory"` entry: a content memory
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
	// The raw record of what --source-lang named, not the resolved read: empty is
	// how this hands the choice on to the recipe ApproveReviewUnit loads.
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
// source the recipe binds (creating l10n/memory.json and binding it when none
// exists), then re-imports the .memory.json into the project store's content memory.
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
// defaults.memory_source, scaffolding and binding `.kapi/memory/memory.json`
// when none is bound and the project holds no bundles yet.
//
// A project may keep many content-memory bundles — this repository commits one
// per content surface, all of them under `.kapi/memory/` — and nothing in an
// unbound recipe says which of them reviewed edits belong in. Picking one
// silently would write approved content into the wrong surface, so an unbound
// project that already has bundles is an error naming the candidates rather
// than a guess. The conventional primary is only the name a project gets when
// it has no bundle at all yet.
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
			"no defaults.memory_source is bound and this project already has %d content-memory %s (%s); "+
				"bind the one that reviewed edits belong in",
			len(existing), pluralBundles(len(existing)), strings.Join(existing, ", "))
	}
	rel := project.RelStatePath(project.MemoryDirName, kmb.ConventionalName)
	proj.Defaults.MemorySource = rel
	if err := project.Save(recipePath, proj); err != nil {
		return "", fmt.Errorf("bind tm source: %w", err)
	}
	return resolveUnder(root, rel), nil
}

// findMemoryBundles returns the project-relative paths of every content-memory
// bundle committed under root.
//
// `.kapi/` is walked like any other directory, because `.kapi/memory/` is where
// a project's bundles live and skipping it would hide the very bundles the
// binding conflict is about. Only `.kapi/work/` is excluded, along with `.git`
// and `node_modules`: a bundle staged in a cache is not a committed source and
// must never win the binding.
func findMemoryBundles(root string) []string {
	work := project.LayoutAt(root).WorkDir()
	var out []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable subtree is not a binding conflict
		}
		if d.IsDir() {
			if path == work || d.Name() == ".git" || d.Name() == "node_modules" {
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

// compileMemorySource re-imports the committed .memory.json into the project
// store's content memory — the same one kapi extract/merge read and write.
func (a *App) compileMemorySource(ctx context.Context, root, srcPath string) error {
	db, err := a.ProjectDB(ctx, root)
	if err != nil {
		return err
	}
	if _, err := a.compileMemoryBundle(ctx, db, root, srcPath); err != nil {
		return fmt.Errorf("compile content memory: %w", err)
	}
	a.RebuildMemorySearchIndexes(ctx, db.Memory())
	a.stampContextSource(ctx, root, srcPath)
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
// voice → committed voice.yaml (VoiceProfile) → voice store compile
// ---------------------------------------------------------------------------

// applyVoiceEntry adds a vocabulary rule to the committed voice profile
// YAML the recipe binds (creating .kapi/voice.yaml and binding it under
// defaults.voice.profile_file when none exists), then re-imports the
// profile into the local voice store so the store reflects the committed source.
func (a *App) applyVoiceEntry(ctx context.Context, cmd Command, e changeEntry) assetResult {
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

	profilePath, err := a.ensureVoiceProfileBinding(recipePath, root)
	if err != nil {
		return errResult(res, err.Error())
	}

	profile, err := loadOrInitProfile(profilePath, root)
	if err != nil {
		return errResult(res, err.Error())
	}

	changed := upsertVoiceRule(profile, e.List, e.Term, e.Replacement, e.Severity)
	if !changed {
		res.Status = "skipped"
		res.Detail = "already present"
		return res
	}

	if err := writeProfileYAML(profilePath, profile); err != nil {
		return errResult(res, err.Error())
	}
	if err := a.compileVoiceProfile(ctx, cmd, profilePath); err != nil {
		return errResult(res, err.Error())
	}

	res.Status = "applied"
	res.Detail = filepath.Base(profilePath)
	return res
}

// ensureVoiceProfileBinding returns the committed profile YAML path bound via
// defaults.voice.profile_file, creating .kapi/voice.yaml and binding
// it when no profile_file is bound. A non-file binding (pack/store profile) is
// an error: apply edits a committed file, not a starter pack or a store row.
func (a *App) ensureVoiceProfileBinding(recipePath, root string) (string, error) {
	proj, err := project.LoadWithOptions(recipePath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return "", fmt.Errorf("load project: %w", err)
	}
	if bv := proj.Defaults.Voice; bv != nil {
		switch {
		case bv.ProfileFile != "":
			return resolveUnder(root, bv.ProfileFile), nil
		case bv.Pack != "" || bv.Profile != "":
			return "", errors.New("brand: defaults.voice binds a pack/store profile rather than a committed profile_file. Bind a profile_file to apply rules")
		}
	}
	rel := project.RelStatePath(VoiceConventionalName)
	proj.Defaults.Voice = &project.VoiceBinding{ProfileFile: rel}
	if err := project.Save(recipePath, proj); err != nil {
		return "", fmt.Errorf("bind voice profile: %w", err)
	}
	return resolveUnder(root, rel), nil
}

// loadOrInitProfile loads the committed profile YAML, or builds a minimal valid
// profile (named after the project root) when the file does not exist yet.
func loadOrInitProfile(path, root string) (*coreprofile.VoiceProfile, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			name := filepath.Base(root)
			if name == "" || name == "." || name == string(filepath.Separator) {
				name = "Brand"
			}
			return &coreprofile.VoiceProfile{Name: name}, nil
		}
		return nil, fmt.Errorf("open voice profile: %w", err)
	}
	defer f.Close()
	p, err := coreprofile.LoadProfileYAML(f)
	if err != nil {
		return nil, fmt.Errorf("load voice profile: %w", err)
	}
	return p, nil
}

// upsertVoiceRule adds a term rule to the named vocabulary list. It is
// idempotent: a rule with the same term, replacement, and severity already on
// the list returns changed=false.
func upsertVoiceRule(profile *coreprofile.VoiceProfile, list, term, replacement, severity string) bool {
	rule := coreprofile.TermRule{Term: term, Replacement: replacement, Severity: severity}
	target := voiceRuleList(profile, list)
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

// voiceRuleList returns a pointer to the vocabulary slice named by list.
func voiceRuleList(profile *coreprofile.VoiceProfile, list string) *[]coreprofile.TermRule {
	switch list {
	case "forbidden":
		return &profile.Vocabulary.ForbiddenTerms
	case "competitor":
		return &profile.Vocabulary.CompetitorTerms
	default: // "preferred"
		return &profile.Vocabulary.PreferredTerms
	}
}

// writeProfileYAML writes a VoiceProfile back into its committed YAML form,
// keeping the document's comments and key order and leaving the file untouched
// when nothing about it moved (core/yamledit). A voice profile is authored,
// reviewed source: a decision that lands three lines of vocabulary and also
// deletes the file's explanation of itself is not a reviewable diff, and a
// rewrite that changed nothing at all would still register under `.kapi/` as
// the decision backing whatever else a run produced.
func writeProfileYAML(path string, profile *coreprofile.VoiceProfile) error {
	if _, err := yamledit.WriteFile(path, profile, 0o644); err != nil {
		return fmt.Errorf("write voice profile: %w", err)
	}
	return nil
}

// compileVoiceProfile re-imports the committed profile YAML into the local
// voice store — the existing brand-import path (SaveProfileToStore's
// create/update by id) — so the store reflects the committed source.
func (a *App) compileVoiceProfile(ctx context.Context, cmd Command, profilePath string) error {
	f, err := os.Open(profilePath)
	if err != nil {
		return fmt.Errorf("open voice profile: %w", err)
	}
	defer f.Close()
	profile, err := coreprofile.LoadProfileYAML(f)
	if err != nil {
		return fmt.Errorf("load voice profile: %w", err)
	}

	store, _, release, err := a.OpenVoiceStore(cmd)
	if err != nil {
		return fmt.Errorf("open voice store: %w", err)
	}
	defer release()

	if profile.ID == "" {
		profile.ID = slugify(profile.Name)
	}
	profile.Scope = LocalScope
	if _, gerr := store.GetProfile(ctx, profile.ID); gerr == nil {
		if uerr := store.UpdateProfile(ctx, profile); uerr != nil {
			return fmt.Errorf("update voice store: %w", uerr)
		}
		return nil
	}
	if cerr := store.CreateProfile(ctx, profile); cerr != nil {
		return fmt.Errorf("create voice store profile: %w", cerr)
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

	changed, err := project.SetField(proj, e.Path, e.Value)
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
