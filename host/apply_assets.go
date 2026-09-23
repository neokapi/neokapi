package host

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
)

// applyAssetEntry lands one asset change (a term, content memory pair, voice
// rule, or recipe field).
//
// A term, a pair and a rule are CONTEXT, and land in the project's stores in
// the user's workspace: the terms store, the content memory and the voice
// profile the recipe binds by name. A recipe field is CONFIGURATION, and lands
// in `kapi.yaml` in the checkout. Nothing here writes a context file.
//
// Applying is idempotent: a term, pair or rule already there with the same
// value is a "skipped" no-op, safe to re-run in a check→fix loop.
//
// No AI provider or credential is touched. Every asset kind requires a kapi
// project; without one the result is a precise error so the caller exits on the
// gate code and the fix loop can act.
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

// applyRecordedAssetEntry lands an asset change and records it in the project's
// context history.
//
// An asset entry states a decision about the project's vocabulary or its
// content memory, so it belongs in the same history as every other such
// decision: `kapi context log` shows what `kapi apply` did beside what an agent
// suggested and what a person kept. The operation is recorded as an edit, which
// is established the moment it lands, because a person ran the command.
//
// Every surface that applies a change-set comes through here, so an asset edit
// is in the log whichever one made it.
//
// The policy is put first, before anything is written. An agent that names
// itself in an entry's `actor` is refused here rather than after the store has
// already moved.
func (a *App) applyRecordedAssetEntry(ctx context.Context, cmd Command, e changeEntry) assetResult {
	actor := contextop.Actor{Kind: contextop.ActorPerson}
	if e.Actor != nil {
		actor = *e.Actor
		if actor.Kind == "" {
			actor.Kind = contextop.ActorPerson
		}
	}
	subject, records := assetSubject(e)
	if e.Kind != kindRecipe {
		if err := contextop.PersonDecides(contextop.Transition{
			Actor:   actor,
			Kind:    contextop.KindEdit,
			Subject: subject.Kind,
		}); err != nil {
			return errResult(assetResult{Kind: e.Kind, Op: e.Op, Target: e.Term}, err.Error())
		}
	}

	res := a.applyAssetEntry(ctx, cmd, e)
	if !records || res.Status != "applied" {
		return res
	}
	if err := a.recordAppliedAsset(ctx, cmd, actor, subject, e.Evidence); err != nil {
		// The decision is in the store; only its history is missing. Say so on
		// the result rather than failing a write that already happened.
		res.Detail = strings.TrimSpace(res.Detail + "; not recorded in the context history: " + err.Error())
	}
	return res
}

// assetSubject reads an asset entry as the context subject it decides, and
// reports whether the entry decides one. A recipe field is configuration rather
// than context, and a voice-profile vocabulary rule is recorded by the voice
// profile's own history, so neither records an operation here.
func assetSubject(e changeEntry) (contextop.Subject, bool) {
	switch e.Kind {
	case kindTerm:
		return contextop.Subject{Kind: contextop.SubjectTerm, Term: &coreprofile.TermRule{
			Term:        e.Term,
			Replacement: e.Replacement,
		}}, true
	case kindMemory:
		return contextop.Subject{Kind: contextop.SubjectMemory, Memory: &contextop.MemoryPair{
			Source:       e.Source,
			Target:       e.Target,
			SourceLocale: e.SourceLocale,
			TargetLocale: e.TargetLocale,
		}}, true
	}
	return contextop.Subject{}, false
}

// recordAppliedAsset writes the operation an applied asset entry produced: one
// edit, established from the start, because the person who ran the command
// wrote the rule directly.
func (a *App) recordAppliedAsset(ctx context.Context, cmd Command, actor contextop.Actor, subject contextop.Subject, evidence []contextop.Evidence) error {
	recipePath, err := ResolveProjectPath(cmd)
	if err != nil || recipePath == "" {
		return err
	}
	s, err := a.contextOps(ctx, recipePath)
	if err != nil {
		return err
	}
	_, err = s.ledger.Append(ctx, s.stamp(contextop.Record{
		Actor:    actor,
		Kind:     contextop.KindEdit,
		Subject:  subject,
		Evidence: evidence,
		Note:     "applied with `kapi apply`",
	}, evidence))
	return err
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
// term → the project's terms store
// ---------------------------------------------------------------------------

// landedTerms, landedMemory and landedVoice name where a change went, for the
// one line a surface prints beside the result.
const (
	landedTerms  = "the project's terms store"
	landedMemory = "the project's content memory"
	landedVoice  = "voice profile "
)

// applyTermEntry upserts a term into the project's terms store.
func (a *App) applyTermEntry(ctx context.Context, cmd Command, e changeEntry) assetResult {
	res := assetResult{Kind: e.Kind, Op: e.Op, Target: e.Term}

	if e.Op != "" && e.Op != "upsert" {
		return errResult(res, fmt.Sprintf("term: unsupported op %q (want \"upsert\")", e.Op))
	}
	if strings.TrimSpace(e.Term) == "" {
		return errResult(res, "term: empty term")
	}

	_, root, err := a.resolveProjectRoot(cmd)
	if err != nil {
		return errResult(res, err.Error())
	}
	db, err := a.ProjectDB(ctx, root)
	if err != nil {
		return errResult(res, err.Error())
	}
	tb := db.Terms()
	if tb == nil {
		return errResult(res, fmt.Sprintf("term: %v", projectdb.ErrNoStore))
	}
	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return errResult(res, fmt.Sprintf("read the project's terms: %v", err))
	}

	locale := e.Locale
	if locale == "" {
		locale = "en"
	}
	status := model.TermStatus(e.Status)
	if status == "" {
		status = model.TermPreferred
	}

	concepts, target, changed := upsertTerm(concepts, termDecision{
		Text:           e.Term,
		Locale:         model.LocaleID(locale),
		Status:         status,
		Replacement:    e.Replacement,
		Replaces:       e.Replaces,
		DoNotTranslate: e.DoNotTranslate,
	})
	if !changed {
		res.Status = "skipped"
		res.Detail = "already present"
		return res
	}
	if err := tb.AddConcept(ctx, concepts[target]); err != nil {
		return errResult(res, fmt.Sprintf("write concept %s: %v", concepts[target].ID, err))
	}

	res.Status = "applied"
	res.Detail = landedTerms
	return res
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
	// DoNotTranslate sets (true) or clears (false) the do-not-translate flag on
	// the concept the term joins; nil leaves it.
	DoNotTranslate *bool
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
// within a locale; a new concept is keyed by a stable id so reading the same
// decision again is reproducible.
//
// target is the index of the one concept the decision touched, which is the
// concept the caller writes back to the store.
func upsertTerm(concepts []terms.Concept, d termDecision) (out []terms.Concept, target int, changed bool) {
	// Canonical before it reaches the store or the concept id, so a decision
	// written as "en_US" joins the concept "en-US" already holds.
	d.Locale = model.NormalizeLocale(d.Locale)
	now := time.Now().UTC()
	noteWant := terms.ReplacementNote(d.Replacement)

	target = indexOfTerm(concepts, d.Text, d.Locale)
	if target < 0 && d.Replaces != "" {
		target = indexOfConcept(concepts, d.Replaces, d.Locale)
	}
	if target < 0 && d.Replacement != "" {
		target = indexOfTerm(concepts, d.Replacement, d.Locale)
	}

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

	if d.DoNotTranslate != nil && c.DoNotTranslate != *d.DoNotTranslate {
		c.DoNotTranslate = *d.DoNotTranslate
		changed = true
	}

	if changed {
		c.UpdatedAt = now
	}
	return concepts, target, changed
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
// locale so re-applying the same term re-seeds the same concept. The locale is
// embedded in canonical form: the id is persisted, and two spellings of one
// locale must mint one concept.
func conceptID(text string, locale model.LocaleID) string {
	return "term:" + string(model.NormalizeLocale(locale)) + ":" + slugify(text)
}

// ---------------------------------------------------------------------------
// memory → the project's content memory
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

// applyMemoryEntry adds a source→target pair to the project's content memory.
func (a *App) applyMemoryEntry(ctx context.Context, cmd Command, e changeEntry) assetResult {
	res := assetResult{Kind: e.Kind, Op: e.Op, Target: e.Source}

	if e.Op != "" && e.Op != "add" {
		return errResult(res, fmt.Sprintf("memory: unsupported op %q (want \"add\")", e.Op))
	}
	if strings.TrimSpace(e.Source) == "" || strings.TrimSpace(e.Target) == "" {
		return errResult(res, "memory: source and target are both required")
	}

	srcLocale := e.SourceLocale
	if srcLocale == "" {
		srcLocale = "en"
	}
	if e.TargetLocale == "" {
		return errResult(res, "memory: target_locale is required")
	}

	// `status` carries the review state: empty/`reviewed` records a reviewed
	// correction, `signed-off` the final sign-off.
	reviewState := e.Status
	switch reviewState {
	case "", string(model.TargetStatusReviewed), string(model.TargetStatusSignedOff):
	default:
		return errResult(res, fmt.Sprintf("memory: status must be empty, %q, or %q", model.TargetStatusReviewed, model.TargetStatusSignedOff))
	}

	_, root, err := a.resolveProjectRoot(cmd)
	if err != nil {
		return errResult(res, err.Error())
	}
	db, err := a.ProjectDB(ctx, root)
	if err != nil {
		return errResult(res, err.Error())
	}
	tm := db.Memory()
	if tm == nil {
		return errResult(res, fmt.Sprintf("memory: %v", projectdb.ErrNoStore))
	}

	src := model.NormalizeLocale(model.LocaleID(srcLocale))
	tgt := model.NormalizeLocale(model.LocaleID(e.TargetLocale))
	held, ok, err := tm.GetEntry(ctx, memoryEntryID(e.Source, src, tgt))
	if err != nil {
		return errResult(res, fmt.Sprintf("read the project's content memory: %v", err))
	}
	entry, changed := upsertMemoryPair(held, ok, e.Source, e.Target, src, tgt, reviewState)
	if !changed {
		res.Status = "skipped"
		res.Detail = "already present"
		return res
	}
	if err := tm.Add(ctx, entry); err != nil {
		return errResult(res, fmt.Sprintf("write content-memory entry %s: %v", entry.ID, err))
	}
	a.RebuildMemorySearchIndexes(ctx, tm)

	res.Status = "applied"
	res.Detail = landedMemory
	return res
}

// upsertMemoryPair folds a source→target correction into the entry the content
// memory holds for it, keyed by a stable id so applying the same pair again is
// idempotent. held is the entry the store returned and ok whether it had one.
//
// reviewState, when non-empty, is recorded on the entry's `review` property
// (the carrier that distinguishes `reviewed` from `signed-off`); an empty
// reviewState leaves the entry at the `reviewed` baseline. It returns changed =
// true when the target text OR the review state changed, so promoting an
// already-present translation to signed-off is not mistaken for a no-op.
func upsertMemoryPair(held memory.Entry, ok bool, source, target string, srcLocale, tgtLocale model.LocaleID, reviewState string) (memory.Entry, bool) {
	srcLocale, tgtLocale = model.NormalizeLocale(srcLocale), model.NormalizeLocale(tgtLocale)
	id := memoryEntryID(source, srcLocale, tgtLocale)
	now := time.Now().UTC()

	if ok {
		sameTarget := held.VariantText(tgtLocale) == target
		reviewChanged := setReviewProperty(&held, reviewState)
		if sameTarget && !reviewChanged {
			return held, false
		}
		if held.Variants == nil {
			held.Variants = map[model.LocaleID][]model.Run{}
		}
		held.Variants[srcLocale] = []model.Run{{Text: &model.TextRun{Text: source}}}
		held.Variants[tgtLocale] = []model.Run{{Text: &model.TextRun{Text: target}}}
		held.UpdatedAt = now
		return held, true
	}

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
	return e, true
}

// setReviewProperty records the review state (signed-off; reviewed is the
// property-absent baseline) on a content-memory entry, returning whether it changed. An empty
// or `reviewed` state clears the property so the entry round-trips minimally.
// reviewPropertyKey is the entry property `kapi apply` uses to tag a
// correction's review state in the content memory. A unit's review state lives
// in the project's decision record (core/state); this is the content-memory
// side tag.
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

// memoryEntryID derives a stable id for a source/locale-pair content-memory
// entry. Both locales are embedded in canonical form: the id is persisted, and
// two spellings of one locale pair must name one entry.
func memoryEntryID(source string, srcLocale, tgtLocale model.LocaleID) string {
	return fmt.Sprintf("apply:%s:%s:%s", model.NormalizeLocale(srcLocale), model.NormalizeLocale(tgtLocale), model.ComputeContentHash(source))
}

// ---------------------------------------------------------------------------
// voice → the voice profile the recipe binds, in the project's voice store
// ---------------------------------------------------------------------------

// applyVoiceEntry adds a vocabulary rule to the voice profile the recipe binds,
// in the project's voice store.
func (a *App) applyVoiceEntry(ctx context.Context, cmd Command, e changeEntry) assetResult {
	res := assetResult{Kind: e.Kind, Op: e.Op, Target: e.Term}

	if e.Op != "" && e.Op != "add-rule" {
		return errResult(res, fmt.Sprintf("voice: unsupported op %q (want \"add-rule\")", e.Op))
	}
	if strings.TrimSpace(e.Term) == "" {
		return errResult(res, "voice: empty term")
	}
	if e.List != "forbidden" && e.List != "competitor" && e.List != "preferred" {
		return errResult(res, fmt.Sprintf("voice: unknown list %q (want forbidden, competitor, or preferred)", e.List))
	}

	recipePath, root, err := a.resolveProjectRoot(cmd)
	if err != nil {
		return errResult(res, err.Error())
	}
	db, err := a.ProjectDB(ctx, root)
	if err != nil {
		return errResult(res, err.Error())
	}
	store := db.Voice()
	if store == nil {
		return errResult(res, fmt.Sprintf("voice: %v", projectdb.ErrNoStore))
	}

	profile, err := a.boundVoiceProfileForWrite(ctx, db, recipePath, root)
	if err != nil {
		return errResult(res, err.Error())
	}

	if !upsertVoiceRule(profile, e.List, e.Term, e.Replacement, e.Advisory) {
		res.Status = "skipped"
		res.Detail = "already present"
		return res
	}
	if err := store.UpdateProfile(ctx, profile); err != nil {
		return errResult(res, fmt.Sprintf("write voice profile %s: %v", profile.ID, err))
	}

	res.Status = "applied"
	res.Detail = landedVoice + profile.ID
	return res
}

// boundVoiceProfileForWrite returns the voice profile a rule lands in: the one
// the recipe binds by name at the project's default point.
//
// A project that binds none gets a profile of its own, created in the store and
// bound in the recipe under `defaults.voice.profile`. A starter pack is
// read-only.
func (a *App) boundVoiceProfileForWrite(ctx context.Context, db *projectdb.DB, recipePath, root string) (*coreprofile.VoiceProfile, error) {
	store := db.Voice()
	proj, err := project.LoadWithOptions(recipePath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return nil, fmt.Errorf("load project: %w", err)
	}
	if bv := proj.Defaults.Voice; bv != nil {
		switch {
		case bv.Pack != "":
			return nil, fmt.Errorf("voice: defaults.voice binds the starter pack %q, which is read-only. Bind a profile of this project's own to apply rules", bv.Pack)
		case bv.Profile != "":
			if p, gerr := lookupProfileIn(ctx, store, bv.Profile); gerr == nil {
				return p, nil
			}
			return createVoiceProfile(ctx, store, bv.Profile)
		}
	}

	name := filepath.Base(root)
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "Voice"
	}
	profile, err := createVoiceProfile(ctx, store, name)
	if err != nil {
		return nil, err
	}
	if err := bindVoiceProfile(recipePath, "", profile.ID); err != nil {
		return nil, err
	}
	return profile, nil
}

// createVoiceProfile opens a voice profile of this name in the project's store.
func createVoiceProfile(ctx context.Context, store coreprofile.Store, name string) (*coreprofile.VoiceProfile, error) {
	id := slugify(name)
	if id == "" {
		id = "voice"
	}
	profile := &coreprofile.VoiceProfile{ID: id, Name: name, Scope: LocalScope}
	if err := store.CreateProfile(ctx, profile); err != nil {
		return nil, fmt.Errorf("create voice profile %s: %w", id, err)
	}
	return profile, nil
}

// upsertVoiceRule adds a term rule to the named vocabulary list. It is
// idempotent: a rule with the same term, replacement, and advisory marking
// already on the list returns changed=false.
func upsertVoiceRule(profile *coreprofile.VoiceProfile, list, term, replacement string, advisory bool) bool {
	rule := coreprofile.TermRule{Term: term, Replacement: replacement, Advisory: advisory}
	target := voiceRuleList(profile, list)
	for _, existing := range *target {
		if existing.Term == term && existing.Replacement == replacement && existing.Advisory == advisory {
			return false
		}
	}
	// Replace an existing rule for the same term (different replacement/advisory)
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

// ---------------------------------------------------------------------------
// recipe → edit the recipe YAML in place via project load/save
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
